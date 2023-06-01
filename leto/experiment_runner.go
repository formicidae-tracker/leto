package main

import (
	"log"
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
	res.artemisCmd, err = env.SetUp(env.Context)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func WaitDoneOrKill(cmd *exec.Cmd, done <-chan struct{}, grace time.Duration, logger *log.Logger, name string) bool {

	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
	}
	logger.Printf("killing %s as it did not terminate after %s", name, grace)
	if err := cmd.Process.Kill(); err != nil {
		logger.Printf("could not kill %s: %s", name, err)
	}
	return false

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
		WaitDoneOrKill(r.artemisCmd, done, 500*time.Millisecond, r.logger, "artemis")
	}()

	r.logger.Printf("started")
	defer r.logger.Printf("done")
	return nil, r.artemisCmd.Run()
}
