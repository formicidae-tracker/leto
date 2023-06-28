package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/leto/pkg/letopb"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v2"
)

type Leto struct {
	mx   sync.Mutex
	leto leto.Config
	node NodeConfiguration

	cancel context.CancelFunc

	env        *TrackingEnvironment
	runnerCond *sync.Cond

	lastExperimentLog *letopb.ExperimentLog

	logger *log.Logger
}

func NewLeto(config leto.Config) (*Leto, error) {
	l := &Leto{
		leto:   config,
		node:   GetNodeConfiguration(),
		logger: NewLogger("leto"),
	}
	l.runnerCond = sync.NewCond(&l.mx)
	if err := l.check(); err != nil {
		return nil, err
	}

	err := os.MkdirAll(xdg.DataHome, 0755)
	if err != nil {
		return nil, err
	}

	l.LoadFromPersistentFile()
	return l, nil
}

func (l *Leto) check() error {
	checks := []func() error{l.checkArtemis, l.checkFFMpeg}
	if l.leto.DevMode == false {
		checks = append(checks, l.checkFirmwareVariant)
	}

	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

func (l *Leto) checkArtemis() error {
	cmd := exec.Command(artemisCommandName, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not get artemis version: %s %w ", string(output), err)
	}
	artemisVersion := strings.TrimPrefix(strings.TrimSpace(string(output)), "artemis ")
	return checkArtemisVersion(artemisVersion, leto.ARTEMIS_MIN_VERSION)
}

func (l *Leto) checkFFMpeg() error {
	cmd := exec.Command(ffmpegCommandName, "-version")
	_, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not found ffmpeg: %w", err)
	}
	return nil
}

func (l *Leto) checkFirmwareVariant() error {
	return getAndCheckFirmwareVariant(l.node)
}

func (l *Leto) Status() *letopb.Status {
	l.mx.Lock()
	defer l.mx.Unlock()
	return l.status()
}

func (l *Leto) status() *letopb.Status {
	res := &letopb.Status{
		Master:     l.node.Master,
		Slaves:     l.node.Slaves,
		Experiment: nil,
	}
	defer l.addDiskInfoToStatus(res)

	if l.env == nil {
		return res
	}

	yamlConfig, err := l.env.Config.Yaml()
	if err != nil {
		yamlConfig = []byte(fmt.Sprintf("could not generate yaml config: %s", err))
	}
	res.Experiment = &letopb.ExperimentStatus{
		ExperimentDir:     filepath.Base(l.env.ExperimentDir),
		YamlConfiguration: string(yamlConfig),
		Since:             timestamppb.New(l.env.Start),
	}
	return res
}

func (l *Leto) addDiskInfoToStatus(status *letopb.Status) {
	var err error
	defer func() {
		if err != nil {
			l.logger.Printf("could not get available disk space: %s", err)
		}
	}()

	if l.isStarted() == false {
		status.FreeBytes, status.TotalBytes, err = getDiskSize(xdg.DataHome)
		return
	}
	status.FreeBytes, status.TotalBytes, status.BytesPerSecond, err = l.env.WatchDisk(time.Now())
}

func (l *Leto) LastExperimentLog() *letopb.ExperimentLog {
	l.mx.Lock()
	defer l.mx.Unlock()
	return l.lastExperimentLog
}

func (l *Leto) Start(user *leto.TrackingConfiguration) error {
	l.mx.Lock()
	defer l.mx.Unlock()
	return l.start(user)
}

func (l *Leto) start(user *leto.TrackingConfiguration) (err error) {
	if l.isStarted() == true {
		return errors.New("already started")
	}
	var ctx context.Context
	ctx, l.cancel = context.WithCancel(context.Background())
	l.env, err = NewExperimentConfiguration(ctx, l.leto, l.node, user)
	if err != nil {
		return err
	}
	runner, err := NewExperimentRunner(l.env)
	if err != nil {
		return err
	}

	go func() {
		l.logger.Printf("starting experiment %s", l.env.Config.ExperimentName)
		log, err := runner.Run()
		if err != nil {
			l.logger.Printf("experiment failed: %s", err)
		}

		l.mx.Lock()
		defer l.mx.Unlock()
		l.lastExperimentLog = log
		l.env = nil
		l.removePersistentFile()
		l.runnerCond.Broadcast()
	}()

	l.writePersistentFile()
	return nil
}

func (l *Leto) Stop() error {
	l.mx.Lock()
	defer l.mx.Unlock()

	if l.isStarted() == false {
		return errors.New("already stopped")
	}
	l.logger.Printf("stopping experiment %s", l.env.Config.ExperimentName)
	l.cancel()

	// to avoid a deadlock, we must unlo
	for l.env != nil {
		l.runnerCond.Wait()
	}

	return nil
}

func (l *Leto) SetMaster(hostname string) error {
	l.mx.Lock()
	defer l.mx.Unlock()
	if l.isStarted() == true {
		return fmt.Errorf("could not change node configuration while experiment %s is running", l.env.Config.ExperimentName)
	}
	return l.setMaster(hostname)
}

func (l *Leto) setMaster(hostname string) (err error) {
	defer func() {
		if err == nil {
			l.node.Save()
		}
	}()

	if len(hostname) == 0 {
		l.node.Master = ""
		return
	}

	if len(l.node.Slaves) != 0 {
		err = fmt.Errorf("cannot set node as slave as it has its own slaves (%s)", l.node.Slaves)
		return
	}
	l.node.Master = hostname
	err = getAndCheckFirmwareVariant(l.node)
	if err != nil {
		l.node.Master = ""
	}
	return
}

func (l *Leto) AddSlave(hostname string) (err error) {
	l.mx.Lock()
	defer l.mx.Unlock()
	if l.isStarted() == true {
		return fmt.Errorf("Could not change node configuration while experiment %s is running", l.env.Config.ExperimentName)
	}

	return l.addSlave(hostname)
}

func (l *Leto) addSlave(hostname string) (err error) {
	defer func() {
		if err == nil {
			l.node.Save()
		}
	}()

	err = l.setMaster("")
	if err != nil {
		return
	}
	err = getAndCheckFirmwareVariant(l.node)
	if err != nil {
		return
	}

	err = l.node.AddSlave(hostname)
	return
}

func (l *Leto) RemoveSlave(hostname string) (err error) {
	l.mx.Lock()
	defer l.mx.Unlock()
	if l.isStarted() == true {
		return fmt.Errorf("could not change node configuration while experiment %s is running", l.env.Config.ExperimentName)
	}
	return l.removeSlave(hostname)
}

func (l *Leto) removeSlave(hostname string) (err error) {
	defer func() {
		if err == nil {
			l.node.Save()
		}
	}()

	return l.node.RemoveSlave(hostname)
}

func (l *Leto) isStarted() bool {
	return l.env != nil
}

func (l *Leto) persitentFilePath() string {
	return filepath.Join(xdg.DataHome, "fort/leto/current-experiment.yml")
}

func (l *Leto) writePersistentFile() {
	err := os.MkdirAll(filepath.Dir(l.persitentFilePath()), 0755)
	if err != nil {
		l.logger.Printf("could not create data dir for '%s': %s",
			l.persitentFilePath(), err)
		return
	}
	configData, err := yaml.Marshal(l.env.Config)
	if err != nil {
		l.logger.Printf("could not marshal config data to persistent file: %s",
			err)
		return
	}
	err = ioutil.WriteFile(l.persitentFilePath(), configData, 0644)
	if err != nil {
		l.logger.Printf("could not write persitent config file %s: %s",
			l.persitentFilePath(), err)
	}
}

func (l *Leto) removePersistentFile() {
	err := os.Remove(l.persitentFilePath())
	if err != nil {
		l.logger.Printf("could not remove persitent file '%s': %s",
			l.persitentFilePath(), err)
	}
}

func (l *Leto) LoadFromPersistentFile() {
	configData, err := ioutil.ReadFile(l.persitentFilePath())
	if err != nil {
		// if there is no file, there is nothing to load
		return
	}
	config := &leto.TrackingConfiguration{}
	err = yaml.Unmarshal(configData, config)
	if err != nil {
		l.logger.Printf("could not load configuration from '%s': %s",
			l.persitentFilePath(), err)
		return
	}
	l.logger.Printf("restarting experiment from '%s'", l.persitentFilePath())
	err = l.Start(config)
	if err != nil {
		l.logger.Printf("could not start experiment from '%s': %s",
			l.persitentFilePath(), err)
	}
}
