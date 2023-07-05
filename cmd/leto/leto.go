package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/leto/pkg/letopb"
	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

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

	logger *logrus.Entry
	tracer trace.Tracer
}

func NewLeto(config leto.Config) (*Leto, error) {
	l := &Leto{
		leto:   config,
		node:   GetNodeConfiguration(),
		logger: tm.NewLogger("leto"),
		tracer: otel.Tracer("github.com/formicidae-tracker/leto/cmd/leto"),
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

func (l *Leto) Status(ctx context.Context) *letopb.Status {
	ctx, span := l.tracer.Start(ctx, "Status")
	defer span.End()
	l.mx.Lock()
	defer l.mx.Unlock()

	return l.status(ctx)
}

func (l *Leto) status(ctx context.Context) *letopb.Status {
	res := &letopb.Status{
		Master:     l.node.Master,
		Slaves:     l.node.Slaves,
		Experiment: nil,
	}
	defer l.addDiskInfoToStatus(ctx, res)

	if l.env == nil {
		return res
	}

	yamlConfig, err := l.env.Config.Yaml()
	if err != nil {
		yamlConfig = []byte(fmt.Sprintf("could not generate yaml config: %s", err))
		l.logger.WithContext(ctx).WithError(err).Error("could not generate yaml config")
	}
	res.Experiment = &letopb.ExperimentStatus{
		ExperimentDir:     filepath.Base(l.env.ExperimentDir),
		YamlConfiguration: string(yamlConfig),
		Since:             timestamppb.New(l.env.Start),
	}
	return res
}

func (l *Leto) addDiskInfoToStatus(ctx context.Context, status *letopb.Status) {
	var err error
	defer func() {
		if err != nil {
			l.logger.WithContext(ctx).
				WithError(err).
				Errorf("could not get available disk space")
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

func endSpan(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, "leto error")
		span.RecordError(err)
	}
	span.End()
}

func (l *Leto) Start(ctx context.Context, user *leto.TrackingConfiguration) (err error) {
	ctx, span := l.tracer.Start(ctx, "Start")
	defer func() { endSpan(span, err) }()

	l.mx.Lock()
	defer l.mx.Unlock()
	return l.start(ctx, user)
}

func (l *Leto) experimentLogger(ctx context.Context, config *leto.TrackingConfiguration) *logrus.Entry {
	return l.logger.WithContext(ctx).WithField("experiment", config.ExperimentName)
}

func (l *Leto) start(ctx context.Context, user *leto.TrackingConfiguration) (err error) {
	if l.isStarted() == true {
		return errors.New("already started")
	}
	ctx, l.cancel = context.WithCancel(ctx)
	l.env, err = NewExperimentConfiguration(ctx, l.leto, l.node, user)
	if err != nil {
		return err
	}
	runner, err := NewExperimentRunner(l.env)
	if err != nil {
		return err
	}

	logger := l.experimentLogger(ctx, l.env.Config)

	go func() {
		logger.Info("starting experiment")
		log, err := runner.Run()
		if err != nil {
			l.logger.WithError(err).Error("experiment failed")
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

func (l *Leto) Stop(ctx context.Context) (err error) {
	ctx, span := l.tracer.Start(ctx, "Stop")
	defer func() { endSpan(span, err) }()

	l.mx.Lock()
	defer l.mx.Unlock()

	if l.isStarted() == false {
		return errors.New("already stopped")
	}
	logger := l.experimentLogger(ctx, l.env.Config)
	logger.Info("stopping experiment")
	l.cancel()

	for l.env != nil {
		l.runnerCond.Wait()
	}

	return nil
}

func (l *Leto) SetMaster(ctx context.Context, hostname string) (err error) {
	_, span := l.tracer.Start(ctx, "SetMaster")
	defer func() { endSpan(span, err) }()

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

func (l *Leto) AddSlave(ctx context.Context, hostname string) (err error) {
	_, span := l.tracer.Start(ctx, "AddSlave")
	defer func() { endSpan(span, err) }()

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

func (l *Leto) RemoveSlave(ctx context.Context, hostname string) (err error) {
	_, span := l.tracer.Start(ctx, "RemoveSlave")
	defer func() { endSpan(span, err) }()

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
	logger := l.logger.WithField("path", l.persitentFilePath())

	err := os.MkdirAll(filepath.Dir(l.persitentFilePath()), 0755)
	if err != nil {
		logger.
			WithError(err).
			Error("could not create destination directory")
		return
	}
	configData, err := yaml.Marshal(l.env.Config)
	if err != nil {
		logger.
			WithError(err).
			Error("could not marshal config data to persistent")
		return
	}
	err = ioutil.WriteFile(l.persitentFilePath(), configData, 0644)
	if err != nil {
		l.logger.
			WithError(err).
			Error("could not write persitent config file")
	}
}

func (l *Leto) removePersistentFile() {
	err := os.Remove(l.persitentFilePath())
	if err != nil {
		l.logger.
			WithError(err).
			WithField("path", l.persitentFilePath()).
			Error("could not remove persitent file")
	}
}

func (l *Leto) LoadFromPersistentFile() {
	logger := l.logger.WithField("path", l.persitentFilePath())
	configData, err := ioutil.ReadFile(l.persitentFilePath())
	if err != nil {
		if err != os.ErrNotExist {
			logger.WithError(err).Error("could not read file")
		}
		// if there is no file, there is nothing to load
		return
	}
	config := &leto.TrackingConfiguration{}
	err = yaml.Unmarshal(configData, config)
	if err != nil {
		logger.
			WithError(err).
			Error("could not load persistent configuration")
		return
	}
	logger.Info("restarting experiment from persistent file")
	err = l.Start(context.Background(), config)
	if err != nil {
		logger.WithError(err).Error("could not restart experiment from persistent file")
	}
}
