package relayer

import (
	"context"
	"sync"
	"time"

	"github.com/polynetwork/bridge-common/chains"
	"github.com/polynetwork/bridge-common/chains/bridge"
	ethcommon "github.com/polynetwork/bridge-common/chains/eth"
	"github.com/top/top-relayer/base"
	"github.com/top/top-relayer/config"
	"github.com/top/top-relayer/msg"
	"github.com/top/top-relayer/relayer/eth"
	"github.com/top/top-relayer/relayer/top"
)

type IChainListener interface {
	Init(*config.HeaderSyncConfig, *ethcommon.SDK) error
	Defer() int
	ListenCheck() time.Duration
	ChainId() uint64
	Nodes() chains.Nodes
	Header(height uint64) (header []byte, hash []byte, err error)
	LastHeaderSync(uint64, uint64) (uint64, error)
	LatestHeight() (uint64, error)
}

type Handler interface {
	Init(context.Context, *sync.WaitGroup) error
	Chain() uint64
	Start() error
	Stop() error
}

type IChainSubmitter interface {
	Init(*config.HeaderSyncConfig) error
	Hook(context.Context, *sync.WaitGroup, <-chan msg.Message) error
	Stop() error
	SDK() *ethcommon.SDK
	GetSideChainHeader(chainId, height uint64) (hash []byte, err error)
	GetSideChainHeight(chainId uint64) (height uint64, err error)
	StartSync(ctx context.Context, wg *sync.WaitGroup, reset chan<- uint64) (ch chan msg.Header, err error)
}

func GetListener(chain uint64) (listener IChainListener) {
	switch chain {
	case base.ETH, base.BSC:
		listener = new(eth.Listener)
	case base.TOP:
		listener = new(top.Listener)
	default:
	}
	return
}

func GetSubmitter(chain uint64) (submitter IChainSubmitter) {
	switch chain {
	case base.ETH, base.BSC:
		submitter = new(eth.Submitter)
	case base.TOP:
		submitter = new(top.Submitter)
	default:
	}
	return
}

// func ChainSubmitter(chain uint64) (sub IChainSubmitter, err error) {
// 	sub = GetSubmitter(chain)
// 	if sub == nil {
// 		err = fmt.Errorf("No submitter for chain %d available", chain)
// 		return
// 	}
// 	conf := config.CONFIG.Chains[chain]
// 	if conf == nil {
// 		return nil, fmt.Errorf("No config available for submitter of chain %d", chain)
// 	}
// 	err = sub.Init(conf.HeaderSync[1])
// 	return
// }

// func ChainListener(chain uint64, poly *poly.SDK) (l IChainListener, err error) {
// 	l = GetListener(chain)
// 	if l == nil {
// 		err = fmt.Errorf("No listener for chain %d available", chain)
// 		return
// 	}
// 	conf := config.CONFIG.Chains[chain]
// 	if conf == nil {
// 		return nil, fmt.Errorf("No config available for listener of chain %d", chain)
// 	}

// 	err = l.Init(conf.HeaderSync[1], poly)
// 	return
// }

func Bridge() (sdk *bridge.SDK, err error) {
	return bridge.WithOptions(0, config.CONFIG.Bridge, time.Minute, 100)
}
