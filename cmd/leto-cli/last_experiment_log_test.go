package main

import (
	"time"

	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/leto/pkg/letopb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var testconfig = leto.TrackingConfiguration{
	ExperimentName: "someexp",
}

var testlog = &letopb.ExperimentLog{
	ExperimentDir: "someexp.0002",
	Log:           "artemis log",
	Stderr:        "artemis stderr",
	Start:         timestamppb.New(time.Date(2023, 4, 1, 10, 58, 21, 0, time.UTC)),
	End:           timestamppb.New(time.Date(2023, 4, 24, 18, 12, 01, 0, time.UTC)),
	HasError:      true,
	Error:         "Something critical happened",
}

func ExampleLastExperimentLogCommand_summary() {

	(&LastExperimentLogCommand{}).printLog(testlog, testconfig)
	//Output: Name       : someexp
	//Output Dir : someexp.0002
	//Start Date : Saturday  1 Apr 12:58:21 2023
	//End Date   : Monday 24 Apr 20:12:01 2023
	//Duration   : 3 weeks 2 days
	//Status     : [31m⚠[m
	//Error      : Something critical happened
}

func ExampleLastExperimentLogCommand_log() {
	(&LastExperimentLogCommand{Log: true}).printLog(testlog, testconfig)
	//Output: artemis log
}

func ExampleLastExperimentLogCommand_stderr() {
	(&LastExperimentLogCommand{Stderr: true}).printLog(testlog, testconfig)
	//Output: artemis stderr
}

func ExampleLastExperimentLogCommand_config() {
	(&LastExperimentLogCommand{Configuration: true}).printLog(testlog, testconfig)
	//Output: experiment: someexp
	// legacy-mode: false
	// new-ant-roi: 600
	// image-renew-period: 2h0m0s
	// stream:
	//   host: ""
	//   bitrate: 2000
	//   bitrate-max-ratio: 1.5
	//   quality: fast
	//   tuning: film
	// camera:
	//   strobe-delay: 0s
	//   strobe-duration: 1.5ms
	//   fps: 8
	//   stub-image-paths: []
	// apriltag:
	//   family: ""
	//   quad:
	//     decimate: 1
	//     sigma: 0
	//     refine-edges: false
	//     min-cluster-pixel: 25
	//     max-n-maxima: 10
	//     critical-angle-radian: 0.17453292519943295
	//     max-line-mean-square-error: 10
	//     min-black-white-diff: 50
	//     deglitch: false
	// highlights: []
	// load-balancing: null
	// threads: 0
}

func ExampleLastExperimentLogCommand_all() {
	(&LastExperimentLogCommand{All: true}).printLog(testlog, testconfig)
	//Output: Name       : someexp
	//Output Dir : someexp.0002
	//Start Date : Saturday  1 Apr 12:58:21 2023
	//End Date   : Monday 24 Apr 20:12:01 2023
	//Duration   : 3 weeks 2 days
	//Status     : [31m⚠[m
	//Error      : Something critical happened
	//
	// === Experiment YAML Configuration ===
	//
	//experiment: someexp
	// legacy-mode: false
	// new-ant-roi: 600
	// image-renew-period: 2h0m0s
	// stream:
	//   host: ""
	//   bitrate: 2000
	//   bitrate-max-ratio: 1.5
	//   quality: fast
	//   tuning: film
	// camera:
	//   strobe-delay: 0s
	//   strobe-duration: 1.5ms
	//   fps: 8
	//   stub-image-paths: []
	// apriltag:
	//   family: ""
	//   quad:
	//     decimate: 1
	//     sigma: 0
	//     refine-edges: false
	//     min-cluster-pixel: 25
	//     max-n-maxima: 10
	//     critical-angle-radian: 0.17453292519943295
	//     max-line-mean-square-error: 10
	//     min-black-white-diff: 50
	//     deglitch: false
	// highlights: []
	// load-balancing: null
	// threads: 0
	//
	// === End of Experiment YAML Configuration ===
	//
	// === Artemis INFO Log ===
	//
	// artemis log
	//
	// === End of Artemis INFO Log ===
	//
	// === Artemis STDERR ===
	//
	// artemis stderr
	//
	// === End of Artemis STDERR ===
}

func init() {
	merged := leto.LoadDefaultConfig()
	merged.Merge(&testconfig)
	testconfig = *merged

	config, _ := testconfig.Yaml()
	testlog.YamlConfiguration = string(config)
}
