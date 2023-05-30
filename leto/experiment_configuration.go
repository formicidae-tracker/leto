package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/formicidae-tracker/leto"
)

type ExperimentConfiguration struct {
	Node                NodeConfiguration
	Tracking            *leto.TrackingConfiguration
	Balancing           *WorkloadBalance
	TestMode            bool
	ExperimentDir       string
	ArtemisIncomingPort int
}

func (c *ExperimentConfiguration) antOutputDir() string {
	return filepath.Join(c.ExperimentDir, "ants")
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
		args = append(args, "--new-ant-output-dir", c.antOutputDir(),
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
