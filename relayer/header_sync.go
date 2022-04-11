package relayer

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/polynetwork/bridge-common/log"
	"github.com/top/top-relayer/base"
	"github.com/top/top-relayer/config"
	"github.com/top/top-relayer/msg"
)

type HeaderSyncHandler struct {
	context.Context
	wg        *sync.WaitGroup
	listener  IChainListener
	submitter IChainSubmitter
	height    uint64
	config    *config.HeaderSyncConfig
	reset     chan uint64
}

func NewHeaderSyncHandler(config *config.HeaderSyncConfig) *HeaderSyncHandler {
	return &HeaderSyncHandler{
		listener:  GetListener(config.ChainId),
		submitter: GetSubmitter(config.Submitter.ChainId),
		config:    config,
		reset:     make(chan uint64, 1),
	}
}

func (h *HeaderSyncHandler) Init(ctx context.Context, wg *sync.WaitGroup) (err error) {
	h.Context = ctx
	h.wg = wg

	err = h.submitter.Init(h.config)
	if err != nil {
		return
	}

	if h.listener == nil {
		return fmt.Errorf("Unabled to create listener for chain %s", base.GetChainName(h.config.ChainId))
	}

	err = h.listener.Init(h.config, h.submitter.SDK())
	if err != nil {
		return
	}

	return
}

func (h *HeaderSyncHandler) monitor(ch chan<- uint64) {
	timer := time.NewTicker(60 * time.Second)
	for {
		select {
		case <-h.Done():
			return
		case <-timer.C:
			switch h.config.ChainId {
			case base.BSC, base.ETH:
				height, err := h.submitter.GetSideChainHeight(h.config.ChainId)
				if err == nil {
					ch <- height
				}
			default:
			}
		}
	}
}

func (h *HeaderSyncHandler) RollbackToCommonAncestor(height, target uint64) uint64 {
	log.Warn("Rolling header sync back to common ancestor", "current", height, "goal", target, "chain", h.config.ChainId)
	switch h.config.ChainId {
	case base.ETH, base.BSC:
	default:
		return target
	}

	var (
		a, b []byte
		err  error
	)
	for {
		// Check err here?
		b, _ = h.submitter.GetSideChainHeader(h.config.ChainId, target)
		if len(b) == 0 {
			target--
			continue
		}
		_, a, err = h.listener.Header(target)
		if err == nil {
			if bytes.Equal(a, b) {
				log.Info("Found common ancestor", "chain", h.config.ChainId, "height", target)
				return target
			} else {
				target--
				continue
			}
		} else {
			log.Error("RollbackToCommonAncestor error", "chain", h.config.ChainId, "height", target)
			time.Sleep(time.Second)
		}
	}
}

func (h *HeaderSyncHandler) watch() {
	h.wg.Add(1)
	defer h.wg.Done()
	ticker := time.NewTicker(3 * time.Second)
	last := uint64(0)
	for {
		select {
		case <-h.Done():
			return
		case <-ticker.C:
			height, err := h.listener.Nodes().Node().GetLatestHeight()
			if err != nil {
				log.Error("Watch chain latest height error", "chain", h.config.ChainId, "err", err)
			} else if height > last {
				log.Info("Latest chain height", "chain", h.config.ChainId, "height", height)
				// h.latest.UpdateHeight(context.Background(), height)
				last = height
			}

			switch h.config.ChainId {
			case base.BSC, base.ETH:
				height, err = h.submitter.GetSideChainHeight(h.config.ChainId)
				if err != nil {
					log.Error("Watch chain sync height error", "chain", h.config.ChainId, "err", err)
				} else {
					log.Info("Latest chain sync height", "chain", h.config.ChainId, "height", height)
				}
			default:
				height = 0
			}
		}
	}
}

func (h *HeaderSyncHandler) start(ch chan msg.Header) {
	h.wg.Add(1)
	defer h.wg.Done()
	confirms := uint64(h.listener.Defer())
	var (
		latest uint64
		ok     bool
	)
LOOP:
	for {
		select {
		case reset := <-h.reset:
			if reset < h.height && reset != 0 {
				// Drain the headers buf
			DRAIN:
				for {
					select {
					case <-ch:
					default:
						break DRAIN
					}
				}

				log.Info("Detected submit failure reset", "chain", h.config.ChainId, "value", reset)
				h.height = h.RollbackToCommonAncestor(h.height, reset-1)
			}
		case <-h.Done():
			break LOOP
		default:
		}

		h.height++
		log.Debug("Header sync processing block", "height", h.height, "chain", h.config.ChainId)
		if latest < h.height+confirms {
			latest, ok = h.listener.Nodes().WaitTillHeight(h.Context, h.height+confirms, h.listener.ListenCheck())
			if !ok {
				break LOOP
			}
		}
		header, hash, err := h.listener.Header(h.height)
		log.Debug("Header sync fetched block header", "height", h.height, "chain", h.config.ChainId, "err", err)
		if err == nil {
			select {
			case ch <- msg.Header{Data: header, Height: h.height, Hash: hash}:
			case <-h.Done():
				break LOOP
			}
			continue
		} else {
			log.Error("Fetch block header error", "chain", h.config.ChainId, "height", h.height, "err", err)
		}
		h.height--
	}
	log.Info("Header sync handler is exiting...", "chain", h.config.ChainId, "height", h.height)
	close(ch)
}

func (h *HeaderSyncHandler) Start() (err error) {
	// Last successful sync height
	h.height, err = h.listener.LastHeaderSync(0, 0)
	if err != nil {
		return
	}
	log.Info("Header sync will start...", "height", h.height+1, "force", 0, "last", 0, "chain", h.config.ChainId)
	ch, err := h.submitter.StartSync(h.Context, h.wg, h.reset)
	if err != nil {
		return
	}
	go h.watch()
	go h.start(ch)
	return
}

func (h *HeaderSyncHandler) Stop() (err error) {
	return
}

func (h *HeaderSyncHandler) Chain() uint64 {
	return h.config.ChainId
}
