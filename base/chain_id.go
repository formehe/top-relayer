package base

import "fmt"

const (
	TOP uint64 = 0
	ETH uint64 = 1
	BSC uint64 = 2

	ENV = "mainnet"
)

func GetChainName(id uint64) string {
	switch id {
	case TOP:
		return "Top"
	case ETH:
		return "Ethereum"
	case BSC:
		return "Bsc"
	default:
		return fmt.Sprintf("Unknown(%d)", id)
	}
}

func BlocksToSkip(chainId uint64) uint64 {
	switch chainId {
	case ETH:
		return 8
	case BSC:
		return 17
	default:
		return 1
	}
}

func BlocksToWait(chainId uint64) uint64 {
	switch chainId {
	case ETH:
		return 12
	case BSC:
		return 21
	default:
		return 100000000
	}
}

var CHAINS = []uint64{
	TOP, ETH, BSC,
}
