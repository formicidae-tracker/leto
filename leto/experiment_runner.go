package main

import (
	"os"
	"os/exec"

	"github.com/formicidae-tracker/leto/letopb"
)

type ExperimentRunner interface {
	Run() (*letopb.ExperimentLog, error)
	Stop()
}

type slaveExperimentRunner struct {
	env        *TrackingEnvironment
	artemisCmd *exec.Cmd
}

func NewExperimentRunner(env *TrackingEnvironment) (ExperimentRunner, error) {
	if env.Node.IsMaster() == true {
		return newMasterExperimentRunner(env)
	}
	return newSlaveExperimentRunner(env)
}

func newSlaveExperimentRunner(env *TrackingEnvironment) (ExperimentRunner, error) {
	res := &slaveExperimentRunner{
		env: env,
	}
	var err error
	res.artemisCmd, err = env.SetUp()
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *slaveExperimentRunner) Run() (log *letopb.ExperimentLog, err error) {
	defer func() {
		var terr error
		log, terr = r.env.TearDown(err != nil)
		if err == nil {
			err = terr
		}
	}()
	return nil, r.artemisCmd.Run()
}

func (r *slaveExperimentRunner) Stop() {
	r.artemisCmd.Process.Signal(os.Interrupt)
}
