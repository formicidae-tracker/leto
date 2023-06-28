package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v2"
)

type NodeConfiguration struct {
	Master string   `yaml:"master"`
	Slaves []string `yaml:"slaves"`
}

func localConfigPath() (string, error) {
	return xdg.ConfigFile("FORmicidae Tracker/leto.yml")
}

var defaultNodeConfiguration NodeConfiguration = NodeConfiguration{
	Master: "",
	Slaves: nil,
}

func GetNodeConfiguration() NodeConfiguration {
	confPath, err := localConfigPath()
	if err != nil {
		return defaultNodeConfiguration
	}

	conf, err := os.Open(confPath)
	if err != nil {
		return defaultNodeConfiguration
	}
	defer conf.Close()
	txt, err := ioutil.ReadAll(conf)
	if err != nil {
		return defaultNodeConfiguration
	}

	res := defaultNodeConfiguration

	err = yaml.Unmarshal(txt, &res)
	if err != nil {
		return defaultNodeConfiguration
	}

	return res
}

func (c NodeConfiguration) Save() {
	confPath, err := localConfigPath()
	if err != nil {
		return
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return
	}
	ioutil.WriteFile(confPath, data, 0644)
}

func (c NodeConfiguration) IsMaster() bool {
	return len(c.Master) == 0
}

func (c *NodeConfiguration) AddSlave(hostname string) error {
	slaves := make(map[string]int, len(c.Slaves))
	for i, s := range c.Slaves {
		slaves[s] = i
	}
	if _, ok := slaves[hostname]; ok == true {
		return fmt.Errorf("NodeConfiguration: already has slave %s", hostname)
	}
	c.Slaves = append(c.Slaves, hostname)
	return nil
}

func (c *NodeConfiguration) RemoveSlave(hostname string) error {
	slaves := make(map[string]int, len(c.Slaves))
	for i, s := range c.Slaves {
		slaves[s] = i
	}
	idx, ok := slaves[hostname]
	if ok == false {
		return fmt.Errorf("NodeConfiguration: does not have slave %s (%s)", hostname, c.Slaves)
	}
	c.Slaves = append(c.Slaves[:idx], c.Slaves[idx+1:]...)
	return nil
}
