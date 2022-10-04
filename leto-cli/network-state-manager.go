package main

import (
	"io/ioutil"

	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/leto/letopb"
	"github.com/jessevdk/go-flags"
	"gopkg.in/yaml.v2"
)

type RestoreNetworkStateCommand struct {
	Args struct {
		StateFile flags.Filename
	} `positional-args:"yes" required:"yes"`
}

type SaveNetworkStateCommand struct {
	StopNodes bool `long:"stop-nodes" description:"stops running instances"`
	Args      struct {
		StateFile flags.Filename
	} `positional-args:"yes" required:"yes"`
}

type networkState struct {
	Nodes map[string]leto.TrackingConfiguration
}

func fetchNodeConfig(n leto.Node) (*leto.TrackingConfiguration, error) {
	status, err := n.GetStatus()
	if err != nil {
		return nil, err
	}

	if status.Experiment == nil {
		return nil, nil
	}
	config := leto.TrackingConfiguration{}

	err = yaml.Unmarshal([]byte(status.Experiment.YamlConfiguration), &config)
	if err != nil {
		return nil, err
	}
	// strips load balancing configuration
	config.Loads = nil

	return &config, nil
}

func fetchNetworkState() (*networkState, error) {
	l := leto.NewNodeLister()
	nodes, err := l.ListNodes()
	if err != nil {
		return nil, err
	}
	res := &networkState{
		Nodes: make(map[string]leto.TrackingConfiguration),
	}

	for _, n := range nodes {
		config, err := fetchNodeConfig(n)
		if err != nil {
			return nil, err
		}
		if config == nil {
			continue
		}
		res.Nodes[n.Name] = *config
	}
	return res, nil
}

func (s *networkState) save(filename string) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, data, 0644)
}

func loadNetworkState(filename string) (*networkState, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	res := &networkState{}
	return res, yaml.Unmarshal(data, res)
}

func stopTracking(name Nodename) error {
	node, err := name.GetNode()
	if err != nil {
		return err
	}
	return node.StopTracking()
}

func startTracking(name Nodename, config leto.TrackingConfiguration) error {
	node, err := name.GetNode()
	if err != nil {
		return err
	}
	asYaml, err := config.Yaml()
	if err != nil {
		return err
	}
	request := &letopb.StartRequest{YamlConfiguration: string(asYaml)}

	return node.StartTracking(request)
}

func (c *SaveNetworkStateCommand) Execute(args []string) error {
	state, err := fetchNetworkState()
	if err != nil {
		return err
	}
	err = state.save(string(c.Args.StateFile))
	if err != nil {
		return err
	}

	if c.StopNodes == true {
		for n, _ := range state.Nodes {
			if err := stopTracking(Nodename(n)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *RestoreNetworkStateCommand) Execute(args []string) error {
	state, err := loadNetworkState(string(c.Args.StateFile))
	if err != nil {
		return err
	}
	for n, s := range state.Nodes {
		if err := startTracking(Nodename(n), s); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	if leto.LETO_VERSION != "development" {
		return
	}

	parser.AddCommand("save-state",
		"save tracking states on all nodes",
		"save all tracking states of all nodes on local network",
		&SaveNetworkStateCommand{})

	parser.AddCommand("restore-state",
		"restore tracking states on all nodes",
		"restore all tracking states of all nodes on local network",
		&RestoreNetworkStateCommand{})

}
