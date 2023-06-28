package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/formicidae-tracker/leto/internal/leto"
	"gopkg.in/yaml.v2"
)

var REFRESH_DURATION = 1 * time.Minute

type MonitorCommand struct {
	SlackURL string `long:"slack-url" description:"URL for the slack service" required:"true"`

	lister *leto.NodeLister

	currentStatuses map[string]*leto.TrackingConfiguration
}

var monitorCommand = &MonitorCommand{}

func (c *MonitorCommand) getStatuses() (map[string]*leto.TrackingConfiguration, error) {
	nodes, err := c.lister.ListNodes()
	if err != nil {
		return nil, err
	}
	wg := sync.WaitGroup{}
	type Result struct {
		Instance string
		Config   *leto.TrackingConfiguration
		Error    error
	}
	results := make(chan Result, 20)
	for _, nLocal := range nodes {
		n := nLocal
		wg.Add(1)
		go func() {
			defer wg.Done()

			status, err := n.GetStatus()
			if err != nil {
				results <- Result{Instance: n.Name, Config: nil, Error: err}
				return
			}

			if status.Experiment == nil {
				results <- Result{Instance: n.Name, Config: nil, Error: nil}
				return
			}

			config := leto.TrackingConfiguration{}
			err = yaml.Unmarshal([]byte(status.Experiment.YamlConfiguration), &config)

			if err != nil {
				results <- Result{Instance: n.Name, Config: nil, Error: err}
				return
			}

			results <- Result{Instance: n.Name, Config: &config, Error: nil}

		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	res := make(map[string]*leto.TrackingConfiguration)
	for r := range results {
		if r.Error != nil {
			log.Printf("Could not fetch status for '%s': %s", r.Instance, r.Error)
		} else {
			res[r.Instance] = r.Config
		}
	}

	return res, nil
}

func (c *MonitorCommand) buildEvents() ([]string, error) {
	newStatuses, err := c.getStatuses()
	if err != nil {
		return nil, err
	}
	defer func() {
		c.currentStatuses = newStatuses
	}()

	events := []string{}
	for nodeName, config := range c.currentStatuses {
		newConfig, ok := newStatuses[nodeName]
		if ok == false {
			if config != nil {
				events = append(events, fmt.Sprintf(":warning: Experiment `%s` on *%s* apparently ended unexpectedly", config.ExperimentName, nodeName))
			}
			continue
		}

		if newConfig == nil && config != nil {
			events = append(events, fmt.Sprintf(":information_source: Experiment `%s` on *%s* ended hopefully gracefully", config.ExperimentName, nodeName))
		}
	}

	return events, nil
}

func encodeMessage(message string) (*bytes.Buffer, error) {
	type SlackMessage struct {
		Text string `json:"text"`
	}

	buffer := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buffer)
	if err := enc.Encode(SlackMessage{Text: message}); err != nil {
		return nil, err
	}
	return buffer, nil
}

func (c *MonitorCommand) postToSlack(message string) error {
	buffer, err := encodeMessage(message)
	if err != nil {
		return err
	}
	resp, err := http.Post(c.SlackURL, "application/json", buffer)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Could not post to slack: got response %d: %s", resp.StatusCode, resp.Status)
	}
	return nil
}

func (c *MonitorCommand) Execute(args []string) error {
	c.lister = leto.NewNodeLister()
	c.currentStatuses = nil
	for {
		events, err := c.buildEvents()
		if err != nil {
			if err = c.postToSlack(fmt.Sprintf("[CRITICAL]: Could not build events: %s", err)); err != nil {
				log.Printf("Could not post to slack: %s", err)
			}
		}
		for _, e := range events {
			if err := c.postToSlack(e); err != nil {
				log.Printf("Could not post to slack: %s", err)
			}
		}

		time.Sleep(REFRESH_DURATION)
	}

}

func init() {
	parser.AddCommand("monitor", "monitors (scarcefully) local network for leto instances", "Monitor every minute the status of nodes on the network and report terminated experiment on slack", monitorCommand)

}
