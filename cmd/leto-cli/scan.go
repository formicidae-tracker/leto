package main

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/atuleu/go-humanize"
	"github.com/atuleu/go-tablifier"
	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/leto/pkg/letopb"
	"golang.org/x/exp/constraints"
	"gopkg.in/yaml.v2"
)

type ScanCommand struct {
}

var scanCommand = &ScanCommand{}

type Result struct {
	Instance string
	Status   *letopb.Status
}

func (r Result) running() int {
	if r.Status.Experiment == nil {
		return 0
	}
	return 1
}

type ResultTableLine struct {
	Status     string `name:" "`
	Node       string
	Experiment string
	Since      string
	Space      string `name:"Space Used"`
	Remaining  string
	Links      string
}

func (c *ScanCommand) Execute(args []string) error {

	statuses := make(chan Result, 20)
	errors := make(chan error, 20)
	wg := sync.WaitGroup{}
	for _, nlocal := range nodes {
		n := nlocal
		wg.Add(1)
		go func() {
			defer wg.Done()

			status, err := n.GetStatus()
			if err != nil {
				errors <- err
				return
			}
			statuses <- Result{Instance: n.Name, Status: status}
		}()
	}
	go func() {
		wg.Wait()
		close(errors)
		close(statuses)
	}()

	for err := range errors {
		log.Printf("Could not fetch status: %s", err)
	}

	now := time.Now()

	c.printStatuses(now, statuses)

	return nil
}

func Max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}

	return b
}

var prefixes = []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei"}

func formatByteFraction(a, b int64) string {
	prefix := ""
	v := float64(Max(a, b))
	div := 1.0
	for _, prefix = range prefixes {
		if math.Abs(v/div) < 1024 {
			break
		}
		div *= 1024.0
	}
	return fmt.Sprintf("%.1f / %.1f %sB", float64(a)/div, float64(b)/div, prefix)
}

func (c *ScanCommand) printStatuses(now time.Time, statuses <-chan Result) {
	lines := make([]ResultTableLine, 0)

	for r := range statuses {
		line := ResultTableLine{
			Node:   strings.TrimPrefix(r.Instance, "leto."),
			Status: "\033[1;96m…\033[m",
		}
		if len(r.Status.Master) != 0 {
			line.Links = "↦ " + strings.TrimPrefix(r.Status.Master, "leto.")
		} else if len(r.Status.Slaves) != 0 {
			slaves := make([]string, len(r.Status.Slaves))
			for i, s := range r.Status.Slaves {
				slaves[i] = "↤  " + strings.TrimPrefix(s, "leto.")
			}
			line.Links = strings.Join(slaves, ",")
		}

		line.Space = formatByteFraction(
			Max(0, r.Status.TotalBytes-r.Status.FreeBytes),
			r.Status.TotalBytes)

		if r.Status.Experiment != nil {
			line.Status = "\033[1;92m✓\033[m"
			config := leto.TrackingConfiguration{}
			yaml.Unmarshal([]byte(r.Status.Experiment.YamlConfiguration), &config)
			line.Experiment = config.ExperimentName
			ellapsed := now.Sub(r.Status.Experiment.Since.AsTime()).Round(time.Minute)
			line.Since = fmt.Sprintf("%s", humanize.Duration(ellapsed))

			if r.Status.BytesPerSecond > 100 {
				rem := time.Duration(float64(r.Status.FreeBytes) /
					float64(r.Status.BytesPerSecond) *
					float64(time.Second))

				line.Remaining = fmt.Sprintf("%s",
					humanize.Duration(rem.Round(time.Minute)))
			} else {
				line.Remaining = fmt.Sprintf("∞")
			}

		}
		lines = append(lines, line)
	}

	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Status == lines[j].Status {
			return lines[i].Node < lines[j].Node
		}
		return strings.Contains(lines[i].Status, "✓")
	})

	tablifier.Tablify(lines)

}

func init() {
	parser.AddCommand("scan", "scans local network for leto instances", "Uses zeroconf to discover available leto instances and their status over the network", scanCommand)

}
