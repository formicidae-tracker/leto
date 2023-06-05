package main

import (
	"math"
	"time"

	"github.com/formicidae-tracker/leto/letopb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ExampleScanCommand() {
	statuses := make(chan Result)
	go func() {
		statuses <- Result{
			Instance: "leto.delphi",
			Status: &letopb.Status{
				TotalBytes:     2 * 1024 * 1024 * 1024 * 1024,
				FreeBytes:      int64(1.2987 * math.Pow(2, 40)),
				BytesPerSecond: 0,
			},
		}
		statuses <- Result{
			Instance: "corinth",
			Status: &letopb.Status{
				TotalBytes:     2 * 1024 * 1024 * 1024 * 1024,
				FreeBytes:      int64(1.99987 * math.Pow(2, 40)),
				BytesPerSecond: 0,
			},
		}
		statuses <- Result{
			Instance: "olympia",
			Status: &letopb.Status{
				TotalBytes:     2 * 1024 * 1024 * 1024 * 1024,
				FreeBytes:      int64(1.96987 * math.Pow(2, 40)),
				BytesPerSecond: 0,
			},
		}

		statuses <- Result{
			Instance: "sparta",
			Status: &letopb.Status{
				Experiment: &letopb.ExperimentStatus{
					Since:             timestamppb.New(time.Date(2023, 03, 31, 10, 18, 56, 0, time.UTC)),
					ExperimentDir:     "someexp.0001",
					YamlConfiguration: "experiment: someexp",
				},
				TotalBytes:     2 * 1024 * 1024 * 1024 * 1024,
				FreeBytes:      1581 * 1024 * 1024 * 1024,
				BytesPerSecond: 245 * 1024,
			},
		}
		statuses <- Result{
			Instance: "athens",
			Status: &letopb.Status{
				Slaves: []string{"leto.piraeus"},
				Experiment: &letopb.ExperimentStatus{
					Since:             timestamppb.New(time.Date(2023, 03, 10, 14, 28, 56, 0, time.UTC)),
					ExperimentDir:     "anotherexp.0002",
					YamlConfiguration: "experiment: anotherexp",
				},
				TotalBytes:     2 * 1024 * 1024 * 1024 * 1024,
				FreeBytes:      881 * 1024 * 1024,
				BytesPerSecond: 341 * 1024,
			},
		}
		statuses <- Result{
			Instance: "leto.piraeus",
			Status: &letopb.Status{
				Master: "leto.athens",
				Experiment: &letopb.ExperimentStatus{
					Since:             timestamppb.New(time.Date(2023, 03, 10, 14, 28, 56, 0, time.UTC)),
					ExperimentDir:     "anotherexp.0002",
					YamlConfiguration: "experiment: anotherexp",
				},
				TotalBytes:     2 * 1024 * 1024 * 1024 * 1024,
				FreeBytes:      2047*1024*1024*1024 + 523*1024*1024,
				BytesPerSecond: 10,
			},
		}

		close(statuses)
	}()

	(&ScanCommand{}).printStatuses(time.Date(2023, 04, 01, 11, 35, 03, 00, time.UTC), statuses)
	//output:
	//┌─────┬─────────┬────────────┬──────────────┬───────────────┬──────────────────┬──────────────┐
	//│     │ Node    │ Experiment │ Since        │ Space Used    │ Remaining        │ Links        │
	//├─────┼─────────┼────────────┼──────────────┼───────────────┼──────────────────┼──────────────┤
	//│   ✓ │ athens  │ anotherexp │ 3 weeks      │ 2.0 / 2.0 TiB │ 44m0s            │ ↤  piraeus   │
	//│   ✓ │ piraeus │ anotherexp │ 3 weeks      │ 0.0 / 2.0 TiB │ ∞                │ ↦ athens     │
	//│   ✓ │ sparta  │ someexp    │ 1 day 1 hour │ 0.5 / 2.0 TiB │ 2 months 2 weeks │              │
	//│   … │ corinth │            │              │ 0.0 / 2.0 TiB │                  │              │
	//│   … │ delphi  │            │              │ 0.7 / 2.0 TiB │                  │              │
	//│   … │ olympia │            │              │ 0.0 / 2.0 TiB │                  │              │
	//└─────┴─────────┴────────────┴──────────────┴───────────────┴──────────────────┴──────────────┘
}
