package main

import (
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
)

func NewLastExperimentStatus(hasError bool,
	startTime time.Time,
	config *leto.TrackingConfiguration,
	experimentDir string) *leto.LastExperimentStatus {
	endTime := time.Now()

	log, err := ioutil.ReadFile(filepath.Join(experimentDir, "artemis.INFO"))
	if err != nil {
		toAdd := fmt.Sprintf("Could not read log: %s", err)

		log = append(log, []byte(toAdd)...)
	}

	return &leto.LastExperimentStatus{
		HasError:      hasError,
		ExperimentDir: filepath.Base(experimentDir),
		Start:         startTime,
		End:           endTime,
		Config:        *config,
		Log:           log,
	}
}

type ArtemisManager struct {
	incoming, merged, file, broadcast chan *hermes.FrameReadout
	mx                                sync.Mutex
	wg, artemisWg                     sync.WaitGroup
	quitEncode                        chan struct{}
	fileWriter                        *FrameReadoutFileWriter
	trackers                          *RemoteManager
	isMaster                          bool
	config                            *leto.TrackingConfiguration

	artemisCmd    *exec.Cmd
	artemisOut    *io.PipeWriter
	streamIn      *io.PipeReader
	streamManager *StreamManager
	testMode      bool

	experimentDir string
	logger        *log.Logger

	experimentName string
	since          time.Time

	workloadBalance   *WorkloadBalance
	artemisCommandLog io.WriteCloser

	lastExperiment *leto.LastExperimentStatus
}

func CheckArtemisVersion(actual, minimal string) error {
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

func extractCoaxlinkFirmwareOutput(output []byte) (string, error) {
	rx := regexp.MustCompile(`Firmware variant:\W+[0-9]+\W+\(([0-9a-z\-]+)\)`)
	m := rx.FindStringSubmatch(string(output))
	if len(m) == 0 {
		return "", fmt.Errorf("Could not determine firmware variant in output: '%s'", output)
	}
	return m[1], nil

}

func getFirmwareVariant() (string, error) {
	cmd := exec.Command("coaxlink-firmware")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Could not check slave firmware variant")
	}

	return extractCoaxlinkFirmwareOutput(output)
}

func getAndCheckFirmwareVariant() (bool, error) {
	variant, err := getFirmwareVariant()
	if err != nil {
		return false, err
	}
	return checkFirmwareVariant(variant)
}

func checkFirmwareVariant(variant string) (bool, error) {
	switch variant {
	case "1-camera":
		return true, nil
	case "1-df-camera":
		return false, nil
	}

	return false, fmt.Errorf("Unknown firmware variant '%s'", variant)
}

func NewArtemisManager() (*ArtemisManager, error) {
	cmd := exec.Command("artemis", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Could not find artemis: %s", err)
	}

	artemisVersion := strings.TrimPrefix(strings.TrimSpace(string(output)), "artemis ")
	err = CheckArtemisVersion(artemisVersion, leto.ARTEMIS_MIN_VERSION)
	if err != nil {
		return nil, err
	}

	cmd = exec.Command("ffmpeg", "-version")
	_, err = cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Could not find ffmpeg: %s", err)
	}

	isMaster, err := getAndCheckFirmwareVariant()
	if err != nil {
		return nil, err
	}

	return &ArtemisManager{
		isMaster: isMaster,
		logger:   log.New(os.Stderr, "[artemis] ", log.LstdFlags),
	}, nil
}

func (m *ArtemisManager) Status() *leto.Status {
	m.mx.Lock()
	defer m.mx.Unlock()
	if m.incoming == nil {
		return nil
	}
	return &leto.Status{
		Since:         m.since,
		Configuration: *m.config,
		ExperimentDir: filepath.Base(m.experimentDir),
	}
}

func (m *ArtemisManager) LastExperimentStatus() *leto.LastExperimentStatus {
	m.mx.Lock()
	defer m.mx.Unlock()
	return m.lastExperiment
}

func (m *ArtemisManager) ExperimentDir(expname string) (string, error) {
	if m.testMode == false {
		basename := filepath.Join(xdg.DataHome, "fort-experiments", expname)
		basedir, _, err := FilenameWithoutOverwrite(basename)
		return basedir, err
	}
	basename := filepath.Join(os.TempDir(), "fort-tests", expname)
	basedir, _, err := FilenameWithoutOverwrite(basename)
	return basedir, err
}

func BuildWorkloadBalance(lb *leto.LoadBalancingConfiguration, FPS float64) *WorkloadBalance {
	wb := &WorkloadBalance{
		FPS:        FPS,
		MasterUUID: lb.UUIDs[lb.Master],
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

func (m *ArtemisManager) IsRunning() bool {
	return m.incoming != nil
}

func (m *ArtemisManager) InsertResolutionInConfig() error {
	if m.isMaster == false {
		return nil
	}
	// test if there is only a single node and it is the master (therefore us)!!!
	_, ok := m.config.Loads.UUIDs[m.config.Loads.Master]
	if len(m.config.Loads.UUIDs) == 1 && ok == true {
		return nil
	}

	cmd := exec.Command("artemis", "--fetch-resolution")

	if m.config.Camera.StubPaths != nil || len(*m.config.Camera.StubPaths) > 0 {
		cmd.Args = append(cmd.Args, "--stub-image-paths", strings.Join(*m.config.Camera.StubPaths, ","))
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Could not determine camera resolution: %s", err)
	}
	_, err = fmt.Sscanf(string(out), "%d %d", &m.config.Loads.Width, &m.config.Loads.Height)
	if err != nil {
		return fmt.Errorf("Could not parse camera resolution in '%s'", out)
	}

	return nil
}

func (m *ArtemisManager) SetUpTestMode() {
	m.testMode = false
	if len(m.config.ExperimentName) == 0 {
		m.logger.Printf("Starting in test mode")
		m.testMode = true
		m.config.ExperimentName = "TEST-MODE"
	} else {
		m.logger.Printf("New experiment '%s'", m.config.ExperimentName)
	}
}

func (m *ArtemisManager) SetUpChannels() {
	m.incoming = make(chan *hermes.FrameReadout, 10)

	if m.isMaster == false {
		return
	}

	m.merged = make(chan *hermes.FrameReadout, 10)
	m.file = make(chan *hermes.FrameReadout, 200)
	m.broadcast = make(chan *hermes.FrameReadout, 10)
}

func (m *ArtemisManager) SetUpExperimentDir() error {
	var err error
	m.experimentDir, err = m.ExperimentDir(m.config.ExperimentName)
	if err != nil {
		return err
	}
	return os.MkdirAll(m.experimentDir, 0755)
}

func (m *ArtemisManager) SaveConfiguration() error {
	//save the config to the experiment dir
	confSaveName := filepath.Join(m.experimentDir, "leto-final-config.yml")
	return m.config.WriteToFile(confSaveName)
}

func (m *ArtemisManager) SetUpFrameReadoutWriter() error {
	if m.isMaster == false {
		return nil
	}
	var err error
	m.fileWriter, err = NewFrameReadoutWriter(filepath.Join(m.experimentDir, "tracking.hermes"))
	return err
}

func (m *ArtemisManager) SetUpRemoteManager() {
	if m.isMaster == false {
		return
	}

	m.trackers = NewRemoteManager()

}

func (m *ArtemisManager) SpawnFrameReadoutDispatchTask() {
	if m.isMaster == false {
		return
	}

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

func (m *ArtemisManager) SpawnFrameReadoutMergeTask() {
	if m.isMaster == false {
		return
	}

	m.wg.Add(1)
	go func() {
		MergeFrameReadout(m.workloadBalance, m.incoming, m.merged)
		m.wg.Done()
	}()
}

func (m *ArtemisManager) SpawnListenTrackerTask() {
	if m.isMaster == false {
		return
	}

	m.wg.Add(1)
	go func() {
		err := m.trackers.Listen(fmt.Sprintf(":%d", leto.ARTEMIS_IN_PORT), m.OnAcceptTracker(), func() {
			m.logger.Printf("All connection closed, cleaning up experiment")
			close(m.incoming)
			m.mx.Lock()
			defer m.mx.Unlock()
			m.incoming = nil
		})
		if err != nil {
			m.logger.Printf("listening for tracker unhandled error: %s", err)
		}
		m.wg.Done()
	}()
}

func (m *ArtemisManager) SpawnBroadcastTask() {
	if m.isMaster == false {
		return
	}
	m.wg.Add(1)
	go func() {
		BroadcastFrameReadout(fmt.Sprintf(":%d", leto.ARTEMIS_OUT_PORT),
			m.broadcast,
			3*time.Duration(1.0e6/(*m.config.Camera.FPS))*time.Microsecond)
		m.wg.Done()
	}()
}

func (m *ArtemisManager) SpawnWriteFileTask() {
	if m.isMaster == false {
		return
	}
	m.wg.Add(1)
	go func() {
		m.fileWriter.WriteAll(m.file)
		m.wg.Done()
	}()
}

func (m *ArtemisManager) SpawnLocalSubTasks() {
	m.SpawnFrameReadoutDispatchTask()
	m.SpawnFrameReadoutMergeTask()
	m.SpawnListenTrackerTask()
	m.SpawnBroadcastTask()
	m.SpawnWriteFileTask()
}

func (m *ArtemisManager) SetupArtemisCommand() error {
	m.artemisCmd = m.TrackingCommand(m.config, m.workloadBalance)
	var err error
	m.artemisCmd.Stderr, err = os.Create(filepath.Join(m.experimentDir, "artemis.stderr"))
	if err != nil {
		return err
	}
	m.artemisCmd.Stdin = nil
	return nil
}

func (m *ArtemisManager) AntOutputDir() string {
	return filepath.Join(m.experimentDir, "ants")
}

func (m *ArtemisManager) SetUpAntOutputDir() error {
	if m.isMaster == false {
		return nil
	}
	return os.MkdirAll(m.AntOutputDir(), 0755)
}

func (m *ArtemisManager) SetUpStreamManager() error {
	if m.isMaster == true {
		m.artemisCmd.Args = append(m.artemisCmd.Args, "--new-ant-output-dir", m.AntOutputDir(),
			"--new-ant-roi-size", fmt.Sprintf("%d", *m.config.NewAntOutputROISize),
			"--ant-renew-period-hour", fmt.Sprintf("%f", m.config.ImageRenewPeriod.Hours()))
		m.streamIn, m.artemisOut = io.Pipe()
		m.artemisCmd.Stdout = m.artemisOut
		var err error
		m.streamManager, err = NewStreamManager(m.experimentDir, *m.config.Camera.FPS/float64(m.workloadBalance.Stride), m.config.Stream)
		if err != nil {
			return err
		}
		go m.streamManager.EncodeAndStreamMuxedStream(m.streamIn)
	} else {
		m.artemisCmd.Stdout = nil
		m.artemisCmd.Args = append(m.artemisCmd.Args,
			"--camera-slave-width", fmt.Sprintf("%d", m.config.Loads.Width),
			"--camera-slave-height", fmt.Sprintf("%d", m.config.Loads.Height))
	}
	return nil
}

func (m *ArtemisManager) SetUpArtemisCommandLog() (io.WriteCloser, error) {
	logCommandPath := filepath.Join(m.experimentDir, "artemis.command")
	artemisCommandLog, err := os.Create(logCommandPath)
	if err != nil {
		return nil, fmt.Errorf("Could not create artemis log file ('%s'): %s", logCommandPath, err)
	}
	return artemisCommandLog, nil
}

func (m *ArtemisManager) SpawnSlaves() {
	if m.isMaster == false {
		return
	}

	for slaveName, UUID := range m.config.Loads.UUIDs {
		slaveConfig := *m.config
		slaveConfig.Loads.SelfUUID = UUID
		resp := leto.Response{}
		_, _, err := leto.RunForHost(slaveName, "Leto.StartTracking", &slaveConfig, &resp)
		if err == nil {
			err = resp.ToError()
		}
		if err != nil {
			m.logger.Printf("Could not start slave %s: %s", slaveName, err)
		}
	}
}

func (m *ArtemisManager) SpawnTracker() {
	m.logger.Printf("Starting tracking for '%s'", m.config.ExperimentName)
	m.experimentName = m.config.ExperimentName
	m.since = time.Now()
	fmt.Fprintf(m.artemisCommandLog, "%s %s\n", m.artemisCmd.Path, m.artemisCmd.Args)
	m.artemisCommandLog.Close()
	m.artemisCommandLog = nil
	m.artemisWg.Add(1)
	go func() {
		defer m.artemisWg.Done()
		err := m.artemisCmd.Run()
		m.mx.Lock()
		defer m.mx.Unlock()

		if err != nil {
			m.logger.Printf("artemis child process exited with error: %s", err)
		}

		m.lastExperiment = NewLastExperimentStatus(err != nil, m.since, m.config, m.experimentDir)

		m.artemisCmd = nil
		//Stops the reading of frame readout, it will close all the chain
		if m.trackers != nil {
			err = m.trackers.Close()
			if err != nil {
				m.logger.Printf("Could not close connections: %s", err)
			}
		}

		log.Printf("Waiting for all connection to be closed")
		m.mx.Unlock()
		m.wg.Wait()
		if m.fileWriter != nil {
			m.fileWriter.Close()
		}
		m.mx.Lock()

		if m.streamManager != nil {
			m.logger.Printf("Waiting for stream tasks to stop")
			m.artemisOut.Close()
			m.streamManager.Wait()
			m.streamManager = nil
			m.streamIn.Close()
			m.artemisOut = nil
			m.streamIn = nil
		}

		m.config = nil
		m.incoming = nil
		m.merged = nil
		m.file = nil
		m.broadcast = nil
		m.logger.Printf("Experiment '%s' done", m.experimentName)

		if m.testMode == true {
			log.Printf("Cleaning '%s'", m.experimentDir)
			if err := os.RemoveAll(m.experimentDir); err != nil {
				log.Printf("Could not clean '%s': %s", m.experimentDir, err)
			}
		}
	}()
}

func (m *ArtemisManager) SetUpExperiment(config *leto.TrackingConfiguration) error {
	m.InsertResolutionInConfig()
	m.config = config
	m.workloadBalance = BuildWorkloadBalance(config.Loads, *config.Camera.FPS)

	m.SetUpTestMode()
	err := m.SetUpExperimentDir()
	if err != nil {
		return err
	}
	err = m.SaveConfiguration()
	if err != nil {
		return err
	}
	err = m.SetUpFrameReadoutWriter()
	if err != nil {
		return err
	}
	m.SetUpRemoteManager()
	artemisCommandLog, err := m.SetUpArtemisCommandLog()
	if err != nil {
		return err
	}
	defer artemisCommandLog.Close()
	err = m.SetUpAntOutputDir()
	if err != nil {
		return err
	}
	err = m.SetUpStreamManager()
	if err != nil {
		return err
	}

	//only setup channels (that indicate a running experiment) when everything is fine
	m.SetUpChannels()
	return nil
}

func (m *ArtemisManager) Start(config *leto.TrackingConfiguration) error {
	m.mx.Lock()
	defer m.mx.Unlock()
	if m.IsRunning() == true {
		return fmt.Errorf("ArtemisManager: Start: already started")
	}
	err := m.SetUpExperiment(config)
	if err != nil {
		return err
	}

	m.SpawnLocalSubTasks()

	m.SpawnSlaves()

	m.SpawnTracker()

	return nil
}

func (m *ArtemisManager) StopSlaves() {
	if m.isMaster == false {
		return
	}
	for slaveName, _ := range m.config.Loads.UUIDs {
		if slaveName == m.config.Loads.Master {
			continue
		}
		resp := leto.Response{}
		_, _, err := leto.RunForHost(slaveName, "Leto.StopTracking", &leto.TrackingStop{}, &resp)
		if err == nil {
			err = resp.ToError()
		}
		if err != nil {
			m.logger.Printf("Could not stop slave %s: %s", slaveName, err)
		}
	}
}

func (m *ArtemisManager) Stop() error {
	m.mx.Lock()
	defer m.mx.Unlock()

	if m.incoming == nil {
		return fmt.Errorf("Already stoppped")
	}

	if m.artemisCmd != nil {
		m.StopSlaves()

		m.artemisCmd.Process.Signal(os.Interrupt)
		m.logger.Printf("Waiting for artemis process to stop")
		m.artemisCmd = nil
	}

	m.mx.Unlock()
	m.artemisWg.Wait()
	m.mx.Lock()
	return nil
}

func (m *ArtemisManager) TrackingCommand(config *leto.TrackingConfiguration, wb *WorkloadBalance) *exec.Cmd {
	targetHost := config.Loads.Master + ".local"
	args := []string{}

	if len(*config.Camera.StubPaths) != 0 {
		args = append(args, "--stub-image-paths", strings.Join(*m.config.Camera.StubPaths, ","))
	}

	if m.testMode == true {
		args = append(args, "--test-mode")
	}
	args = append(args, "--host", targetHost)
	args = append(args, "--port", fmt.Sprintf("%d", leto.ARTEMIS_IN_PORT))
	args = append(args, "--uuid", config.Loads.SelfUUID)

	if *config.LegacyMode == true {
		args = append(args, "--legacy-mode")
	}
	args = append(args, "--camera-fps", fmt.Sprintf("%f", *config.Camera.FPS))
	args = append(args, "--camera-strobe-us", fmt.Sprintf("%d", config.Camera.StrobeDuration.Nanoseconds()/1000))
	args = append(args, "--camera-strobe-delay-us", fmt.Sprintf("%d", config.Camera.StrobeDelay.Nanoseconds()/1000))
	args = append(args, "--at-family", *config.Detection.Family)
	args = append(args, "--at-quad-decimate", fmt.Sprintf("%f", *config.Detection.Quad.Decimate))
	args = append(args, "--at-quad-sigma", fmt.Sprintf("%f", *config.Detection.Quad.Sigma))
	if *config.Detection.Quad.RefineEdges == true {
		args = append(args, "--at-refine-edges")
	}
	args = append(args, "--at-quad-min-cluster", fmt.Sprintf("%d", *config.Detection.Quad.MinClusterPixel))
	args = append(args, "--at-quad-max-n-maxima", fmt.Sprintf("%d", *config.Detection.Quad.MaxNMaxima))
	args = append(args, "--at-quad-critical-radian", fmt.Sprintf("%f", *config.Detection.Quad.CriticalRadian))
	args = append(args, "--at-quad-max-line-mse", fmt.Sprintf("%f", *config.Detection.Quad.MaxLineMSE))
	args = append(args, "--at-quad-min-bw-diff", fmt.Sprintf("%d", *config.Detection.Quad.MinBWDiff))
	if *config.Detection.Quad.Deglitch == true {
		args = append(args, "--at-quad-deglitch")
	}

	if m.isMaster == true {
		args = append(args, "--video-to-stdout")
		args = append(args, "--video-output-height", "1080")
		args = append(args, "--video-output-add-header")
	}

	if len(wb.IDsByUUID) > 1 {
		args = append(args, "--frame-stride", fmt.Sprintf("%d", len(wb.IDsByUUID)))
		ids := []string{}
		for i, isSet := range wb.IDsByUUID[config.Loads.SelfUUID] {
			if isSet == false {
				continue
			}
			ids = append(ids, fmt.Sprintf("%d", i))
		}
		args = append(args, "--frame-ids", strings.Join(ids, ","))
	}

	tags := make([]string, 0, len(*config.Highlights))
	for _, id := range *config.Highlights {
		tags = append(tags, "0x"+strconv.FormatUint(uint64(id), 16))
	}

	if len(tags) != 0 {
		m.artemisCmd.Args = append(m.artemisCmd.Args, "--highlight-tags", strings.Join(tags, ","))
	}

	args = append(args, "--log-output-dir", m.experimentDir)

	cmd := exec.Command("artemis", args...)
	cmd.Stderr = nil
	cmd.Stdin = nil
	return cmd
}

func (m *ArtemisManager) OnAcceptTracker() func(c net.Conn) {
	return func(c net.Conn) {
		errors := make(chan error)
		logger := log.New(os.Stderr, fmt.Sprintf("[artemis/%s] ", c.RemoteAddr().String()), log.LstdFlags)
		logger.Printf("new connection from %s", c.RemoteAddr().String())
		go func() {
			for e := range errors {
				logger.Printf("unhandled error: %s", e)
			}
		}()
		FrameReadoutReadAll(c, m.incoming, errors)
	}
}
