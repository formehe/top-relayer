package top

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polynetwork/bridge-common/chains"
	ethcommon "github.com/polynetwork/bridge-common/chains/eth"
	"github.com/polynetwork/bridge-common/log"
	"github.com/top/top-relayer/abi/bridge"
	"github.com/top/top-relayer/base"
	"github.com/top/top-relayer/config"
)

type Listener struct {
	sdk        *ethcommon.SDK
	peer       *ethcommon.SDK
	hscontract common.Address
	config     *config.HeaderSyncConfig
	name       string
}

func (l *Listener) Init(config *config.HeaderSyncConfig, peerSdk *ethcommon.SDK) (err error) {
	if config.ChainId != base.TOP {
		return fmt.Errorf("expect chain id is TOP, but real chain id is %d", config.ChainId)
	}

	l.config = config
	l.sdk, err = ethcommon.WithOptions(config.ChainId, config.Nodes, time.Minute, 1)
	if err != nil {
		return fmt.Errorf("fail to init sdk, err is %s", err.Error())
	}

	l.hscontract = common.HexToAddress(config.Submitter.HSContract)

	l.peer = peerSdk
	return nil
}

func (l *Listener) ChainId() uint64 {
	return base.TOP
}

func (l *Listener) Defer() int {
	return 1
}

func (l *Listener) Header(height uint64) (header []byte, hash []byte, err error) {
	hdr, err := l.sdk.Node().HeaderByNumber(context.Background(), big.NewInt(int64(height)))
	if err != nil {
		err = fmt.Errorf("Fetch block header error %v", err)
		return nil, nil, err
	}
	log.Info("Fetched block header", "chain", l.name, "height", height, "hash", hdr.Hash().String())
	hash = hdr.Hash().Bytes()
	header, err = hdr.MarshalJSON()
	return
}

func (l *Listener) ListenCheck() time.Duration {
	duration := time.Second
	if l.config.ListenCheck > 0 {
		duration = time.Duration(l.config.ListenCheck) * time.Second
	}
	return duration
}

func (l *Listener) Nodes() chains.Nodes {
	return l.sdk.ChainSDK
}

func (l *Listener) LastHeaderSync(force, last uint64) (height uint64, err error) {
	if l.peer == nil {
		err = fmt.Errorf("No poly sdk provided for listener", "chain", l.name)
		return
	}

	if force != 0 {
		return force, nil
	}

	return l.getSideChainHeight(l.config.ChainId)
}

//todo
func (l *Listener) getSideChainHeight(chainId uint64) (height uint64, err error) {
	hscaller, err := bridge.NewBridgeCaller(l.hscontract, l.sdk.Node())
	if err != nil {
		return 0, fmt.Errorf("Proccess: fail to get side chain height by chain id %d", chainId)
	}

	height, err = hscaller.GetMaxHeight(nil)
	return
}

func (l *Listener) LatestHeight() (uint64, error) {
	return l.sdk.Node().GetLatestHeight()
}
