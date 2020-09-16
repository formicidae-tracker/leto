package main

import (
	"fmt"
	"time"

	"github.com/formicidae-tracker/leto"
	"github.com/google/uuid"
)

type NodeCurrentExperimentStatus struct {
	Since  time.Time
	Config leto.TrackingConfiguration
}

type NodeExperimentLocalLogEntry struct {
	Start   time.Time
	End     *time.Time
	Name    string
	Config  *leto.TrackingConfiguration
	LogData []byte
}

type Node struct {
	Name   string   `yaml:"name"`
	Master string   `yaml:"master"`
	Slaves []string `yaml:"slaves"`

	CurrentExperiment *NodeCurrentExperimentStatus
	LocalLog          []NodeExperimentLocalLogEntry
}

func (n Node) IsMaster() bool {
	return len(n.Master) != 0
}

func (c Node) GenerateLoadBalancing() *leto.LoadBalancingConfiguration {
	if len(c.Slaves) == 0 {
		return &leto.LoadBalancingConfiguration{
			SelfUUID:     "single-node",
			Master:       c.Name,
			UUIDs:        map[string]string{c.Name: "single-node"},
			Assignements: map[int]string{0: "single-node"},
		}
	}
	res := &leto.LoadBalancingConfiguration{
		SelfUUID:     uuid.New().String(),
		Master:       c.Name,
		UUIDs:        make(map[string]string),
		Assignements: make(map[int]string),
	}
	res.UUIDs["localhost"] = res.SelfUUID
	res.Assignements[0] = res.SelfUUID
	for i, s := range c.Slaves {
		uuid := uuid.New().String()
		res.UUIDs[s] = uuid
		res.Assignements[i+1] = uuid
	}
	return res
}

func DisplayExperimentName(config leto.TrackingConfiguration) string {
	if len(config.ExperimentName) == 0 {
		return "TEST-MODE"
	}
	return config.ExperimentName
}

func (n *Node) StartExperiment(userConfig *leto.TrackingConfiguration) error {
	if n.CurrentExperiment != nil {
		return fmt.Errorf("Node '%s' is currently running an experiment '%s' since %s", n.Name,
			DisplayExperimentName(n.CurrentExperiment.Config),
			n.CurrentExperiment.Since)
	}

	defaultConfig := leto.LoadDefaultTrackingConfiguration()

	return nil
}
