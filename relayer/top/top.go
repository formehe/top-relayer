package top

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ontio/ontology-crypto/signature"

	"github.com/polynetwork/bridge-common/chains/eth"
	"github.com/polynetwork/bridge-common/log"
	"github.com/polynetwork/bridge-common/wallet"

	"github.com/top/top-relayer/abi/hsc"
	"github.com/top/top-relayer/base"
	"github.com/top/top-relayer/config"
	"github.com/top/top-relayer/msg"
)

type Submitter struct {
	context.Context
	wg     *sync.WaitGroup
	sdk    *eth.SDK
	wallet wallet.IWallet
	name   string
	config *config.HeaderSyncConfig

	hscontract common.Address
	// Check last header commit
	lastCommit uint64
	lastCheck  uint64

	blocksToWait uint64
}

func (s *Submitter) Init(config *config.HeaderSyncConfig) (err error) {
	if config.ChainId != base.TOP {
		return fmt.Errorf("eth submit invalid chain id %d", config.ChainId)
	}

	s.config = config
	s.sdk, err = eth.WithOptions(config.ChainId, config.Nodes, time.Minute, 1)
	if err != nil {
		return
	}

	if config.Submitter.Wallet != nil {
		sdk, err := eth.WithOptions(config.ChainId, config.Submitter.Wallet.Nodes, time.Minute, 1)
		if err != nil {
			return err
		}
		w := wallet.New(config.Submitter.Wallet, sdk)
		err = w.Init()
		if err != nil {
			return err
		}

		s.wallet = w.Upgrade()
	}
	s.name = base.GetChainName(config.Submitter.ChainId)
	s.hscontract = common.HexToAddress(config.Submitter.HSContract)
	return
}

func (s *Submitter) SDK() *eth.SDK {
	return s.sdk
}

func (s *Submitter) Hook(ctx context.Context, wg *sync.WaitGroup, ch <-chan msg.Message) error {
	s.Context = ctx
	s.wg = wg
	return nil
}

func (s *Submitter) SubmitHeadersWithLoop(chainId uint64, headers [][]byte, header *msg.Header) (err error) {
	start := time.Now()
	h := uint64(0)
	if len(headers) > 0 {
		err = s.submitHeadersWithLoop(chainId, headers, header)
		if err == nil && header != nil {
			// Check last commit every 4 successful submit
			if s.lastCommit > 0 && s.lastCheck > 3 {
				s.lastCheck = 0
				height, e := s.GetSideChainHeight(chainId)
				if e != nil {
					log.Error("Get side chain header height failure", "err", e)
				} else if height < s.lastCommit {
					log.Error("Chain header submit confirm check failure", "chain", s.name, "height", height, "last_submit", s.lastCommit)
					err = msg.ERR_HEADER_MISSING
				} else {
					log.Info("Chain header submit confirm check success", "chain", s.name, "height", height, "last_submit", s.lastCommit)
				}
			} else {
				s.lastCheck++
			}
		}
	}
	if header != nil {
		h = header.Height
		if err == nil {
			s.lastCommit = header.Height // Mark last commit
		}
	}
	log.Info("Submit headers to poly", "chain", chainId, "size", len(headers), "height", h, "elapse", time.Since(start), "err", err)
	return
}

func (s *Submitter) submitHeadersWithLoop(chainId uint64, headers [][]byte, header *msg.Header) error {
	attempt := 0
	var ok bool
	for {
		var err error
		if header != nil {
			ok, err = s.CheckHeaderExistence(header)
			if ok {
				return nil
			}
			if err != nil {
				log.Error("Failed to check header existence", "chain", chainId, "height", header.Height)
			}
		}

		if err == nil {
			attempt += 1
			_, err = s.SubmitHeaders(chainId, headers)
			if err == nil {
				return nil
			}
			info := err.Error()
			if strings.Contains(info, "parent header not exist") ||
				strings.Contains(info, "missing required field") ||
				strings.Contains(info, "parent block failed") ||
				strings.Contains(info, "span not correct") ||
				strings.Contains(info, "VerifySpan err") {
				//NOTE: reset header height back here
				log.Error("Possible hard fork, will rollback some blocks", "chain", chainId, "err", err)
				return msg.ERR_HEADER_INCONSISTENT
			}
			log.Error("Failed to submit header to poly", "chain", chainId, "err", err)
		}
		select {
		case <-s.Done():
			log.Warn("Header submitter exiting with headers not submitted", "chain", chainId)
			return nil
		default:
			if attempt > 30 {
				log.Error("Header submit too many failed attempts", "chain", chainId, "attempts", attempt)
				return msg.ERR_HEADER_SUBMIT_FAILURE
			}
			time.Sleep(time.Second)
		}
	}
}

func (s *Submitter) SubmitHeaders(chainId uint64, headers [][]byte) (hash string, err error) {
	hsContract, err := hsc.NewHsc(s.hscontract, s.sdk.Node())
	if err != nil {
		return "", err
	}

	tx, err := hsContract.SyncBlockHeader(nil, headers[0])
	if err != nil {
		return "", err
	}

	hash, err = s.wallet.Send(s.hscontract, big.NewInt(0), 0, nil, nil, tx.Data())
	txHash := common.HexToHash(hash)
	_, _, _, err = s.sdk.Node().Confirm(txHash, 0, 10)
	if err == nil {
		log.Info("Submitted header to top", "chain", chainId, "hash", hash)
	}
	return
}

func (s *Submitter) Stop() error {
	s.wg.Wait()
	return nil
}

func (s *Submitter) CollectSigs(tx *msg.Tx) (err error) {
	var (
		sigs []byte
	)
	sigHeader := tx.PolyHeader
	if tx.AnchorHeader != nil && tx.AnchorProof != "" {
		sigHeader = tx.AnchorHeader
	}
	for _, sig := range sigHeader.SigData {
		temp := make([]byte, len(sig))
		copy(temp, sig)
		s, err := signature.ConvertToEthCompatible(temp)
		if err != nil {
			return fmt.Errorf("MakeTx signature.ConvertToEthCompatible %v", err)
		}
		sigs = append(sigs, s...)
	}
	tx.PolySigs = sigs
	return
}

func (s *Submitter) ReadyBlock() (height uint64) {
	var err error
	height, err = s.GetSideChainHeight(s.config.ChainId)
	if height > s.blocksToWait {
		height -= s.blocksToWait
	}
	if err != nil {
		log.Error("Failed to get ready block height", "chain", s.name, "err", err)
	}
	return
}

func (s *Submitter) StartSync(
	ctx context.Context, wg *sync.WaitGroup, reset chan<- uint64,
) (ch chan msg.Header, err error) {
	s.Context = ctx
	s.wg = wg

	if s.config.Batch == 0 {
		s.config.Batch = 1
	}
	if s.config.Buffer == 0 {
		s.config.Buffer = 2 * s.config.Batch
	}
	if s.config.Timeout == 0 {
		s.config.Timeout = 1
	}

	if s.config.ChainId == 0 {
		return nil, fmt.Errorf("Invalid header sync side chain id")
	}

	ch = make(chan msg.Header, s.config.Buffer)
	go s.startSync(ch, reset)
	return
}

func (s *Submitter) GetSideChainHeight(chainId uint64) (height uint64, err error) {
	hscontract, err := hsc.NewHscCaller(s.hscontract, s.sdk.Node())
	if err != nil {
		return 0, err
	}

	hscRaw := hsc.HscCallerRaw{Contract: hscontract}
	result := make([]interface{}, 1)
	err = hscRaw.Call(nil, &result, "getCurrentBlockHeight", chainId)
	if err != nil {
		return 0, err
	}

	value, success := result[0].(uint64)
	if !success {
		return 0, fmt.Errorf("fail to convert error")
	}

	return value, nil
}

func (s *Submitter) GetSideChainHeader(chainId, height uint64) (hash []byte, err error) {
	hsContract, err := hsc.NewHscCaller(s.hscontract, s.sdk.Node())
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, 1)
	hscRaw := hsc.HscCallerRaw{Contract: hsContract}
	err = hscRaw.Call(nil, &result, "getBlockBashByHeight", chainId, height)
	if err != nil {
		return
	}

	hash, success := result[0].([]byte)
	if !success {
		return nil, fmt.Errorf("fail to convert error")
	}

	return hash, nil
}

func (s *Submitter) CheckHeaderExistence(header *msg.Header) (ok bool, err error) {
	hash, err := s.GetSideChainHeader(s.config.ChainId, header.Height)
	if err != nil {
		return
	}
	ok = bytes.Equal(hash, header.Hash)
	return true, nil
}

func (s *Submitter) syncHeaderLoop(ch <-chan msg.Header, reset chan<- uint64) {
	for {
		select {
		case <-s.Done():
			return
		case header, ok := <-ch:
			if !ok {
				return
			}
			// NOTE err reponse here will revert header sync with delta - 2
			headers := [][]byte{header.Data}
			if header.Data == nil {
				headers = nil
			}
			err := s.SubmitHeadersWithLoop(s.config.ChainId, headers, &header)
			if err != nil {
				reset <- header.Height - 2
			}
		}
	}
}

func (s *Submitter) syncHeaderBatchLoop(ch <-chan msg.Header, reset chan<- uint64) {
	headers := [][]byte{}
	commit := false
	duration := time.Duration(s.config.Timeout) * time.Second
	var (
		height uint64
		hdr    *msg.Header
	)

COMMIT:
	for {
		select {
		case <-s.Done():
			break COMMIT
		case header, ok := <-ch:
			if ok {
				hdr = &header
				height = header.Height
				if hdr.Data == nil {
					// Update header sync height
					commit = true
				} else {
					headers = append(headers, header.Data)
					commit = len(headers) >= s.config.Batch
				}
			} else {
				commit = len(headers) > 0
				break COMMIT
			}
		case <-time.After(duration):
			commit = len(headers) > 0
		}
		if commit {
			commit = false
			// NOTE err reponse here will revert header sync with delta -100
			err := s.SubmitHeadersWithLoop(s.config.ChainId, headers, hdr)
			if err != nil {
				reset <- height - uint64(len(headers)) - 2
			}
			headers = [][]byte{}
		}
	}
	if len(headers) > 0 {
		s.SubmitHeadersWithLoop(s.config.ChainId, headers, hdr)
	}
}

func (s *Submitter) startSync(ch <-chan msg.Header, reset chan<- uint64) {
	if s.config.Batch == 1 {
		s.syncHeaderLoop(ch, reset)
	} else {
		s.syncHeaderBatchLoop(ch, reset)
	}
	log.Info("Header sync exiting loop now")
}

func (s *Submitter) Peer() *eth.SDK {
	return s.sdk
}
