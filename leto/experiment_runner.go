package main

import (
	"errors"
	"os/exec"

	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/leto/letopb"
)

type ExperimentRunner interface {
	Run() (*letopb.ExperimentLog, error)
}

type masterExperimentRunner struct {
}

func NewExperimentRunner(
	nodeConfig NodeConfiguration,
	experimentConfig *leto.TrackingConfiguration,
) (ExperimentRunner, error) {
	if nodeConfig.IsMaster() == false {
		return newSlaveExperimentRunner(nodeConfig, experimentConfig)
	}
	return newMasterExperimentRunner(nodeConfig, experimentConfig)
}

func newSlaveExperimentRunner(nodeConfig NodeConfiguration,
	experimentConfig *leto.TrackingConfiguration,
) (ExperimentRunner, error) {
	return &slaveExperimentRunner{}, nil
}

func newMasterExperimentRunner(nodeConfig NodeConfiguration,
	experimentConfig *leto.TrackingConfiguration,
) (ExperimentRunner, error) {
	return nil, errors.New("not implemented")
}

type slaveExperimentRunner struct {
	artemisCmd *exec.Cmd
}

func (r *slaveExperimentRunner) Run() (*letopb.ExperimentLog, error) {
	return nil, nil
}
