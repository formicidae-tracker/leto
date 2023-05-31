package main

import (
	"os"
	"os/exec"

	"github.com/formicidae-tracker/leto/letopb"
)

type masterExperimentRunner struct {
	env *TrackingEnvironment

	artemisCmd *exec.Cmd

	artemisListener ArtemisListener
}

func newMasterExperimentRunner(env *TrackingEnvironment) (ExperimentRunner, error) {
	res := &masterExperimentRunner{
		env: env,
	}
	if err := res.SetUp(); err != nil {
		return nil, err
	}

	return res, nil
}

func (r *masterExperimentRunner) SetUp() error {

	var err error
	r.artemisListener, err = NewArtemisListener(r.env.Context, r.env.Leto.ArtemisIncomingPort)
	if err != nil {
		return err
	}

	r.hermes

}

func (r *masterExperimentRunner) Run() (log *letopb.ExperimentLog, err error) {
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

	err = r.startTasks()
	if err != nil {
		return nil, err
	}

	r.waitCriticalTask()

	r.stopAllTask()

	return nil, r.waitAllTask()
}
