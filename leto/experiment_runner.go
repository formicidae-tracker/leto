package main

import (
	"errors"
	"os/exec"

	"github.com/formicidae-tracker/leto/letopb"
)

type ExperimentRunner interface {
	Run() (*letopb.ExperimentLog, error)
}

type masterExperimentRunner struct {
}

func NewExperimentRunner(config ExperimentConfiguration) (ExperimentRunner, error) {
	if config.Node.IsMaster() == false {
		return newSlaveExperimentRunner(config)
	}
	return newMasterExperimentRunner(config)
}

func newSlaveExperimentRunner(config ExperimentConfiguration) (ExperimentRunner, error) {
	return &slaveExperimentRunner{}, nil
}

func newMasterExperimentRunner(config ExperimentConfiguration) (ExperimentRunner, error) {
	return nil, errors.New("not implemented")
}

type slaveExperimentRunner struct {
	artemisCmd *exec.Cmd
}

func (r *slaveExperimentRunner) Run() (*letopb.ExperimentLog, error) {
	return nil, nil
}
