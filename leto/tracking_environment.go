package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/leto/letopb"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var artemisCommandName = "artemis"

// An TrackingEnvironment contains all configuration and information
// about a running tracking experiment. It can be SetUp() in order to
// initialize any needed resources (like directories where data would
// be put), and TearDown() will compute the letopb.ExperimentLog once
// done,
type TrackingEnvironment struct {
	Node           NodeConfiguration
	Config         *leto.TrackingConfiguration
	Balancing      *WorkloadBalance
	TestMode       bool
	ExperimentDir  string
	Leto           leto.Config
	Start          time.Time
	FreeStartBytes int64
	Context        context.Context
}

func NewExperimentConfiguration(ctx context.Context, leto leto.Config, node NodeConfiguration, user *leto.TrackingConfiguration) (*TrackingEnvironment, error) {
	tracking, err := finalizeTracking(user, node)
	if err != nil {
		return nil, err
	}

	balancing := newWorkloadBalance(tracking.Loads, *tracking.Camera.FPS)

	res := &TrackingEnvironment{
		Node:      node,
		Config:    tracking,
		Balancing: balancing,
		Leto:      leto,
		Context:   ctx,
	}

	res.setUpTestMode()
	if err := res.computeExperimentDir(); err != nil {
		return nil, err
	}

	return res, nil
}

func finalizeTracking(user *leto.TrackingConfiguration, node NodeConfiguration) (*leto.TrackingConfiguration, error) {
	tracking := leto.LoadDefaultConfig()
	if err := tracking.Merge(user); err != nil {
		return nil, fmt.Errorf("could not merge tracking configuration: %w", err)
	}

	if err := setUpLoadBalancing(tracking, node); err != nil {
		return nil, fmt.Errorf("could not setup load balancing: %w", err)
	}

	if err := tracking.CheckAllFieldAreSet(); err != nil {
		return nil, fmt.Errorf("incomplete tracking configuration: %w", err)
	}
	return tracking, nil
}

func setUpLoadBalancing(tracking *leto.TrackingConfiguration, node NodeConfiguration) error {
	if node.IsMaster() == false {
		return nil
	}
	tracking.Loads = generateLoadBalancing(node)
	if len(node.Slaves) == 0 {
		return nil
	}
	var err error
	tracking.Loads.Width, tracking.Loads.Height, err = fetchCameraResolution(tracking.Camera.StubPaths)
	return err
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

func fetchCameraResolution(stubPaths *[]string) (int, int, error) {
	cmd := exec.Command(artemisCommandName, "--fetch-resolution")
	if stubPaths != nil && len(*stubPaths) > 0 {
		cmd.Args = append(cmd.Args, "--stub-image-paths", strings.Join(*stubPaths, ","))
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("could not determine camera resolution from artemis: %s", err)
	}
	width := 0
	height := 0
	_, err = fmt.Sscanf(string(out), "%d %d", &width, &height)
	if err != nil {
		return 0, 0, fmt.Errorf("could not scan artemis output '%s': %w", string(out), err)
	}
	return width, height, nil

}

func newWorkloadBalance(lb *leto.LoadBalancing, FPS float64) *WorkloadBalance {
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

func (e *TrackingEnvironment) setUpTestMode() {
	if e.Config.ExperimentName == "" || e.Config.ExperimentName == "TEST-MODE" {
		e.TestMode = true
		e.Config.ExperimentName = "TEST-MODE"
	} else {
		e.TestMode = false
	}
}

func (e *TrackingEnvironment) experimentDestination() string {
	if e.TestMode == true {
		return filepath.Join(os.TempDir(), "fort-tests")
	}
	return filepath.Join(xdg.DataHome, "fort-experiments")
}

func (e *TrackingEnvironment) computeExperimentDir() error {
	basename := filepath.Join(e.experimentDestination(), e.Config.ExperimentName)
	var err error
	e.ExperimentDir, _, err = FilenameWithoutOverwrite(basename)
	return err
}

func (e *TrackingEnvironment) Path(p ...string) string {
	p = append([]string{e.ExperimentDir}, p...)
	return filepath.Join(p...)
}

func (e *TrackingEnvironment) newAntPath() string {
	return e.Path("ants")
}

func (e *TrackingEnvironment) TrackingCommandArgs() []string {
	args := []string{}

	targetHost := "localhost"
	if e.Node.IsMaster() == false {
		targetHost = strings.TrimPrefix(e.Node.Master, "leto.") + ".local"
	}

	if len(*e.Config.Camera.StubPaths) != 0 {
		args = append(args, "--stub-image-paths", strings.Join(*e.Config.Camera.StubPaths, ","))
	}

	if e.TestMode == true {
		args = append(args, "--test-mode")
	}
	args = append(args, "--host", targetHost)
	args = append(args, "--port", fmt.Sprintf("%d", e.Leto.ArtemisIncomingPort))
	args = append(args, "--uuid", e.Config.Loads.SelfUUID)

	if *e.Config.Threads > 0 {
		args = append(args, "--number-threads", fmt.Sprintf("%d", *e.Config.Threads))
	}

	if *e.Config.LegacyMode == true {
		args = append(args, "--legacy-mode")
	}
	args = append(args, "--camera-fps", fmt.Sprintf("%f", *e.Config.Camera.FPS))
	args = append(args, "--camera-strobe", fmt.Sprintf("%s", e.Config.Camera.StrobeDuration))
	args = append(args, "--camera-strobe-delay", fmt.Sprintf("%s", e.Config.Camera.StrobeDelay))
	args = append(args, "--at-family", *e.Config.Detection.Family)
	args = append(args, "--at-quad-decimate", fmt.Sprintf("%f", *e.Config.Detection.Quad.Decimate))
	args = append(args, "--at-quad-sigma", fmt.Sprintf("%f", *e.Config.Detection.Quad.Sigma))
	if *e.Config.Detection.Quad.RefineEdges == true {
		args = append(args, "--at-refine-edges")
	}
	args = append(args, "--at-quad-min-cluster", fmt.Sprintf("%d", *e.Config.Detection.Quad.MinClusterPixel))
	args = append(args, "--at-quad-max-n-maxima", fmt.Sprintf("%d", *e.Config.Detection.Quad.MaxNMaxima))
	args = append(args, "--at-quad-critical-radian", fmt.Sprintf("%f", *e.Config.Detection.Quad.CriticalRadian))
	args = append(args, "--at-quad-max-line-mse", fmt.Sprintf("%f", *e.Config.Detection.Quad.MaxLineMSE))
	args = append(args, "--at-quad-min-bw-diff", fmt.Sprintf("%d", *e.Config.Detection.Quad.MinBWDiff))
	if *e.Config.Detection.Quad.Deglitch == true {
		args = append(args, "--at-quad-deglitch")
	}

	if e.Node.IsMaster() == true {
		args = append(args, "--video-output-to-stdout")
		args = append(args, "--video-output-height", "1080")
		args = append(args, "--video-output-add-header")
		args = append(args, "--new-ant-output-dir", e.newAntPath(),
			"--new-ant-roi-size", fmt.Sprintf("%d", *e.Config.NewAntOutputROISize),
			"--image-renew-period", fmt.Sprintf("%s", e.Config.NewAntRenewPeriod))

	} else {
		args = append(args,
			"--camera-slave-width", fmt.Sprintf("%d", e.Config.Loads.Width),
			"--camera-slave-height", fmt.Sprintf("%d", e.Config.Loads.Height))
	}

	args = append(args, "--log-output-dir", e.ExperimentDir)

	if len(e.Balancing.IDsByUUID) > 1 {
		args = append(args, "--frame-stride", fmt.Sprintf("%d", len(e.Balancing.IDsByUUID)))
		ids := []string{}
		for i, isSet := range e.Balancing.IDsByUUID[e.Config.Loads.SelfUUID] {
			if isSet == false {
				continue
			}
			ids = append(ids, fmt.Sprintf("%d", i))
		}
		args = append(args, "--frame-ids", strings.Join(ids, ","))
	}

	tags := make([]string, 0, len(*e.Config.Highlights))
	for _, id := range *e.Config.Highlights {
		tags = append(tags, "0x"+strconv.FormatUint(uint64(id), 16))
	}

	if len(tags) != 0 {
		args = append(args, "--highlight-tags", strings.Join(tags, ","))
	}

	return args
}

func (e *TrackingEnvironment) SetUp() (*exec.Cmd, error) {
	defer func() {
		e.Start = time.Now()
	}()

	if err := e.makeAllDestinationDirs(); err != nil {
		return nil, err
	}

	if err := e.saveLocalConfig(); err != nil {
		return nil, err
	}

	var err error
	e.FreeStartBytes, _, err = fsStat(e.ExperimentDir)
	if err != nil {
		return nil, err
	}

	if e.FreeStartBytes < e.Leto.DiskLimit {
		return nil, fmt.Errorf("unsufficient disk space: available: %s minimum: %s",
			ByteSize(e.FreeStartBytes),
			ByteSize(e.Leto.DiskLimit))
	}

	return e.buildArtemisCommand()
}

func (e *TrackingEnvironment) makeAllDestinationDirs() error {
	target := e.ExperimentDir
	if e.Node.IsMaster() == true {
		target = e.newAntPath()
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("could not create %s: %w", target, err)
	}
	return nil
}

func (e *TrackingEnvironment) saveLocalConfig() error {
	return e.Config.WriteConfiguration(e.Path("leto-final-config.yaml"))
}

func (e *TrackingEnvironment) buildArtemisCommand() (*exec.Cmd, error) {
	cmd := exec.Command(artemisCommandName, e.TrackingCommandArgs()...)
	err := e.saveArtemisCommand(cmd)
	if err != nil {
		return nil, err
	}
	cmd.Stderr, err = os.Create(e.Path("artemis.stderr"))
	if err != nil {
		return nil, err
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	return cmd, nil
}

func (e *TrackingEnvironment) saveArtemisCommand(cmd *exec.Cmd) error {
	f, err := os.Create(e.Path("artemis.cmd"))
	if err != nil {
		return err
	}
	defer f.Close()
	values := append([]string{cmd.Path}, cmd.Args...)
	_, err = fmt.Fprintln(f, strings.Join(values, " "))
	return err
}

func (e *TrackingEnvironment) TearDown(hasError bool) (*letopb.ExperimentLog, error) {
	log := e.buildLog(hasError)
	return log, e.removeTestExperimentData()
}

func (e *TrackingEnvironment) removeTestExperimentData() error {
	if e.TestMode == false {
		return nil
	}
	return os.RemoveAll(e.ExperimentDir)
}

func (e *TrackingEnvironment) buildLog(hasError bool) *letopb.ExperimentLog {
	end := time.Now()
	log, err := ioutil.ReadFile(e.Path("artemis.INFO"))
	if err != nil {
		log = append(log, []byte(fmt.Sprintf("\ncould not read log: %s", err))...)
	}
	stderr, err := ioutil.ReadFile(e.Path("artemis.stderr"))
	if err != nil {
		stderr = append(stderr, []byte(fmt.Sprintf("\ncould not read stderr: %s", err))...)
	}

	yaml, err := e.Config.Yaml()
	if err != nil {
		yaml = []byte(fmt.Sprintf("could not generate yaml config: %s", err))
	}
	return &letopb.ExperimentLog{
		HasError:          hasError,
		ExperimentDir:     filepath.Base(e.ExperimentDir),
		Start:             timestamppb.New(e.Start),
		End:               timestamppb.New(end),
		YamlConfiguration: string(yaml),
		Log:               string(log),
		Stderr:            string(stderr),
	}
}

func (e *TrackingEnvironment) WatchDisk(now time.Time) (free int64, total int64, bps int64, err error) {
	if e.Start.Equal(time.Time{}) {
		return 0, 0, 0, errors.New("environment not setup")
	}

	free, total, err = fsStat(e.ExperimentDir)
	if err != nil {
		return free, total, 0, err
	}
	ellapsed := now.Sub(e.Start).Seconds()
	written := e.FreeStartBytes - free
	bps = int64(float64(written) / ellapsed)

	return free, total, bps, nil
}
