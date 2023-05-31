package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"reflect"
	"sync"
	"time"

	"github.com/formicidae-tracker/leto/letopb"
)

type masterExperimentRunner struct {
	env *TrackingEnvironment

	artemisCmd *exec.Cmd

	artemisListener   ArtemisListener
	hermesBroadcaster HermesBroadcaster
	fileWriter        HermesFileWriter
	video             VideoManager
	dispatcher        FrameDispatcher

	ctx    context.Context
	cancel context.CancelFunc

	artemisOut *io.PipeWriter
	videoIn    *io.PipeReader

	subtasks map[string]<-chan error
	logger   *log.Logger
}

func newMasterExperimentRunner(env *TrackingEnvironment) (ExperimentRunner, error) {
	ctx, cancel := context.WithCancel(env.Context)
	res := &masterExperimentRunner{
		env:      env,
		subtasks: make(map[string]<-chan error),
		ctx:      ctx,
		cancel:   cancel,
		logger:   NewLogger("experiment-runner"),
	}
	if err := res.SetUp(); err != nil {
		return nil, err
	}

	return res, nil
}

func (r *masterExperimentRunner) SetUp() error {
	var err error
	r.artemisListener, err = NewArtemisListener(r.ctx, r.env.Leto.ArtemisIncomingPort)
	if err != nil {
		return err
	}

	r.hermesBroadcaster, err = NewHermesBroadcaster(r.ctx,
		r.env.Leto.HermesBroadcastPort,
		time.Duration(3.0*float64(time.Second)/(*r.env.Config.Camera.FPS)),
	)
	if err != nil {
		return err
	}

	r.fileWriter, err = NewFrameReadoutWriter(r.env.Path("tracking.hermes"))
	if err != nil {
		return err
	}

	r.dispatcher = NewFrameDispatcher(r.fileWriter.Incoming(), r.hermesBroadcaster.Incoming())

	r.video, err = NewVideoManager(r.env.ExperimentDir, *r.env.Config.Camera.FPS, r.env.Config.Stream)
	if err != nil {
		return err
	}

	r.videoIn, r.artemisOut = io.Pipe()

	r.artemisCmd, err = r.env.SetUp()
	r.artemisCmd.Stdout = r.artemisOut

	return err
}

func (r *masterExperimentRunner) Run() (log *letopb.ExperimentLog, err error) {
	defer func() {
		var terr error
		log, terr = r.env.TearDown(err != nil)
		if err == nil {
			err = terr
		}
	}()

	r.startSubtasks()
	go func() {
		<-r.env.Context.Done()
		r.artemisCmd.Process.Signal(os.Interrupt)
	}()

	err = r.waitAnyCriticalSubtask()

	r.stopAllSubtask()

	r.waitAllSubtask()

	return nil, err
}

func (r *masterExperimentRunner) startSubtasks() {
	r.startSubtask(r.artemisListener, "artemis-in")
	r.startSubtaskFunction(r.mergeFrames(), "frame-merger")
	r.startSubtask(r.dispatcher, "frame-dispatcher")
	r.startSubtask(r.fileWriter, "writer")
	r.startSubtask(r.hermesBroadcaster, "broadcaster")
	r.startSubtaskFunction(func() error {
		return r.video.Run(r.videoIn)
	}, "video")
	r.startSubtaskFunction(func() error {
		defer func() {
			err := r.artemisOut.Close()
			if err != nil {
				r.logger.Printf("could not close pipe: %s", err)
			}
		}()
		return r.artemisCmd.Run()
	}, "local-tracker")
}

func (r *masterExperimentRunner) startSubtask(t Task, name string) {
	s := Start(t)
	r.subtasks[name] = s
}

func (r *masterExperimentRunner) startSubtaskFunction(f func() error, name string) {
	s := StartFunc(f)
	r.subtasks[name] = s
}

func (r *masterExperimentRunner) mergeFrames() func() error {
	return func() error {
		return MergeFrameReadout(r.env.Balancing, r.artemisListener.Outbound(), r.dispatcher.Incoming())
	}
}

func (r *masterExperimentRunner) waitAnyCriticalSubtask() error {
	criticalTasks := []string{"artemis-in", "frame-merger", "frame-dispatcher", "writer", "video", "local-tracker"}

	cases := make([]reflect.SelectCase, len(criticalTasks))
	for i, name := range criticalTasks {
		errs := r.subtasks[name]
		cases[i] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(errs),
		}
	}

	chosen, v, ok := reflect.Select(cases)
	task := criticalTasks[chosen]

	if ok == false {
		return fmt.Errorf("logic error: channel for task %s is closed", task)
	}

	err, ok := v.Interface().(error)
	if ok == false {
		err = fmt.Errorf("logic error: task %s did not returned an error", task)
	}

	if err != nil {
		return fmt.Errorf("critical task %s error: %w", task, err)
	}

	return nil
}

func (r *masterExperimentRunner) stopAllSubtask() {
	r.cancel()
}

func (r *masterExperimentRunner) waitAllSubtask() {
	wg := sync.WaitGroup{}
	wg.Add(len(r.subtasks))

	for n, t := range r.subtasks {
		go func(name string, errs <-chan error) {
			defer wg.Done()
			err := <-errs
			if err != nil {
				r.logger.Printf("task %s terminated with error: %s", name, err)
			}
		}(n, t)
	}

	wg.Wait()
}
