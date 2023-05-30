package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/formicidae-tracker/leto/letopb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ExperimentRunner interface {
	Run() (*letopb.ExperimentLog, error)
}

type experimentRunner struct {
	config     *ExperimentConfiguration
	artemisCmd *exec.Cmd
}

func NewExperimentRunner(config *ExperimentConfiguration) (ExperimentRunner, error) {
	res := &experimentRunner{
		config: config,
	}

	if err := res.setUp(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *experimentRunner) setUp() error {
	err := r.makeAllDestinationDirs()
	if err != nil {
		return err
	}
	r.artemisCmd, err = r.buildArtemisCommand()

	if r.config.Node.IsMaster() == true {
		return nil
	}
	//TODO: implement master ?
	return nil
}

func (r *experimentRunner) buildArtemisCommand() (*exec.Cmd, error) {
	cmd := exec.Command("artemis", r.config.TrackingCommandArgs()...)
	err := r.saveArtemisCommand(cmd)
	if err != nil {
		return nil, err
	}
	cmd.Stderr, err = os.Create(r.config.Path("artemis.stderr"))
	if err != nil {
		return nil, err

	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	return cmd, nil
}

func (r *experimentRunner) saveArtemisCommand(cmd *exec.Cmd) error {
	f, err := os.Create(r.config.Path("artemis.cmd"))
	if err != nil {
		return err
	}
	defer f.Close()
	values := append([]string{cmd.Path}, cmd.Args...)
	fmt.Fprintln(f, strings.Join(values, " "))
	return nil
}

func (r *experimentRunner) makeAllDestinationDirs() error {
	target := r.config.ExperimentDir
	if r.config.Node.IsMaster() == true {
		target = r.config.Path("ants")
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("could not create %s: %w", target, err)
	}
	return nil
}

func (r *experimentRunner) tearDown() error {
	return r.removeTestExperimentData()
}

func (r *experimentRunner) removeTestExperimentData() error {
	if r.config.TestMode == false {
		return nil
	}
	return os.RemoveAll(r.config.ExperimentDir)
}

func (r *experimentRunner) Run() (*letopb.ExperimentLog, error) {
	defer r.tearDown()
	start := time.Now()
	err := r.artemisCmd.Run()
	return r.buildLog(err != nil, start), err
}

func (r *experimentRunner) buildLog(hasError bool, start time.Time) *letopb.ExperimentLog {
	end := time.Now()
	log, err := ioutil.ReadFile(r.config.Path("artemis.INFO"))
	if err != nil {
		log = append(log, []byte(fmt.Sprintf("\ncould not read log: %s", err))...)
	}
	stderr, err := ioutil.ReadFile(r.config.Path("artemis.stderr"))
	if err != nil {
		stderr = append(stderr, []byte(fmt.Sprintf("\ncould not read stderr: %s", err))...)
	}

	yaml, err := r.config.Tracking.Yaml()
	if err != nil {
		yaml = []byte(fmt.Sprintf("could not generate yaml config: %s", err))
	}
	return &letopb.ExperimentLog{
		HasError:          hasError,
		ExperimentDir:     filepath.Base(r.config.ExperimentDir),
		Start:             timestamppb.New(start),
		End:               timestamppb.New(end),
		YamlConfiguration: string(yaml),
		Log:               string(log),
		Stderr:            string(stderr),
	}
}
