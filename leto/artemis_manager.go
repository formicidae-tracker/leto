package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/blang/semver"
	"github.com/formicidae-tracker/hermes"
	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/leto/letopb"
	olympuspb "github.com/formicidae-tracker/olympus/api"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v2"
)

type ArtemisManager struct {
	incoming, merged, file, broadcast chan *hermes.FrameReadout
	mx                                sync.Mutex
	wg, artemisWg, trackerWg          sync.WaitGroup

	ctx    context.Context
	cancel func()

	fileWriter  HermesFileWriter
	trackers    ArtemisListener
	broadcaster HermesBroadcaster
	nodeConfig  NodeConfiguration

	artemisCmd      *exec.Cmd
	artemisOut      *io.PipeWriter
	videoIn         *io.PipeReader
	videoManager    VideoManager
	videoManagerErr <-chan error
	testMode        bool

	stopRegistration  func()
	registrationEnded chan struct{}

	logger *log.Logger

	since time.Time

	lastExperimentLog *letopb.ExperimentLog
	letoConfig        leto.Config
	experimentConfig  *TrackingEnvironment
}

func NewArtemisManager(letoConfig leto.Config) (*ArtemisManager, error) {
	cmd := exec.Command("artemis", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Could not find artemis: %s", err)
	}

	artemisVersion := strings.TrimPrefix(strings.TrimSpace(string(output)), "artemis ")
	err = checkArtemisVersion(artemisVersion, leto.ARTEMIS_MIN_VERSION)
	if err != nil {
		return nil, err
	}

	cmd = exec.Command("ffmpeg", "-version")
	_, err = cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Could not find ffmpeg: %s", err)
	}

	nodeConfig := GetNodeConfiguration()

	err = getAndCheckFirmwareVariant(nodeConfig, false)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ArtemisManager{
		nodeConfig: nodeConfig,
		logger:     NewLogger("artemis"),
		letoConfig: letoConfig,
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

func (m *ArtemisManager) Status() *letopb.Status {
	m.mx.Lock()
	defer m.mx.Unlock()
	res := &letopb.Status{
		Master:     m.nodeConfig.Master,
		Slaves:     m.nodeConfig.Slaves,
		Experiment: nil,
	}

	yamlConfig, err := m.experimentConfig.Config.Yaml()
	if err != nil {
		yamlConfig = []byte(fmt.Sprintf("Could not generate yaml config: %s", err))
	}
	if m.incoming != nil {
		res.Experiment = &letopb.ExperimentStatus{
			ExperimentDir:     filepath.Base(m.experimentConfig.ExperimentDir),
			YamlConfiguration: string(yamlConfig),
			Since:             timestamppb.New(m.since),
		}
	}
	return res
}

func (m *ArtemisManager) LastExperimentLog() *letopb.ExperimentLog {
	m.mx.Lock()
	defer m.mx.Unlock()
	return m.lastExperimentLog
}

func (m *ArtemisManager) Start(userConfig *leto.TrackingConfiguration) error {
	m.mx.Lock()
	defer m.mx.Unlock()
	if m.incoming != nil {
		return fmt.Errorf("ArtemisManager: Start: already started")
	}

	//why two steps ?
	if err := m.setUpExperiment(userConfig); err != nil {
		return err
	}

	m.spawnTasks()

	// again ? or is it a task
	// slave should not register ....
	m.registerOlympus()

	// ok
	m.writePersistentFile()

	return nil
}

func (m *ArtemisManager) Stop() error {
	m.mx.Lock()
	defer m.mx.Unlock()

	if m.isStarted() == false {
		return fmt.Errorf("Already stoppped")
	}

	m.removePersistentFile()

	m.unregisterOlympus()

	// why would it be nil ?
	if m.artemisCmd != nil {
		if m.nodeConfig.IsMaster() == true {
			m.stopSlavesTrackers()
		}

		m.artemisCmd.Process.Signal(os.Interrupt)
		m.logger.Printf("Waiting for artemis process to stop")
		m.artemisCmd = nil
	}

	// WHY?????
	m.mx.Unlock()
	m.artemisWg.Wait()
	m.mx.Lock()
	return nil
}

func (m *ArtemisManager) SetMaster(hostname string) error {
	m.mx.Lock()
	defer m.mx.Unlock()
	if m.isStarted() == true {
		return fmt.Errorf("Could not change master/slave configuration while experiment %s is running", m.experimentConfig.Config.ExperimentName)
	}
	return m.setMaster(hostname)
}

func (m *ArtemisManager) setMaster(hostname string) (err error) {
	defer func() {
		if err == nil {
			m.nodeConfig.Save()
		}
	}()

	if len(hostname) == 0 {
		m.nodeConfig.Master = ""
		return
	}

	if len(m.nodeConfig.Slaves) != 0 {
		err = fmt.Errorf("Cannot set node as slave as it has its own slaves (%s)", m.nodeConfig.Slaves)
		return
	}
	m.nodeConfig.Master = hostname
	err = getAndCheckFirmwareVariant(m.nodeConfig, true)
	if err != nil {
		m.nodeConfig.Master = ""
	}
	return
}

func (m *ArtemisManager) AddSlave(hostname string) (err error) {
	m.mx.Lock()
	defer m.mx.Unlock()
	if m.isStarted() == true {
		return fmt.Errorf("Could not change master/slave configuration while experiment %s is running", m.experimentConfig.Config.ExperimentName)
	}

	return m.addSlave(hostname)
}

func (m *ArtemisManager) addSlave(hostname string) (err error) {
	defer func() {
		if err == nil {
			m.nodeConfig.Save()
		}
	}()

	err = m.setMaster("")
	if err != nil {
		return
	}
	err = getAndCheckFirmwareVariant(m.nodeConfig, true)
	if err != nil {
		return
	}

	err = m.nodeConfig.AddSlave(hostname)
	return
}

func (m *ArtemisManager) RemoveSlave(hostname string) (err error) {
	m.mx.Lock()
	defer m.mx.Unlock()
	if m.isStarted() == true {
		return fmt.Errorf("Could not change master/slave configuration while experiment %s is running", m.experimentConfig.Config.ExperimentName)
	}
	return m.removeSlave(hostname)
}

func (m *ArtemisManager) removeSlave(hostname string) (err error) {
	defer func() {
		if err == nil {
			m.nodeConfig.Save()
		}
	}()

	return m.nodeConfig.RemoveSlave(hostname)
}

func checkArtemisVersion(actual, minimal string) error {
	a, err := semver.ParseTolerant(actual)
	if err != nil {
		return err
	}
	m, err := semver.ParseTolerant(minimal)
	if err != nil {
		return err
	}

	if m.Major == 0 {
		if a.Major != 0 || a.Minor != m.Minor {
			return fmt.Errorf("Unexpected major version v%d.%d (expected: v%d.%d)", a.Major, a.Minor, m.Major, m.Minor)
		}
	} else if m.Major != a.Major {
		return fmt.Errorf("Unexpected major version v%d (expected: v%d)", a.Major, m.Major)
	}

	if a.GE(m) == false {
		return fmt.Errorf("Invalid version v%s (minimal: v%s)", a, m)
	}

	return nil
}

func getAndCheckFirmwareVariant(c NodeConfiguration, checkMaster bool) error {
	variant, err := getFirmwareVariant()
	if err != nil {
		if c.IsMaster() && checkMaster == false {
			return nil
		}
		return err
	}
	return checkFirmwareVariant(c, variant, checkMaster)
}

func getFirmwareVariant() (string, error) {
	cmd := exec.Command("coaxlink-firmware")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Could not check slave firmware variant")
	}

	return extractCoaxlinkFirmwareOutput(output)
}

func checkFirmwareVariant(c NodeConfiguration, variant string, checkMaster bool) error {
	expected := "1-camera"
	if c.IsMaster() == false {
		expected = "1-df-camera"
	} else if checkMaster == false {
		return nil
	}

	if variant != expected {
		return fmt.Errorf("Unexpected firmware variant %s (expected: %s)", variant, expected)
	}

	return nil
}

func extractCoaxlinkFirmwareOutput(output []byte) (string, error) {
	rx := regexp.MustCompile(`Firmware variant:\W+[0-9]+\W+\(([0-9a-z\-]+)\)`)
	m := rx.FindStringSubmatch(string(output))
	if len(m) == 0 {
		return "", fmt.Errorf("Could not determine firmware variant in output: '%s'", output)
	}
	return m[1], nil
}

func (m *ArtemisManager) isStarted() bool {
	return m.incoming != nil
}

func (m *ArtemisManager) setUpExperiment(user *leto.TrackingConfiguration) error {
	var err error
	m.experimentConfig, err = NewExperimentConfiguration(context.TODO(), m.letoConfig, m.nodeConfig, user)
	if err != nil {
		return err
	}

	err = os.MkdirAll(m.experimentConfig.ExperimentDir, 0755)
	if err != nil {
		return err
	}

	if err := m.setUpTrackerTask(); err != nil {
		return err
	}

	if m.nodeConfig.IsMaster() == true {
		if err := m.setUpExperimentAsMaster(); err != nil {
			return err
		}
	}

	if err := m.backUpConfigToExperimentDir(); err != nil {
		return err
	}

	// we sets the channel last, as it sets the experiment as started
	// externally, an we do it only were there were no error.
	m.incoming = make(chan *hermes.FrameReadout, 10)

	return nil
}

func (m *ArtemisManager) spawnTasks() {
	if m.nodeConfig.IsMaster() == true {
		m.spawnMasterSubTasks()
	}
	m.spawnLocalTracker()
}

func (m *ArtemisManager) setUpSubTasksChannels() {
	m.merged = make(chan *hermes.FrameReadout, 10)
	m.file = make(chan *hermes.FrameReadout, 200)
	m.broadcast = make(chan *hermes.FrameReadout, 10)
}

func (m *ArtemisManager) setUpFileWriterTask() error {
	var err error
	m.fileWriter, err = NewFrameReadoutWriter(filepath.Join(m.experimentConfig.ExperimentDir, "tracking.hermes"))
	return err
}

func (m *ArtemisManager) setUpStreamTask() error {
	var err error
	m.videoIn, m.artemisOut = io.Pipe()
	m.artemisCmd.Stdout = m.artemisOut
	m.videoManager, err = NewVideoManager(
		m.experimentConfig.ExperimentDir,
		*m.experimentConfig.Config.Camera.FPS/float64(m.experimentConfig.Balancing.Stride),
		m.experimentConfig.Config.Stream,
	)
	return err
}

func (m *ArtemisManager) antOutputDir() string {
	return filepath.Join(m.experimentConfig.ExperimentDir, "ants")
}

func (m *ArtemisManager) setUpAntOutputDir() error {
	return os.MkdirAll(m.antOutputDir(), 0755)
}

func (m *ArtemisManager) setUpExperimentAsMaster() error {
	if err := m.setUpAntOutputDir(); err != nil {
		return err
	}

	m.setUpSubTasksChannels()

	if err := m.setUpFileWriterTask(); err != nil {
		return err
	}

	if err := m.setUpStreamTask(); err != nil {
		return err
	}

	return nil
}

func (m *ArtemisManager) backUpConfigToExperimentDir() error {
	//save the config to the experiment dir
	confSaveName := filepath.Join(m.experimentConfig.ExperimentDir, "leto-final-config.yml")
	return m.experimentConfig.Config.WriteConfiguration(confSaveName)
}

func (m *ArtemisManager) setUpTrackerTask() error {
	logFilePath := filepath.Join(m.experimentConfig.ExperimentDir, "artemis.command")
	artemisCommandLog, err := os.Create(logFilePath)
	if err != nil {
		return fmt.Errorf("Could not create artemis log file ('%s'): %s", logFilePath, err)
	}
	defer artemisCommandLog.Close()

	m.artemisCmd = m.buildTrackingCommand()
	m.artemisCmd.Stderr, err = os.Create(filepath.Join(m.experimentConfig.ExperimentDir, "artemis.stderr"))
	if err != nil {
		return err
	}
	m.artemisCmd.Stdin = nil
	m.artemisCmd.Stdout = nil

	fmt.Fprintf(artemisCommandLog, "%s %s\n", m.artemisCmd.Path, m.artemisCmd.Args)
	return nil
}

func (m *ArtemisManager) spawnFrameReadoutMergeTask() {
	m.wg.Add(1)
	go func() {
		MergeFrameReadout(m.experimentConfig.Balancing, m.incoming, m.merged)
		m.wg.Done()
	}()
}

func (m *ArtemisManager) spawnFrameReadoutDispatchTask() {
	m.wg.Add(1)
	go func() {
		for i := range m.merged {
			select {
			case m.file <- i:
			default:
			}
			select {
			case m.broadcast <- i:
			default:
			}
		}
		close(m.file)
		close(m.broadcast)
		m.wg.Done()
	}()
}

func (m *ArtemisManager) spawnTrackerListenTask() {
	m.trackerWg.Add(1)
	go func() {
		defer m.trackerWg.Done()
		var err error
		m.trackers, err = NewArtemisListener(m.ctx, m.letoConfig.ArtemisIncomingPort)
		if err != nil {
			m.logger.Printf("could not listen: %s", err)
			return
		}

		err = m.trackers.Run()
		if err != nil {
			m.logger.Printf("listening for tracker unhandled error: %s", err)
		} else {
			m.logger.Printf("All connection closed, cleaning up experiment")
		}
	}()
}

func (m *ArtemisManager) spawnFrameReadoutBroadCastTask() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		broadcaster, err := NewHermesBroadcaster(m.ctx, m.letoConfig.HermesBroadcastPort, 3*time.Duration(1.0e6/(*m.experimentConfig.Config.Camera.FPS))*time.Microsecond)
		if err != nil {
			m.logger.Printf("could not broadcast frames: %s", err)
			return
		}
		err = broadcaster.Run()
		if err != nil {
			m.logger.Printf("broadcast unhandled error: %s", err)
		}
	}()
}

func (m *ArtemisManager) spawnFrameReadoutWriteTask() {
	m.wg.Add(1)
	go func() {
		m.fileWriter.Run()
		m.wg.Done()
	}()
}

func (m *ArtemisManager) spawnStreamTask() {
	//TODO: setup waitgroup ? Was not done so maybe it was stopping
	//the application to work from a weird race condition. But it
	//should ultimately have some kind of synchronization
	m.videoManagerErr = StartFunc(func() error {
		return m.videoManager.Run(m.videoIn)
	})
}

func (m *ArtemisManager) startSlavesTrackers() {
	if len(m.nodeConfig.Slaves) == 0 {
		return
	}

	nl := leto.NewNodeLister()
	nodes, err := nl.ListNodes()
	if err != nil {
		m.logger.Printf("Could not list all local nodes: %s", err)
		m.logger.Printf("Not starting slaves")
	}

	for _, slaveName := range m.nodeConfig.Slaves {
		slave, ok := nodes[slaveName]
		if ok == false {
			m.logger.Printf("Could not find slave '%s', not starting it", slaveName)
			continue
		}

		slaveConfig := *m.experimentConfig.Config
		slaveConfig.Loads.SelfUUID = slaveConfig.Loads.UUIDs[slaveName]
		asYaml, err := slaveConfig.Yaml()
		if err != nil {
			m.logger.Printf("Could not serialize slave %s config: %s", slaveName, err)
		}

		err = slave.StartTracking(&letopb.StartRequest{
			YamlConfiguration: string(asYaml),
		})

		if err != nil {
			m.logger.Printf("Could not start slave %s: %s", slaveName, err)
		}
	}
}

func (m *ArtemisManager) stopSlavesTrackers() {
	nl := leto.NewNodeLister()
	nodes, err := nl.ListNodes()
	if err != nil {
		m.logger.Printf("Could not list all local nodes: %s", err)
		m.logger.Printf("Not stopping slaves")
	}

	for _, slaveName := range m.nodeConfig.Slaves {
		slave, ok := nodes[slaveName]
		if ok == false {
			m.logger.Printf("Could not find slave '%s', not stopping it", slaveName)
			continue
		}
		err := slave.StopTracking()
		if err != nil {
			m.logger.Printf("Could not stop slave %s: %s", slaveName, err)
		}
	}
}

func (m *ArtemisManager) spawnMasterSubTasks() {
	m.spawnFrameReadoutDispatchTask()
	m.spawnFrameReadoutMergeTask()
	m.spawnTrackerListenTask()
	m.spawnFrameReadoutBroadCastTask()
	m.spawnFrameReadoutWriteTask()
	m.spawnStreamTask()
	m.startSlavesTrackers()
}

func (m *ArtemisManager) tearDownTrackerListenTask() {
	//Stops the reading of frame readout, it will close all the chain
	m.cancel()

	m.logger.Printf("Waiting for all tracker connections to be closed")

	m.trackerWg.Wait()
}

func (m *ArtemisManager) tearDownFilewriter() {
}

func (m *ArtemisManager) tearDownStreamTask() {
	if m.videoManager != nil {
		m.logger.Printf("Waiting for stream tasks to stop")
		m.artemisOut.Close()
		err := <-m.videoManagerErr
		if err != nil {
			m.logger.Printf("video error: %s", err)
		}
		m.videoManager = nil
		m.videoManagerErr = nil
		m.videoIn.Close()
		m.artemisOut = nil
		m.videoIn = nil
	}
}

func (m *ArtemisManager) tearDownSubTasks() {
	close(m.incoming)
	m.logger.Printf("Waiting for all sub task to finish")
	m.wg.Wait()

	m.tearDownFilewriter()
	m.tearDownStreamTask()
}

func (m *ArtemisManager) cleanUpGlobalVariables() {
	m.artemisCmd = nil
	m.incoming = nil
	m.merged = nil
	m.file = nil
	m.broadcast = nil
	m.trackers = nil
	m.artemisOut = nil
	m.videoIn = nil
	m.videoManager = nil
	m.experimentConfig = nil
}

func (m *ArtemisManager) tearDownExperiment(err error) {
	m.mx.Lock()
	defer m.mx.Unlock()

	/// this is done twice !!!! WHY ????
	if err != nil {
		m.removePersistentFile()
	}

	m.lastExperimentLog = newExperimentLog(err != nil, m.since, m.experimentConfig.Config, m.experimentConfig.ExperimentDir)

	// Why two cleanup ?
	m.tearDownTrackerListenTask()
	m.tearDownSubTasks()

	m.logger.Printf("Experiment '%s' done", m.experimentConfig.Config.ExperimentName)

	if m.testMode == true {
		log.Printf("Cleaning '%s'", m.experimentConfig.ExperimentDir)
		if err := os.RemoveAll(m.experimentConfig.ExperimentDir); err != nil {
			log.Printf("Could not clean '%s': %s", m.experimentConfig.ExperimentDir, err)
		}
	}

	m.cleanUpGlobalVariables()
}

func (m *ArtemisManager) spawnLocalTracker() {
	m.logger.Printf("Starting tracking for '%s'", m.experimentConfig.Config.ExperimentName)
	m.since = time.Now()

	m.artemisWg.Add(1)
	go func() {
		err := m.artemisCmd.Run()
		m.tearDownExperiment(err)
		m.artemisWg.Done()
	}()
}

func (m *ArtemisManager) buildTrackingCommand() *exec.Cmd {
	args := m.experimentConfig.TrackingCommandArgs()
	cmd := exec.Command("artemis", args...)
	cmd.Stderr = nil
	cmd.Stdin = nil
	return cmd
}

func newExperimentLog(hasError bool,
	startTime time.Time,
	experimentConfig *leto.TrackingConfiguration,
	experimentDir string) *letopb.ExperimentLog {

	endTime := time.Now()

	log, err := ioutil.ReadFile(filepath.Join(experimentDir, "artemis.INFO"))
	if err != nil {
		toAdd := fmt.Sprintf("Could not read log: %s", err)

		log = append(log, []byte(toAdd)...)
	}

	stderr, err := ioutil.ReadFile(filepath.Join(experimentDir, "artemis.stderr"))
	if err != nil {
		toAdd := fmt.Sprintf("Could not read stderr: %s", err)
		stderr = append(stderr, []byte(toAdd)...)
	}

	yamlConfig, err := experimentConfig.Yaml()
	if err != nil {
		yamlConfig = []byte(fmt.Sprintf("Could not generate yaml config: %s", err))
	}

	return &letopb.ExperimentLog{
		HasError:          hasError,
		ExperimentDir:     filepath.Base(experimentDir),
		Start:             timestamppb.New(startTime),
		End:               timestamppb.New(endTime),
		YamlConfiguration: string(yamlConfig),
		Log:               string(log),
		Stderr:            string(stderr),
	}
}

func (m *ArtemisManager) persitentFilePath() string {
	return filepath.Join(xdg.DataHome, "fort/leto/current-experiment.yml")
}

func (m *ArtemisManager) writePersistentFile() {
	err := os.MkdirAll(filepath.Dir(m.persitentFilePath()), 0755)
	if err != nil {
		m.logger.Printf("Could not create data dir for '%s': %s", m.persitentFilePath(), err)
		return
	}
	configData, err := yaml.Marshal(m.experimentConfig)
	if err != nil {
		m.logger.Printf("Could not marshal config data to persistent file: %s", err)
		return
	}
	err = ioutil.WriteFile(m.persitentFilePath(), configData, 0644)
	if err != nil {
		m.logger.Printf("Could not write persitent config file: %s", err)
	}
}

func (m *ArtemisManager) removePersistentFile() {
	err := os.Remove(m.persitentFilePath())
	if err != nil {
		m.logger.Printf("Could not remove persitent file '%s': %s", m.persitentFilePath(), err)
	}
}

func (m *ArtemisManager) LoadFromPersistentFile() {
	configData, err := ioutil.ReadFile(m.persitentFilePath())
	if err != nil {
		// if there is no file, there is nothing to load
		return
	}
	config := &leto.TrackingConfiguration{}
	err = yaml.Unmarshal(configData, config)
	if err != nil {
		m.logger.Printf("Could not load configuration from '%s': %s", m.persitentFilePath(), err)
		return
	}
	m.logger.Printf("Restarting experiment from '%s'", m.persitentFilePath())
	err = m.Start(config)
	if err != nil {
		m.logger.Printf("Could not start experiment from '%s': %s", m.persitentFilePath(), err)
	}
}

func (m *ArtemisManager) registerOlympus() {
	if err := m.registerOlympusE(); err != nil {
		m.logger.Printf("olympus registration failure: %s", err)
	}
}

func (m *ArtemisManager) unregisterOlympus() {
	if err := m.unregisterOlympusE(); err != nil {
		m.logger.Printf("olympus unregistration failure: %s", err)
	}
}

func (m *ArtemisManager) registerOlympusE() (err error) {
	defer func() {
		if err == nil {
			return
		}
		m.stopRegistration = func() {}
		m.registrationEnded = nil
	}()

	if m.stopRegistration != nil || m.registrationEnded != nil {
		return errors.New("registration loop already started")
	}
	olympusHost := m.experimentConfig.Config.Stream.Host
	if olympusHost == nil || len(*olympusHost) == 0 {
		return errors.New("no olympus host defined in configuration")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("could not get hostname: %w", err)
	}
	m.logger.Printf("registring tracking to %s", *olympusHost)

	var c context.Context
	c, m.stopRegistration = context.WithCancel(context.Background())
	m.registrationEnded = make(chan struct{})

	declaration := &olympuspb.TrackingDeclaration{
		Hostname:       hostname,
		ExperimentName: m.experimentConfig.Config.ExperimentName,
		StreamServer:   *olympusHost,
	}

	go m.registrationLoop(c,
		m.registrationEnded,
		fmt.Sprintf("%s:%d", *olympusHost, m.letoConfig.OlympusPort),
		declaration)
	return nil
}

func (m *ArtemisManager) unregisterOlympusE() error {
	if m.registrationEnded == nil {
		return errors.New("already unregistered")
	}
	m.stopRegistration()
	<-m.registrationEnded
	m.stopRegistration = func() {}
	m.registrationEnded = nil
	return nil
}

func (m *ArtemisManager) registrationLoop(c context.Context,
	finished chan<- struct{},
	address string,
	declaration *olympuspb.TrackingDeclaration) {

	defer close(finished)

	conn := &olympuspb.TrackingConnection{}
	defer func() {
		conn.CloseAll(m.logger)
	}()

	dialOptions := []grpc.DialOption{
		grpc.WithConnectParams(
			grpc.ConnectParams{
				MinConnectTimeout: 20 * time.Second,
				Backoff: backoff.Config{
					BaseDelay:  500 * time.Millisecond,
					Multiplier: backoff.DefaultConfig.Multiplier,
					Jitter:     backoff.DefaultConfig.Jitter,
					MaxDelay:   2 * time.Second,
				},
			}),
	}
	connections, connErrors := olympuspb.ConnectTrackingAsync(nil,
		address,
		declaration,
		m.logger,
		dialOptions...)

	for {
		if conn.Established() == false && connErrors == nil && connections == nil {
			conn.CloseAll(m.logger)
			time.Sleep(time.Duration(float64(2*time.Second) * (1.0 + 0.2*rand.Float64())))
			m.logger.Printf("reconnection to %s", address)
			connections, connErrors = olympuspb.ConnectTrackingAsync(nil,
				address,
				declaration,
				m.logger,
				dialOptions...)
		}
		select {
		case err, ok := <-connErrors:
			if ok == false {
				connErrors = nil
			} else {
				m.logger.Printf("gRPC connection failure: %s", err)
			}
		case newConn, ok := <-connections:
			if ok == false {
				connections = nil
			} else {
				conn = newConn
			}
		case <-c.Done():
			return
		}
	}
}
