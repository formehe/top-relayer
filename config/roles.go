package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/top/top-relayer/base"
)

type Role struct {
	HeaderSync bool // header sync
}

type Roles map[uint64]Role

func (c *Config) ReadRoles(path string) (err error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Read roles file error %v", err)
	}
	roles := Roles{}
	err = json.Unmarshal(data, &roles)
	if err != nil {
		return fmt.Errorf("Parse roles file error %v", err)
	}
	c.ApplyRoles(roles)
	return
}

func (c *Config) ApplyRoles(roles Roles) {
	for id, role := range roles {
		c.chains[id] = true
		if id == base.TOP {
			if c.Top == nil {
				c.Top = new(TopChainConfig)
			}
		} else {
			chain, ok := c.Chains[id]
			if !ok {
				chain = new(ChainConfig)
				c.Chains[id] = chain
			}

			chain.HeaderSync[0].Enabled = role.HeaderSync
			chain.HeaderSync[1].Enabled = role.HeaderSync
		}
	}
}
