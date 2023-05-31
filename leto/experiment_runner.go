package main

import (
	"os"
	"os/exec"

	"github.com/formicidae-tracker/leto/letopb"
)

type ExperimentRunner interface {
	Run() (*letopb.ExperimentLog, error)
}

type slaveRunner struct {
	env        *TrackingEnvironment
	artemisCmd *exec.Cmd
}

func NewExperimentRunner(env *TrackingEnvironment) (ExperimentRunner, error) {
	if env.Node.IsMaster() == true {
		return newMasterRunner(env)
	}
	return newSlaveRunner(env)
}

func newSlaveRunner(env *TrackingEnvironment) (ExperimentRunner, error) {
	res := &slaveRunner{
		env: env,
	}
	var err error
	res.artemisCmd, err = env.SetUp()
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *slaveRunner) Run() (log *letopb.ExperimentLog, err error) {
	defer func() {
		var terr error
		log, terr = r.env.TearDown(err != nil)
		if err == nil {
			err = terr
		}
	}()
	go func() {
		<-r.env.Context.Done()
		r.artemisCmd.Process.Signal(os.Interrupt)
	}()
	return nil, r.artemisCmd.Run()
}
