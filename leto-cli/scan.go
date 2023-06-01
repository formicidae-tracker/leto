package main

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/atuleu/go-tablifier"
	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/leto/letopb"
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
	Status     string
	Node       string
	Experiment string
	Since      string
	Space      string
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

	lines := make([]ResultTableLine, 0, len(nodes))

	now := time.Now()

	for r := range statuses {
		line := ResultTableLine{
			Node:       strings.TrimPrefix(r.Instance, "leto."),
			Status:     "Idle",
			Experiment: "N.A.",
			Since:      "N.A.",
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
		if r.Status.Experiment != nil {
			line.Status = "Running"
			config := leto.TrackingConfiguration{}
			yaml.Unmarshal([]byte(r.Status.Experiment.YamlConfiguration), &config)
			line.Experiment = config.ExperimentName
			ellapsed := now.Sub(r.Status.Experiment.Since.AsTime()).Round(time.Second)
			line.Since = fmt.Sprintf("%s", ellapsed)
		}
		lines = append(lines, line)
	}

	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Status == lines[j].Status {
			return lines[i].Node < lines[j].Node
		}
		return lines[i].Status == "Running"
	})

	tablifier.Tablify(lines)

	return nil
}

func init() {
	parser.AddCommand("scan", "scans local network for leto instances", "Uses zeroconf to discover available leto instances and their status over the network", scanCommand)

}
