package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adrg/xdg"
	"github.com/formicidae-tracker/leto"
	"github.com/google/uuid"
)

// An ExperimentConfiguration contains all information on an actual
// Experiment to be run by leto. It merges the leto.Config,
// NodeConfiguration and WorkloadBalance, and the actual destination
// on the disk.
type ExperimentConfiguration struct {
	Node                NodeConfiguration
	Tracking            *leto.TrackingConfiguration
	Balancing           *WorkloadBalance
	TestMode            bool
	ExperimentDir       string
	ArtemisIncomingPort int
}

func NewExperimentConfiguration(leto leto.Config, node NodeConfiguration, user *leto.TrackingConfiguration) (*ExperimentConfiguration, error) {
	tracking, err := finalizeTracking(user, node)
	if err != nil {
		return nil, err
	}

	balancing := newWorkloadBalance(tracking.Loads, *tracking.Camera.FPS)

	res := &ExperimentConfiguration{
		Node:                node,
		Tracking:            tracking,
		Balancing:           balancing,
		ArtemisIncomingPort: leto.ArtemisIncomingPort,
	}

	res.setUpTestMode()
	if err := res.computeExperimentDir(); err != nil {
		return nil, err
	}

	return nil, errors.New("not yet implemented")
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
	cmd := exec.Command("artemis", "--fetch-resolution")
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

func (c *ExperimentConfiguration) setUpTestMode() {
	if c.Tracking.ExperimentName == "" || c.Tracking.ExperimentName == "TEST-MODE" {
		c.TestMode = true
		c.Tracking.ExperimentName = "TEST-MODE"
	} else {
		c.TestMode = false
	}
}

func (c *ExperimentConfiguration) experimentDestination() string {
	if c.TestMode == true {
		return filepath.Join(os.TempDir(), "fort-tests")
	}
	return filepath.Join(xdg.DataHome, "fort-experiments")
}

func (c *ExperimentConfiguration) computeExperimentDir() error {
	basename := filepath.Join(c.experimentDestination(), c.Tracking.ExperimentName)
	var err error
	c.ExperimentDir, _, err = FilenameWithoutOverwrite(basename)
	return err
}

func (c *ExperimentConfiguration) Path(p ...string) string {
	p = append([]string{c.ExperimentDir}, p...)
	return filepath.Join(p...)
}

func (c *ExperimentConfiguration) TrackingCommandArgs() []string {
	args := []string{}

	targetHost := "localhost"
	if c.Node.IsMaster() == false {
		targetHost = strings.TrimPrefix(c.Node.Master, "leto.") + ".local"
	}

	if len(*c.Tracking.Camera.StubPaths) != 0 {
		args = append(args, "--stub-image-paths", strings.Join(*c.Tracking.Camera.StubPaths, ","))
	}

	if c.TestMode == true {
		args = append(args, "--test-mode")
	}
	args = append(args, "--host", targetHost)
	args = append(args, "--port", fmt.Sprintf("%d", c.ArtemisIncomingPort))
	args = append(args, "--uuid", c.Tracking.Loads.SelfUUID)

	if *c.Tracking.Threads > 0 {
		args = append(args, "--number-threads", fmt.Sprintf("%d", *c.Tracking.Threads))
	}

	if *c.Tracking.LegacyMode == true {
		args = append(args, "--legacy-mode")
	}
	args = append(args, "--camera-fps", fmt.Sprintf("%f", *c.Tracking.Camera.FPS))
	args = append(args, "--camera-strobe", fmt.Sprintf("%s", c.Tracking.Camera.StrobeDuration))
	args = append(args, "--camera-strobe-delay", fmt.Sprintf("%s", c.Tracking.Camera.StrobeDelay))
	args = append(args, "--at-family", *c.Tracking.Detection.Family)
	args = append(args, "--at-quad-decimate", fmt.Sprintf("%f", *c.Tracking.Detection.Quad.Decimate))
	args = append(args, "--at-quad-sigma", fmt.Sprintf("%f", *c.Tracking.Detection.Quad.Sigma))
	if *c.Tracking.Detection.Quad.RefineEdges == true {
		args = append(args, "--at-refine-edges")
	}
	args = append(args, "--at-quad-min-cluster", fmt.Sprintf("%d", *c.Tracking.Detection.Quad.MinClusterPixel))
	args = append(args, "--at-quad-max-n-maxima", fmt.Sprintf("%d", *c.Tracking.Detection.Quad.MaxNMaxima))
	args = append(args, "--at-quad-critical-radian", fmt.Sprintf("%f", *c.Tracking.Detection.Quad.CriticalRadian))
	args = append(args, "--at-quad-max-line-mse", fmt.Sprintf("%f", *c.Tracking.Detection.Quad.MaxLineMSE))
	args = append(args, "--at-quad-min-bw-diff", fmt.Sprintf("%d", *c.Tracking.Detection.Quad.MinBWDiff))
	if *c.Tracking.Detection.Quad.Deglitch == true {
		args = append(args, "--at-quad-deglitch")
	}

	if c.Node.IsMaster() == true {
		args = append(args, "--video-output-to-stdout")
		args = append(args, "--video-output-height", "1080")
		args = append(args, "--video-output-add-header")
		args = append(args, "--new-ant-output-dir", c.Path("ants"),
			"--new-ant-roi-size", fmt.Sprintf("%d", *c.Tracking.NewAntOutputROISize),
			"--image-renew-period", fmt.Sprintf("%s", c.Tracking.NewAntRenewPeriod))

	} else {
		args = append(args,
			"--camera-slave-width", fmt.Sprintf("%d", c.Tracking.Loads.Width),
			"--camera-slave-height", fmt.Sprintf("%d", c.Tracking.Loads.Height))
	}

	args = append(args, "--log-output-dir", c.ExperimentDir)

	if len(c.Balancing.IDsByUUID) > 1 {
		args = append(args, "--frame-stride", fmt.Sprintf("%d", len(c.Balancing.IDsByUUID)))
		ids := []string{}
		for i, isSet := range c.Balancing.IDsByUUID[c.Tracking.Loads.SelfUUID] {
			if isSet == false {
				continue
			}
			ids = append(ids, fmt.Sprintf("%d", i))
		}
		args = append(args, "--frame-ids", strings.Join(ids, ","))
	}

	tags := make([]string, 0, len(*c.Tracking.Highlights))
	for _, id := range *c.Tracking.Highlights {
		tags = append(tags, "0x"+strconv.FormatUint(uint64(id), 16))
	}

	if len(tags) != 0 {
		args = append(args, "--highlight-tags", strings.Join(tags, ","))
	}

	return args
}
