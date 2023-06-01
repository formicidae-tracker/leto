package main

import (
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/formicidae-tracker/leto/letopb"
)

type ExperimentRunner interface {
	Run() (*letopb.ExperimentLog, error)
}

type slaveRunner struct {
	env        *TrackingEnvironment
	artemisCmd *exec.Cmd
	logger     *log.Logger
}

func NewExperimentRunner(env *TrackingEnvironment) (ExperimentRunner, error) {
	if env.Node.IsMaster() == true {
		return newMasterRunner(env)
	}
	return newSlaveRunner(env)
}

func newSlaveRunner(env *TrackingEnvironment) (ExperimentRunner, error) {
	res := &slaveRunner{
		env:    env,
		logger: NewLogger("experiment-runner"),
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
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-done:
			return
		case <-r.env.Context.Done():
		}

		r.artemisCmd.Process.Signal(os.Interrupt)

		grace := 500 * time.Millisecond
		timer := time.NewTimer(grace)
		defer timer.Stop()

		select {
		case <-done:
			return
		case <-timer.C:
		}
		r.logger.Printf("killing artemis as it did not terminate after %s", grace)
		if err := r.artemisCmd.Process.Kill(); err != nil {
			r.logger.Printf("could not kill artemis: %s", err)
		}

	}()
	r.logger.Printf("started")

	defer r.logger.Printf("done")
	return nil, r.artemisCmd.Run()
}
