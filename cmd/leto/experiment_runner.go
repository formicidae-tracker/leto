package main

import (
	"os"
	"os/exec"
	"time"

	"github.com/formicidae-tracker/leto/pkg/letopb"
	"github.com/sirupsen/logrus"
)

type ExperimentRunner interface {
	Run() (*letopb.ExperimentLog, error)
}

type slaveRunner struct {
	env        *TrackingEnvironment
	artemisCmd *exec.Cmd
	logger     *logrus.Entry
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

func WaitDoneOrFunc(done <-chan struct{}, grace time.Duration, f func(time.Duration)) bool {

	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
	}
	f(grace)
	return false

}

func (r *slaveRunner) Run() (log *letopb.ExperimentLog, err error) {
	defer func() {
		var terr error
		log, terr = r.env.TearDown(err)
		if err == nil {
			err = terr
		}
	}()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
			// here artemis may simply have crashed before env.Context
			// was canceled. Therefore we simply return to avoid to
			// leak the go routine.
			return
		case <-r.env.Context.Done():
			// we nicely ask artemis to interrupt (he will eventually
			// close after finishing processing its current frame.
			r.artemisCmd.Process.Signal(os.Interrupt)
		}

		// we ensure that we kill artemis if it does not comply
		for !WaitDoneOrFunc(done, 500*time.Millisecond, func(grace time.Duration) {
			r.logger.Warnf("killing artemis as it did not exit after %s", grace)
			if err := r.artemisCmd.Process.Kill(); err != nil {
				r.logger.WithField("error", err).Errorf("could not kill artemis")
			}
		}) {
		}
	}()

	r.logger.Infof("started")
	defer r.logger.Infof("done")
	return nil, r.artemisCmd.Run()
}
