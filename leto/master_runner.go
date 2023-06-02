package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/leto/letopb"
	"golang.org/x/exp/constraints"
)

type masterRunner struct {
	env *TrackingEnvironment

	artemisCmd *exec.Cmd

	artemisListener   ArtemisListener
	hermesBroadcaster HermesBroadcaster
	fileWriter        HermesFileWriter
	video             VideoTask
	dispatcher        FrameDispatcher
	olympus           OlympusTask

	trackerCtx, otherCtx             context.Context
	cancelLocalTracker, cancelOthers context.CancelFunc

	artemisOut, videoIn *os.File

	subtasks map[string]<-chan error
	logger   *log.Logger
}

func newMasterRunner(env *TrackingEnvironment) (ExperimentRunner, error) {
	trackerCtx, cancelTracker := context.WithCancel(env.Context)
	otherCtx, cancelOther := context.WithCancel(context.Background())
	res := &masterRunner{
		env:                env,
		subtasks:           make(map[string]<-chan error),
		trackerCtx:         trackerCtx,
		otherCtx:           otherCtx,
		cancelLocalTracker: cancelTracker,
		cancelOthers:       cancelOther,
		logger:             NewLogger("runner"),
	}
	if err := res.SetUp(); err != nil {
		return nil, err
	}

	return res, nil
}

func (r *masterRunner) SetUp() error {
	var err error
	r.artemisListener, err = NewArtemisListener(r.otherCtx, r.env.Leto.ArtemisIncomingPort)
	if err != nil {
		return err
	}

	r.hermesBroadcaster, err = NewHermesBroadcaster(r.otherCtx,
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

	r.videoIn, r.artemisOut, err = os.Pipe()
	if err != nil {
		return err
	}

	r.artemisCmd, err = r.env.SetUp()
	r.artemisCmd.Stdout = r.artemisOut
	if err != nil {
		return err
	}
	r.olympus, err = NewOlympusTask(r.otherCtx, r.env)
	if err != nil {
		r.logger.Printf("will not register to olympus: %s", err)
	}

	return nil
}

func (r *masterRunner) Run() (log *letopb.ExperimentLog, err error) {
	defer func() {
		var terr error
		log, terr = r.env.TearDown(err != nil)
		if err == nil {
			err = terr
		}
	}()

	r.startSubtasks()

	go func() {
		//wait for either the env.Context or own to be Done
		<-r.trackerCtx.Done()
		// if another critical task or env.Context we need to signal
		// artemis. artemis may have crashed but then the signal will
		// simply be lost.
		r.artemisCmd.Process.Signal(os.Interrupt)

		// if already terminated, will do nothing (artemis crashed before signal).
		for !WaitDoneOrFunc(r.otherCtx.Done(), 500*time.Millisecond, func(grace time.Duration) {
			r.logger.Printf("killing artemis as it did not terminate after %s", grace)
			r.cancelOthers() // to avoid to mark X timeout while we wait for termination
			if err := r.artemisCmd.Process.Kill(); err != nil {
				r.logger.Printf("could not kill artemis: %s", err)
			}
		}) {
		}
	}()

	err = r.waitAnyCriticalSubtask()

	r.stopLocalTracker()
	lerr := r.waitForLocalTracker()
	if err == nil {
		err = lerr
	}
	r.stopAllOtherSubtasks()
	r.waitAllSubtasks()

	return nil, err
}

func (r *masterRunner) startSubtasks() {
	r.startSubtask(NewDiskWatcher(r.otherCtx, r.env, r.olympus), "disk-watcher")
	r.startSubtask(r.artemisListener, "artemis-in")

	r.startSubtaskFunction(r.mergeFrames(), "frame-merger")
	r.startSubtask(r.dispatcher, "frame-dispatcher")
	r.startSubtask(r.fileWriter, "writer")
	r.startSubtask(r.hermesBroadcaster, "broadcaster")
	r.startSubtaskFunction(func() error {
		return r.video.Run(r.videoIn)
	}, "video")

	// slaves must be started before local tracker !!
	r.startSlaves()

	if r.olympus != nil {
		r.startSubtask(r.olympus, "olympus-registration")
	}

	r.startSubtask(r.artemisCmd, "local-tracker")
}

func (r *masterRunner) startSubtask(t Task, name string) {
	s := Start(t)
	r.subtasks[name] = s
}

func (r *masterRunner) startSubtaskFunction(f func() error, name string) {
	s := StartFunc(f)
	r.subtasks[name] = s
}

func (r *masterRunner) mergeFrames() func() error {
	return func() error {
		return MergeFrameReadout(r.otherCtx, r.env.Balancing, r.artemisListener.Outbound(), r.dispatcher.Incoming())
	}
}

func (r *masterRunner) waitAnyCriticalSubtask() error {
	name := ""
	var err error
	select {
	case err = <-r.subtasks["local-tracker"]:
		name = "local-tracker"
	case err = <-r.subtasks["artemis-in"]:
		name = "artemis-in"
	case err = <-r.subtasks["frame-merger"]:
		name = "frame-merger"
	case err = <-r.subtasks["frame-dispatcher"]:
		name = "frame-dispatcher"
	case err = <-r.subtasks["writer"]:
		name = "writer"
	case err = <-r.subtasks["video"]:
		name = "video"
	case err = <-r.subtasks["disk-watcher"]:
		name = "disk-watcher"
	}

	if err == nil && name != "local-tracker" {
		return fmt.Errorf("critical task %s exited early without an error", name)
	}
	return err
}

func (r *masterRunner) stopLocalTracker() {
	r.stopSlaves()
	r.cancelLocalTracker()
}

func (r *masterRunner) waitForLocalTracker() error {
	err := <-r.subtasks["local-tracker"]
	if cerr := r.artemisOut.Close(); cerr != nil {
		r.logger.Printf("could not close artemis out pipe: %s", cerr)
	}
	return err
}

func (r *masterRunner) stopAllOtherSubtasks() {
	err := r.artemisOut.Close()
	if err != nil {
		r.logger.Printf("could not kill artemis out pipe: %s", err)
	}
	r.cancelOthers()
}

func Min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func (r *masterRunner) waitAllSubtasks() {
	wg := sync.WaitGroup{}
	wg.Add(len(r.subtasks))

	for n, t := range r.subtasks {
		go func(name string, errs <-chan error) {
			defer wg.Done()
			var err error
			delay := 500 * time.Millisecond
			total := time.Duration(0)
			stop := false
			for stop == false {
				select {
				case err = <-errs:
					stop = true
				case <-time.After(delay):
					total += delay
					r.logger.Printf("%s task still running after %s", name, total)
					delay = Min(2*delay, 10*time.Second)
				}
			}

			if err != nil {
				r.logger.Printf("task %s terminated with error: %s", name, err)
			}
		}(n, t)
	}

	wg.Wait()
}

func (r *masterRunner) startSlaves() {
	if len(r.env.Node.Slaves) == 0 {
		return
	}
	nl := leto.NewNodeLister()
	nodes, err := nl.ListNodes()
	if err != nil {
		r.logger.Printf("could not list local nodes: %s", err)
		return
	}

	for _, name := range r.env.Node.Slaves {
		if err := r.startSlave(nodes, name); err != nil {
			r.logger.Printf("could not start slave %s: %s", name, err)
		}
	}
}

func (r *masterRunner) stopSlaves() {
	if len(r.env.Node.Slaves) == 0 {
		return
	}
	nl := leto.NewNodeLister()
	nodes, err := nl.ListNodes()
	if err != nil {
		r.logger.Printf("could not list local nodes: %s", err)
		return
	}

	for _, name := range r.env.Node.Slaves {
		if err := r.stopSlave(nodes, name); err != nil {
			r.logger.Printf("could not stop slave %s: %s", name, err)
		}
	}
}

func (r *masterRunner) startSlave(nodes map[string]leto.Node, name string) error {
	slave, ok := nodes[name]
	if ok == false {
		return errors.New("not found on the network")
	}
	slaveConfig := *r.env.Config
	slaveConfig.Loads.SelfUUID = slaveConfig.Loads.UUIDs[name]
	asYaml, err := slaveConfig.Yaml()
	if err != nil {
		return fmt.Errorf("could not serialize config: %s", err)
	}
	return slave.StartTracking(&letopb.StartRequest{
		YamlConfiguration: string(asYaml),
	})
}

func (r *masterRunner) stopSlave(nodes map[string]leto.Node, name string) error {
	slave, ok := nodes[name]
	if ok == false {
		return errors.New("not found on the network")
	}
	return slave.StopTracking()
}
