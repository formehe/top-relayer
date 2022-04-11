package eth

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/polynetwork/bridge-common/chains"
	"github.com/polynetwork/bridge-common/chains/eth"
	ethcommon "github.com/polynetwork/bridge-common/chains/eth"
	"github.com/polynetwork/bridge-common/log"
	"github.com/top/top-relayer/abi/hsc"
	"github.com/top/top-relayer/base"
	"github.com/top/top-relayer/config"
)

type Listener struct {
	sdk        *eth.SDK
	peer       *eth.SDK
	hsContract common.Address
	config     *config.HeaderSyncConfig
	name       string
}

func (l *Listener) Init(config *config.HeaderSyncConfig, peerSdk *ethcommon.SDK) (err error) {
	l.config = config
	l.name = base.GetChainName(config.ChainId)
	l.hsContract = common.HexToAddress(config.Submitter.HSContract)
	l.peer = peerSdk
	l.sdk, err = eth.WithOptions(config.ChainId, config.Nodes, time.Minute, 1)
	return
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

func (l *Listener) ChainId() uint64 {
	return l.config.ChainId
}

func (l *Listener) Defer() int {
	return l.config.Defer
}

func (l *Listener) Name() string {
	return l.name
}

func (l *Listener) SDK() *eth.SDK {
	return l.sdk
}

func (l *Listener) LatestHeight() (uint64, error) {
	return l.sdk.Node().GetLatestHeight()
}

//todo
func (l *Listener) getSideChainHeight(chainId uint64) (height uint64, err error) {
	hscaller, err := hsc.NewHscCaller(l.hsContract, l.peer.Node())
	if err != nil {
		return 0, err
	}

	hscRaw := hsc.HscCallerRaw{Contract: hscaller}
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
