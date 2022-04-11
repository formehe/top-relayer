package relayer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/polynetwork/bridge-common/chains/bridge"
	"github.com/polynetwork/bridge-common/log"
	"github.com/top/top-relayer/msg"
)

var (
	BIN_DIR = "."
)

func init() {
	dir := os.Getenv("RELAYER_BIN")
	if len(dir) == 0 {
		dir = "."
	}
	BIN_DIR, _ = filepath.Abs(dir)
}

func Bin(chainId uint64, hash string) (bin string, err error) {
	// if chainId == base.TOP {
	// 	listener := GetListener(base.TOP)
	// 	height, err := listener.GetTxBlock(hash)
	// 	if err != nil {
	// 		return "", err
	// 	}
	// 	txs, err := listener.Scan(height)
	// 	if err != nil {
	// 		log.Error("Fetch block txs error", "height", height, "err", err)
	// 		return "", err
	// 	}

	// 	for _, tx := range txs {
	// 		if util.LowerHex(hash) == util.LowerHex(tx.PolyHash) {
	// 			log.Info("Found patch target tx", "hash", hash, "height", height)
	// 			chainId = tx.DstChainId
	// 		}
	// 	}
	// }
	// bin = "relayer_main"
	// bin = path.Join(BIN_DIR, bin)
	return
}

func Relay(tx *msg.Tx) {
	chain := tx.SrcChainId
	hash := tx.SrcHash
	if len(tx.PolyHash) > 0 {
		hash = tx.PolyHash
	}
	bin, err := Bin(chain, hash)
	if len(bin) == 0 {
		log.Error("Failed to find relayer bin", "chain", chain, "hash", hash, "err", err)
		return
	}
	config := os.Getenv("RELAYER_CONFIG")
	if len(config) == 0 {
		config = "config.json"
	}
	args := []string{
		"-config", config,
		"submit",
		"-hash", hash, "-chain", strconv.Itoa(int(chain)),
		"-price", tx.DstGasPrice, "-pricex", tx.DstGasPriceX, "-limit", strconv.Itoa(int(tx.DstGasLimit)),
	}
	if tx.SkipCheckFee {
		args = append(args, "-free")
	}
	if tx.DstSender != nil {
		args = append(args, "-sender", tx.DstSender.(string))
	}
	cmd := exec.Command(bin, args...)
	log.Info(fmt.Sprintf("Executing auto patch %v: %v", bin, args))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	cmd.Start()
	done := make(chan bool)
	go func() {
		log.Error("Command executed", "err", cmd.Wait())
		close(done)
	}()
	select {
	case <-done:
		log.Info("Relay tx executed", "chain", chain, "hash", hash)
	case <-time.After(40 * time.Second):
		log.Error("Failed to relay tx for a timeout", "chain", chain, "hash", hash)
	}
	cmd.Process.Kill()
	return
}

func AutoPatch() (err error) {
	timer := time.NewTicker(2 * time.Minute)
	for range timer.C {
		log.Info("Auto patch ticking")
		txs := []*msg.Tx{}
		for i, tx := range txs {
			log.Info(fmt.Sprintf("Auto patching %d/%d", i, len(txs)))
			Relay(tx)
		}
	}
	return
}

func CheckFee(sdk *bridge.SDK, tx *msg.Tx) (res *bridge.CheckFeeRequest, err error) {
	state := map[string]*bridge.CheckFeeRequest{}
	state[tx.PolyHash] = &bridge.CheckFeeRequest{
		ChainId:  tx.SrcChainId,
		TxId:     tx.TxId,
		PolyHash: tx.PolyHash,
	}
	err = sdk.Node().CheckFee(state)
	if err != nil {
		return
	}
	if state[tx.PolyHash] == nil {
		state[tx.PolyHash] = new(bridge.CheckFeeRequest)
	}
	return state[tx.PolyHash], nil
}
