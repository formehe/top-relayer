package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/polynetwork/bridge-common/util"
	"github.com/polynetwork/bridge-common/wallet"
	"github.com/top/top-relayer/base"
)

var (
	CONFIG      *Config
	WALLET_PATH string
	CONFIG_PATH string
)

type Config struct {
	Env    string
	Top    *TopChainConfig
	Chains map[uint64]*ChainConfig

	// Http
	Host string
	Port int

	ValidMethods []string
	validMethods map[string]bool
	chains       map[uint64]bool
	Bridge       []string
}

// Parse file path, if path is empty, use config file directory path
func GetConfigPath(path, file string) string {
	if path == "" {
		path = filepath.Dir(CONFIG_PATH)
	}
	return filepath.Join(path, file)
}

func New(path string) (config *Config, err error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Read config file error %v", err)
	}
	config = &Config{chains: map[uint64]bool{}}
	err = json.Unmarshal(data, config)
	if err != nil {
		return nil, fmt.Errorf("Parse config file error %v", err)
	}
	if config.Env != base.ENV {
		util.Fatal("Config env(%s) and build env(%s) does not match!", config.Env, base.ENV)
	}

	methods := map[string]bool{}
	for _, m := range config.ValidMethods {
		methods[m] = true
	}

	if config.Chains == nil {
		config.Chains = map[uint64]*ChainConfig{}
	}
	config.validMethods = methods
	return
}

type ChainConfig struct {
	ChainId     uint64
	Nodes       []string
	ExtraNodes  []string
	HSContract  string
	ListenCheck int
	CheckFee    bool
	Defer       int
	Wallet      *wallet.Config
	HeaderSync  [2]*HeaderSyncConfig // 0:chain -> ch -> top; 1: top -> ch -> chain
}

type ListenerConfig struct {
	ChainId     uint64
	Nodes       []string
	ExtraNodes  []string
	ListenCheck int
	Defer       int
}

type SubmitterConfig struct {
	ChainId    uint64
	Nodes      []string
	ExtraNodes []string
	HSContract string
	Wallet     *wallet.Config
}

type TopChainConfig struct {
	ChainId    uint64
	Nodes      []string
	ExtraNodes []string
	HSContract string
	Wallet     *wallet.Config
}

func (c *TopChainConfig) Fill(o *TopChainConfig) *TopChainConfig {
	if o == nil {
		o = new(TopChainConfig)
	}
	o.ChainId = base.TOP
	if len(o.Nodes) == 0 {
		o.Nodes = c.Nodes
	}
	if o.Wallet == nil {
		o.Wallet = c.Wallet
	} else {
		o.Wallet.Path = GetConfigPath(WALLET_PATH, o.Wallet.Path)
	}
	return o
}

type WalletConfig struct {
	Nodes    []string
	KeyStore string
	KeyPwd   map[string]string
}

type HeaderSyncConfig struct {
	Batch     int
	Timeout   int
	Buffer    int
	Enabled   bool
	Submitter *SubmitterConfig
	*ListenerConfig
}

func (c *Config) Active(chain uint64) bool {
	return c.chains[chain]
}

func (c *Config) Init() (err error) {
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 6500
	}

	if c.Top != nil {
		err = c.Top.Init()
		if err != nil {
			return
		}
	}

	for chain, conf := range c.Chains {
		err = conf.Init(chain, c.Top)
		if err != nil {
			return
		}
	}

	CONFIG = c
	return
}

func (c *Config) AllowMethod(method string) bool {
	return c.validMethods[method]
}

func (c *TopChainConfig) Init() (err error) {
	c.ChainId = base.TOP
	if c.Wallet != nil {
		c.Wallet.Path = GetConfigPath(WALLET_PATH, c.Wallet.Path)
	}

	return
}

func (c *TopChainConfig) FillListener(o *ListenerConfig) *ListenerConfig {
	if o == nil {
		o = new(ListenerConfig)
	}

	o.ChainId = c.ChainId
	if len(o.Nodes) == 0 {
		o.Nodes = c.Nodes
	}
	if len(o.ExtraNodes) == 0 {
		o.ExtraNodes = c.ExtraNodes
	}

	// if o.Defer == 0 {
	// 	o.Defer = c.Defer
	// }

	// if o.ListenCheck == 0 {
	// 	o.ListenCheck = c.ListenCheck
	// }

	return o
}

func (c *TopChainConfig) FillSubmitter(o *SubmitterConfig) *SubmitterConfig {
	if o == nil {
		o = new(SubmitterConfig)
	}

	o.ChainId = c.ChainId
	if len(o.Nodes) == 0 {
		o.Nodes = c.Nodes
	}
	if len(o.ExtraNodes) == 0 {
		o.ExtraNodes = c.ExtraNodes
	}

	if o.Wallet == nil {
		o.Wallet = c.Wallet
	} else {
		o.Wallet.Path = GetConfigPath(WALLET_PATH, o.Wallet.Path)
		if len(o.Wallet.Nodes) == 0 {
			o.Wallet.Nodes = c.Wallet.Nodes
		}
		for _, p := range o.Wallet.KeyStoreProviders {
			p.Path = GetConfigPath(WALLET_PATH, p.Path)
		}
	}

	if o.HSContract == "" {
		o.HSContract = c.HSContract
	}
	return o
}

func (c *ChainConfig) Init(chain uint64, top *TopChainConfig) (err error) {
	if c.ChainId != 0 && c.ChainId != chain {
		err = fmt.Errorf("Conflict chain id in config %d <> %d", c.ChainId, chain)
		return
	}
	c.ChainId = chain
	if c.Wallet != nil {
		if len(c.Wallet.Nodes) == 0 {
			c.Wallet.Nodes = c.Nodes
		}
		c.Wallet.Path = GetConfigPath(WALLET_PATH, c.Wallet.Path)
		for _, p := range c.Wallet.KeyStoreProviders {
			p.Path = GetConfigPath(WALLET_PATH, p.Path)
		}
	}

	if c.HeaderSync[0] == nil {
		c.HeaderSync[0].ListenerConfig = c.FillListener(c.HeaderSync[0].ListenerConfig)
		c.HeaderSync[0].ChainId = chain
		c.HeaderSync[0].Submitter = top.FillSubmitter(c.HeaderSync[0].Submitter)
	}

	if c.HeaderSync[1] == nil {
		c.HeaderSync[1].ListenerConfig = top.FillListener(c.HeaderSync[1].ListenerConfig)
		c.HeaderSync[1].ChainId = base.TOP
		c.HeaderSync[1].Submitter = c.FillSubmitter(c.HeaderSync[1].Submitter)
	}

	return
}

func (c *ChainConfig) FillSubmitter(o *SubmitterConfig) *SubmitterConfig {
	if o == nil {
		o = new(SubmitterConfig)
	}
	if o.ChainId != 0 && c.ChainId != o.ChainId {
		util.Fatal("Conflict chain id in config for submitters %d <> %d", o.ChainId, c.ChainId)
	}
	o.ChainId = c.ChainId
	if len(o.Nodes) == 0 {
		o.Nodes = c.Nodes
	}
	if len(o.ExtraNodes) == 0 {
		o.ExtraNodes = c.ExtraNodes
	}
	if o.Wallet == nil {
		o.Wallet = c.Wallet
	} else {
		o.Wallet.Path = GetConfigPath(WALLET_PATH, o.Wallet.Path)
		if len(o.Wallet.Nodes) == 0 {
			o.Wallet.Nodes = c.Wallet.Nodes
		}
		for _, p := range o.Wallet.KeyStoreProviders {
			p.Path = GetConfigPath(WALLET_PATH, p.Path)
		}
	}

	if o.HSContract == "" {
		o.HSContract = c.HSContract
	}

	return o
}

func (c *ChainConfig) FillListener(o *ListenerConfig) *ListenerConfig {
	if o == nil {
		o = new(ListenerConfig)
	}
	if o.ChainId != 0 && c.ChainId != o.ChainId {
		util.Fatal("Conflict chain id in config for listeners %d <> %d", o.ChainId, c.ChainId)
	}
	o.ChainId = c.ChainId
	if len(o.Nodes) == 0 {
		o.Nodes = c.Nodes
	}
	if len(o.ExtraNodes) == 0 {
		o.ExtraNodes = c.ExtraNodes
	}
	if o.Defer == 0 {
		o.Defer = c.Defer
	}

	if o.ListenCheck == 0 {
		o.ListenCheck = c.ListenCheck
	}

	return o
}
