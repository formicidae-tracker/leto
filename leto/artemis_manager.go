package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/blang/semver"
	"github.com/formicidae-tracker/hermes"
	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/olympus/olympuspb"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"gopkg.in/yaml.v2"
)

type ArtemisManager struct {
	incoming, merged, file, broadcast chan *hermes.FrameReadout
	mx                                sync.Mutex
	wg, artemisWg, trackerWg          sync.WaitGroup
	fileWriter                        *FrameReadoutFileWriter
	trackers                          *RemoteManager
	nodeConfig                        NodeConfiguration

	artemisCmd    *exec.Cmd
	artemisOut    *io.PipeWriter
	streamIn      *io.PipeReader
	streamManager *StreamManager
	testMode      bool

	stopRegistration, registrationEnded chan struct{}

	experimentDir string
	logger        *log.Logger

	experimentConfig *leto.TrackingConfiguration
	workBalance      *WorkloadBalance
	since            time.Time

	lastExperimentLog *leto.ExperimentLog
}

func NewArtemisManager() (*ArtemisManager, error) {
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

	return &ArtemisManager{
		nodeConfig: nodeConfig,
		logger:     log.New(os.Stderr, "[artemis] ", 0),
	}, nil
}

func (m *ArtemisManager) Status() leto.Status {
	m.mx.Lock()
	defer m.mx.Unlock()
	res := leto.Status{
		Master:     m.nodeConfig.Master,
		Slaves:     m.nodeConfig.Slaves,
		Experiment: nil,
	}

	yamlConfig, err := m.experimentConfig.Yaml()
	if err != nil {
		yamlConfig = []byte(fmt.Sprintf("Could not generate yaml config: %s", err))
	}
	if m.incoming != nil {
		res.Experiment = &leto.ExperimentStatus{
			ExperimentDir:     filepath.Base(m.experimentDir),
			YamlConfiguration: string(yamlConfig),
			Since:             m.since,
		}
	}
	return res
}

func (m *ArtemisManager) LastExperimentLog() *leto.ExperimentLog {
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

	if err := m.setUpExperiment(userConfig); err != nil {
		return err
	}

	m.spawnTasks()

	m.registerOlympus()

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

	if m.artemisCmd != nil {
		if m.nodeConfig.IsMaster() == true {
			m.stopSlavesTrackers()
		}

		m.artemisCmd.Process.Signal(os.Interrupt)
		m.logger.Printf("Waiting for artemis process to stop")
		m.artemisCmd = nil
	}

	m.mx.Unlock()
	m.artemisWg.Wait()
	m.mx.Lock()
	return nil
}

func (m *ArtemisManager) SetMaster(hostname string) error {
	m.mx.Lock()
	defer m.mx.Unlock()
	if m.isStarted() == true {
		return fmt.Errorf("Could not change master/slave configuration while experiment %s is running", m.experimentConfig.ExperimentName)
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
		return fmt.Errorf("Could not change master/slave configuration while experiment %s is running", m.experimentConfig.ExperimentName)
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
		return fmt.Errorf("Could not change master/slave configuration while experiment %s is running", m.experimentConfig.ExperimentName)
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

func (m *ArtemisManager) setUpExperiment(userConfig *leto.TrackingConfiguration) error {
	if err := m.mergeConfiguration(userConfig); err != nil {
		return err
	}

	m.setUpTestMode()

	if err := m.setUpExperimentDir(); err != nil {
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

func (m *ArtemisManager) getExperimentDirName(expname string) (string, error) {
	if m.testMode == false {
		basename := filepath.Join(xdg.DataHome, "fort-experiments", expname)
		basedir, _, err := FilenameWithoutOverwrite(basename)
		return basedir, err
	}
	basename := filepath.Join(os.TempDir(), "fort-tests", expname)
	basedir, _, err := FilenameWithoutOverwrite(basename)
	return basedir, err
}

func generateLoadBalancing(c NodeConfiguration) *leto.LoadBalancing {
	if len(c.Slaves) == 0 {
		return &leto.LoadBalancing{
			SelfUUID:     "single-node",
			UUIDs:        map[string]string{"localhost": "single-node"},
			Assignements: map[int]string{0: "single-node"},
		}
	}
	res := &leto.LoadBalancing{
		SelfUUID:     uuid.New().String(),
		UUIDs:        make(map[string]string),
		Assignements: make(map[int]string),
	}
	res.UUIDs["localhost"] = res.SelfUUID
	res.Assignements[0] = res.SelfUUID
	for i, s := range c.Slaves {
		uuid := uuid.New().String()
		res.UUIDs[s] = uuid
		res.Assignements[i+1] = uuid
	}
	return res
}

func buildWorkloadBalance(lb *leto.LoadBalancing, FPS float64) *WorkloadBalance {
	wb := &WorkloadBalance{
		FPS:        FPS,
		MasterUUID: lb.UUIDs["localhost"],
		Stride:     len(lb.Assignements),
		IDsByUUID:  make(map[string][]bool),
	}

	for id, uuid := range lb.Assignements {
		if _, ok := wb.IDsByUUID[uuid]; ok == false {
			wb.IDsByUUID[uuid] = make([]bool, len(lb.Assignements))
		}
		wb.IDsByUUID[uuid][id] = true

	}
	return wb
}

func (m *ArtemisManager) setUpLoadBalancing() error {
	if m.nodeConfig.IsMaster() {
		m.experimentConfig.Loads = generateLoadBalancing(m.nodeConfig)
		if len(m.nodeConfig.Slaves) > 0 {
			cmd := exec.Command("artemis", "--fetch-resolution")

			if m.experimentConfig.Camera.StubPaths != nil || len(*m.experimentConfig.Camera.StubPaths) > 0 {
				cmd.Args = append(cmd.Args, "--stub-image-paths", strings.Join(*m.experimentConfig.Camera.StubPaths, ","))
			}

			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("Could not determine camera resolution: %s", err)
			}
			_, err = fmt.Sscanf(string(out), "%d %d", &m.experimentConfig.Loads.Width, &m.experimentConfig.Loads.Height)
			if err != nil {
				return fmt.Errorf("Could not parse camera resolution in '%s'", out)
			}
		}
	}
	//if not master the loads were sent by the master in the config
	return nil
}

func (m *ArtemisManager) mergeConfiguration(userConfig *leto.TrackingConfiguration) error {
	config := leto.LoadDefaultConfig()

	if err := config.Merge(userConfig); err != nil {
		return fmt.Errorf("could not merge user configuration: %s", err)
	}

	m.experimentConfig = config

	if err := m.setUpLoadBalancing(); err != nil {
		return err
	}

	if err := m.experimentConfig.CheckAllFieldAreSet(); err != nil {
		return fmt.Errorf("incomplete tracking configuration: %s", err)
	}

	m.workBalance = buildWorkloadBalance(config.Loads, *config.Camera.FPS)

	return nil
}

func (m *ArtemisManager) setUpSubTasksChannels() {
	m.merged = make(chan *hermes.FrameReadout, 10)
	m.file = make(chan *hermes.FrameReadout, 200)
	m.broadcast = make(chan *hermes.FrameReadout, 10)
}

func (m *ArtemisManager) setUpFileWriterTask() error {
	var err error
	m.fileWriter, err = NewFrameReadoutWriter(filepath.Join(m.experimentDir, "tracking.hermes"))
	return err
}

func (m *ArtemisManager) setUpStreamTask() error {
	var err error
	m.streamIn, m.artemisOut = io.Pipe()
	m.artemisCmd.Stdout = m.artemisOut
	m.streamManager, err = NewStreamManager(m.experimentDir, *m.experimentConfig.Camera.FPS/float64(m.workBalance.Stride), m.experimentConfig.Stream)
	return err
}

func (m *ArtemisManager) antOutputDir() string {
	return filepath.Join(m.experimentDir, "ants")
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

	m.trackers = NewRemoteManager()

	if err := m.setUpStreamTask(); err != nil {
		return err
	}

	return nil
}

func (m *ArtemisManager) setUpTestMode() {
	m.testMode = false
	if len(m.experimentConfig.ExperimentName) == 0 {
		m.logger.Printf("Starting in test mode")
		m.testMode = true
		// enforces display
		m.experimentConfig.ExperimentName = "TEST-MODE"
	} else {
		m.logger.Printf("New experiment '%s'", m.experimentConfig.ExperimentName)
	}

}

func (m *ArtemisManager) setUpExperimentDir() error {
	var err error
	m.experimentDir, err = m.getExperimentDirName(m.experimentConfig.ExperimentName)
	if err != nil {
		return err
	}
	return os.MkdirAll(m.experimentDir, 0755)
}

func (m *ArtemisManager) backUpConfigToExperimentDir() error {
	//save the config to the experiment dir
	confSaveName := filepath.Join(m.experimentDir, "leto-final-config.yml")
	return m.experimentConfig.WriteConfiguration(confSaveName)
}

func (m *ArtemisManager) setUpTrackerTask() error {
	logFilePath := filepath.Join(m.experimentDir, "artemis.command")
	artemisCommandLog, err := os.Create(logFilePath)
	if err != nil {
		return fmt.Errorf("Could not create artemis log file ('%s'): %s", logFilePath, err)
	}
	defer artemisCommandLog.Close()

	m.artemisCmd = m.buildTrackingCommand()
	m.artemisCmd.Stderr, err = os.Create(filepath.Join(m.experimentDir, "artemis.stderr"))
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
		MergeFrameReadout(m.workBalance, m.incoming, m.merged)
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
		err := m.trackers.Listen(fmt.Sprintf(":%d", leto.ARTEMIS_IN_PORT), m.onTrackerAccept(), func() {
			m.logger.Printf("All connection closed, cleaning up experiment")
		})
		if err != nil {
			m.logger.Printf("listening for tracker unhandled error: %s", err)
		}
		m.trackerWg.Done()
	}()
}

func (m *ArtemisManager) spawnFrameReadoutBroadCastTask() {
	m.wg.Add(1)
	go func() {
		BroadcastFrameReadout(fmt.Sprintf(":%d", leto.ARTEMIS_OUT_PORT),
			m.broadcast,
			3*time.Duration(1.0e6/(*m.experimentConfig.Camera.FPS))*time.Microsecond)
		m.wg.Done()
	}()
}

func (m *ArtemisManager) spawnFrameReadoutWriteTask() {
	m.wg.Add(1)
	go func() {
		m.fileWriter.WriteAll(m.file)
		m.wg.Done()
	}()
}

func (m *ArtemisManager) spawnStreamTask() {
	//TODO: setup waitgroup ? Was not done so maybe it was stopping
	//the application to work from a weird race condition. But it
	//should ultimately have some kind of synchronization
	go m.streamManager.EncodeAndStreamMuxedStream(m.streamIn)
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

		slaveConfig := *m.experimentConfig
		slaveConfig.Loads.SelfUUID = slaveConfig.Loads.UUIDs[slaveName]
		resp := leto.Response{}
		err := slave.RunMethod("Leto.StartTracking", &slaveConfig, &resp)
		if err == nil {
			err = resp.ToError()
		}
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
		resp := leto.Response{}
		err := slave.RunMethod("Leto.StopTracking", &leto.NoArgs{}, &resp)
		if err == nil {
			err = resp.ToError()
		}
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
	if m.trackers != nil {
		err := m.trackers.Close()
		if err != nil {
			m.logger.Printf("Could not close connections: %s", err)
		}
	}
	m.logger.Printf("Waiting for all tracker connections to be closed")

	m.trackerWg.Wait()
}

func (m *ArtemisManager) tearDownFilewriter() {
	if m.fileWriter != nil {
		m.fileWriter.Close()
	}
}

func (m *ArtemisManager) tearDownStreamTask() {
	if m.streamManager != nil {
		m.logger.Printf("Waiting for stream tasks to stop")
		m.artemisOut.Close()
		m.streamManager.Wait()
		m.streamManager = nil
		m.streamIn.Close()
		m.artemisOut = nil
		m.streamIn = nil
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
	m.streamIn = nil
	m.streamManager = nil
	m.experimentConfig = nil
	m.workBalance = nil
}

func (m *ArtemisManager) tearDownExperiment(err error) {
	m.mx.Lock()
	defer m.mx.Unlock()

	if err != nil {
		m.removePersistentFile()
	}

	m.lastExperimentLog = newExperimentLog(err != nil, m.since, m.experimentConfig, m.experimentDir)

	m.tearDownTrackerListenTask()
	m.tearDownSubTasks()

	m.logger.Printf("Experiment '%s' done", m.experimentConfig.ExperimentName)

	if m.testMode == true {
		log.Printf("Cleaning '%s'", m.experimentDir)
		if err := os.RemoveAll(m.experimentDir); err != nil {
			log.Printf("Could not clean '%s': %s", m.experimentDir, err)
		}
	}

	m.cleanUpGlobalVariables()
}

func (m *ArtemisManager) spawnLocalTracker() {
	m.logger.Printf("Starting tracking for '%s'", m.experimentConfig.ExperimentName)
	m.since = time.Now()

	m.artemisWg.Add(1)
	go func() {
		err := m.artemisCmd.Run()
		m.tearDownExperiment(err)
		m.artemisWg.Done()
	}()
}

func (m *ArtemisManager) buildTrackingCommand() *exec.Cmd {
	args := []string{}

	targetHost := "localhost"
	if m.nodeConfig.IsMaster() == false {
		targetHost = strings.TrimPrefix(m.nodeConfig.Master, "leto.") + ".local"
	}

	if len(*m.experimentConfig.Camera.StubPaths) != 0 {
		args = append(args, "--stub-image-paths", strings.Join(*m.experimentConfig.Camera.StubPaths, ","))
	}

	if m.testMode == true {
		args = append(args, "--test-mode")
	}
	args = append(args, "--host", targetHost)
	args = append(args, "--port", fmt.Sprintf("%d", leto.ARTEMIS_IN_PORT))
	args = append(args, "--uuid", m.experimentConfig.Loads.SelfUUID)

	if *m.experimentConfig.Threads > 0 {
		args = append(args, "--number-threads", fmt.Sprintf("%d", *m.experimentConfig.Threads))
	}

	if *m.experimentConfig.LegacyMode == true {
		args = append(args, "--legacy-mode")
	}
	args = append(args, "--camera-fps", fmt.Sprintf("%f", *m.experimentConfig.Camera.FPS))
	args = append(args, "--camera-strobe", fmt.Sprintf("%s", m.experimentConfig.Camera.StrobeDuration))
	args = append(args, "--camera-strobe-delay", fmt.Sprintf("%s", m.experimentConfig.Camera.StrobeDelay))
	args = append(args, "--at-family", *m.experimentConfig.Detection.Family)
	args = append(args, "--at-quad-decimate", fmt.Sprintf("%f", *m.experimentConfig.Detection.Quad.Decimate))
	args = append(args, "--at-quad-sigma", fmt.Sprintf("%f", *m.experimentConfig.Detection.Quad.Sigma))
	if *m.experimentConfig.Detection.Quad.RefineEdges == true {
		args = append(args, "--at-refine-edges")
	}
	args = append(args, "--at-quad-min-cluster", fmt.Sprintf("%d", *m.experimentConfig.Detection.Quad.MinClusterPixel))
	args = append(args, "--at-quad-max-n-maxima", fmt.Sprintf("%d", *m.experimentConfig.Detection.Quad.MaxNMaxima))
	args = append(args, "--at-quad-critical-radian", fmt.Sprintf("%f", *m.experimentConfig.Detection.Quad.CriticalRadian))
	args = append(args, "--at-quad-max-line-mse", fmt.Sprintf("%f", *m.experimentConfig.Detection.Quad.MaxLineMSE))
	args = append(args, "--at-quad-min-bw-diff", fmt.Sprintf("%d", *m.experimentConfig.Detection.Quad.MinBWDiff))
	if *m.experimentConfig.Detection.Quad.Deglitch == true {
		args = append(args, "--at-quad-deglitch")
	}

	if m.nodeConfig.IsMaster() == true {
		args = append(args, "--video-output-to-stdout")
		args = append(args, "--video-output-height", "1080")
		args = append(args, "--video-output-add-header")
		args = append(args, "--new-ant-output-dir", m.antOutputDir(),
			"--new-ant-roi-size", fmt.Sprintf("%d", *m.experimentConfig.NewAntOutputROISize),
			"--image-renew-period", fmt.Sprintf("%s", m.experimentConfig.NewAntRenewPeriod))

	} else {
		args = append(args,
			"--camera-slave-width", fmt.Sprintf("%d", m.experimentConfig.Loads.Width),
			"--camera-slave-height", fmt.Sprintf("%d", m.experimentConfig.Loads.Height))
	}

	args = append(args, "--log-output-dir", m.experimentDir)

	if len(m.workBalance.IDsByUUID) > 1 {
		args = append(args, "--frame-stride", fmt.Sprintf("%d", len(m.workBalance.IDsByUUID)))
		ids := []string{}
		for i, isSet := range m.workBalance.IDsByUUID[m.experimentConfig.Loads.SelfUUID] {
			if isSet == false {
				continue
			}
			ids = append(ids, fmt.Sprintf("%d", i))
		}
		args = append(args, "--frame-ids", strings.Join(ids, ","))
	}

	tags := make([]string, 0, len(*m.experimentConfig.Highlights))
	for _, id := range *m.experimentConfig.Highlights {
		tags = append(tags, "0x"+strconv.FormatUint(uint64(id), 16))
	}

	if len(tags) != 0 {
		args = append(args, "--highlight-tags", strings.Join(tags, ","))
	}

	cmd := exec.Command("artemis", args...)
	cmd.Stderr = nil
	cmd.Stdin = nil
	return cmd
}

func (m *ArtemisManager) onTrackerAccept() func(c net.Conn) {
	return func(c net.Conn) {
		errors := make(chan error)
		logger := log.New(os.Stderr, fmt.Sprintf("[artemis/%s] ", c.RemoteAddr().String()), 0)
		logger.Printf("new connection from %s", c.RemoteAddr().String())
		go func() {
			for e := range errors {
				logger.Printf("unhandled error: %s", e)
			}
		}()
		FrameReadoutReadAll(c, m.incoming, errors)
	}
}

func newExperimentLog(hasError bool,
	startTime time.Time,
	experimentConfig *leto.TrackingConfiguration,
	experimentDir string) *leto.ExperimentLog {

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

	return &leto.ExperimentLog{
		HasError:          hasError,
		ExperimentDir:     filepath.Base(experimentDir),
		Start:             startTime,
		End:               endTime,
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
		m.stopRegistration = nil
		m.registrationEnded = nil
	}()

	if m.stopRegistration != nil || m.registrationEnded != nil {
		return errors.New("registration loop already started")
	}
	m.stopRegistration = make(chan struct{})
	m.registrationEnded = make(chan struct{})
	olympusHost := m.experimentConfig.Stream.Host
	if olympusHost == nil || len(*olympusHost) == 0 {
		return errors.New("no olympus host defined in configuration")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("could not get hostname: %w", err)
	}
	m.logger.Printf("registring tracking to %s", *olympusHost)

	declaration := &olympuspb.TrackingDeclaration{
		Hostname:       hostname,
		ExperimentName: m.experimentConfig.ExperimentName,
		StreamServer:   *olympusHost,
	}

	go m.registrationLoop(m.stopRegistration,
		m.registrationEnded,
		fmt.Sprintf("%s:%d", *olympusHost, 3001),
		declaration)
	return nil
}

func (m *ArtemisManager) unregisterOlympusE() error {
	if m.registrationEnded == nil {
		return errors.New("already unregistered")
	}
	close(m.stopRegistration)
	<-m.registrationEnded
	m.stopRegistration = nil
	m.registrationEnded = nil
	return nil
}

type connection struct {
	conn   *grpc.ClientConn
	stream olympuspb.Olympus_TrackingClient
}

func (c *connection) closeAndLogError(logger *log.Logger) {
	if c.stream != nil {
		err := c.stream.CloseSend()
		if err != nil {
			logger.Printf("gRPC CloseSend() failure: %s", err)
		}
	}
	c.stream = nil
	if c.conn != nil {
		err := c.conn.Close()
		if err != nil {
			logger.Printf("gRPC close() failure: %s", err)
		}
	}
	c.conn = nil
}

func (m *ArtemisManager) connect(address string, declaration *olympuspb.TrackingDeclaration) (res connection, err error) {
	defer func() {
		if err == nil {
			return
		}
		res.closeAndLogError(m.logger)
	}()
	dialOptions := append(olympuspb.DefaultDialOptions,
		grpc.WithConnectParams(
			grpc.ConnectParams{
				MinConnectTimeout: 20 * time.Second,
				Backoff: backoff.Config{
					BaseDelay:  500 * time.Millisecond,
					Multiplier: backoff.DefaultConfig.Multiplier,
					Jitter:     backoff.DefaultConfig.Jitter,
					MaxDelay:   2 * time.Second,
				},
			}))
	res.conn, err = grpc.Dial(address, dialOptions...)
	if err != nil {
		return
	}

	client := olympuspb.NewOlympusClient(res.conn)

	res.stream, err = client.Tracking(context.Background(), olympuspb.DefaultCallOptions...)
	if err != nil {
		return
	}
	err = res.stream.Send(&olympuspb.TrackingUpStream{
		Declaration: declaration,
	})
	if err != nil {
		return
	}
	_, err = res.stream.Recv()
	return
}

func (m *ArtemisManager) connectAsync(address string, declaration *olympuspb.TrackingDeclaration) (<-chan connection, <-chan error) {
	errors := make(chan error)
	res := make(chan connection)
	go func() {
		conn, err := m.connect(address, declaration)
		if err != nil {
			errors <- err
		} else {
			res <- conn
		}
		close(res)
		close(errors)

	}()
	return res, errors
}

func (m *ArtemisManager) registrationLoop(quit <-chan struct{},
	finished chan<- struct{},
	address string,
	declaration *olympuspb.TrackingDeclaration) {

	defer close(finished)

	var conn connection
	defer conn.closeAndLogError(m.logger)
	conns, errors := m.connectAsync(address, declaration)

	for {
		if conn.stream == nil && errors == nil && conns == nil {
			conn.closeAndLogError(m.logger)
			time.Sleep(500 * time.Millisecond)
			m.logger.Printf("reconnection to %s", address)
			conns, errors = m.connectAsync(address, declaration)
		}
		select {
		case err, ok := <-errors:
			if ok == false {
				errors = nil
			} else {
				m.logger.Printf("gRPC connect failure: %s", err)
			}
		case newConn, ok := <-conns:
			if ok == false {
				conns = nil
			} else {
				conn = newConn
			}
		case <-quit:
			return
		}
	}
}
