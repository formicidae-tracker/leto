package main

import (
	"fmt"

	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/leto/letopb"
	"gopkg.in/yaml.v2"
)

type LastExperimentLogCommand struct {
	Args struct {
		Node Nodename
	} `positional-args:"yes" required:"yes"`
	All           bool `short:"a" long:"all" description:"print all information (former default)"`
	Log           bool `short:"l" long:"log" description:"print artemis logs"`
	Stderr        bool `short:"e" long:"stderr" description:"print artemis stderr (for checking segfaults)"`
	Configuration bool `short:"c" long:"configuration" description:"print the experiment configuration"`
}

var lastExperimentCommand = &LastExperimentLogCommand{}

func (c *LastExperimentLogCommand) printSummary(name string, log *letopb.ExperimentLog) {
	status := "\033[36m✓\033[m"
	if log.HasError == true {
		status = "\031[36m⚠\033[m"
	}

	fmt.Printf("Experiment Name       : %s\n", name)
	fmt.Printf("Experiment Output Dir : %s\n", log.ExperimentDir)
	fmt.Printf("Experiment Start Date : %s\n", log.Start)
	fmt.Printf("Experiment End Date   : %s\n", log.End)
	fmt.Printf("Experiment Status     : %s\n", status)
	if log.HasError == true {
		fmt.Printf("Experiment Error      : %s\n", log.Error)
	}
}

func (c *LastExperimentLogCommand) None() bool {
	return (c.All || c.Log || c.Configuration || c.Stderr) == false
}

func (c *LastExperimentLogCommand) MultipleSections() bool {
	if c.All == true {
		return true
	}
	sections := []bool{c.Configuration, c.Log, c.Stderr}
	count := 0
	for _, s := range sections {
		if s == true {
			count += 1
		}
	}
	return count > 1
}

func (c *LastExperimentLogCommand) printHeader(section string) {
	if c.MultipleSections() == false {
		return
	}
	fmt.Printf("\n=== %s ===\n\n", section)
}

func (c *LastExperimentLogCommand) printFooter(section string) {
	if c.MultipleSections() == false {
		return
	}
	fmt.Printf("\n=== End of %s ===\n\n", section)
}

func (c *LastExperimentLogCommand) printConfiguration(log *letopb.ExperimentLog) {
	c.printHeader("Experiment YAML Configuration")
	fmt.Println(log.YamlConfiguration)
	c.printFooter("Experiment YAML Configuration")
}

func (c *LastExperimentLogCommand) printLog(log *letopb.ExperimentLog) {
	c.printHeader("Artemis INFO Log")
	fmt.Println(log.YamlConfiguration)
	c.printFooter("Artemis INFO Log")
}

func (c *LastExperimentLogCommand) printStderr(log *letopb.ExperimentLog) {
	c.printHeader("Artemis STDERR")
	fmt.Println(log.YamlConfiguration)
	c.printFooter("Artemis STDERR")
}

func (c *LastExperimentLogCommand) Execute(args []string) error {
	n, err := c.Args.Node.GetNode()
	if err != nil {
		return err
	}

	log, err := n.GetLastExperimentLog()
	if err != nil {
		return err
	}

	config := leto.TrackingConfiguration{}
	err = yaml.Unmarshal([]byte(log.YamlConfiguration), &config)
	if err != nil {
		return fmt.Errorf("Could not parse YAML configuration: %s", err)
	}

	if c.None() {
		c.printSummary(config.ExperimentName, log)
	}

	if c.All || c.Configuration {
		c.printConfiguration(log)
	}

	if c.All || c.Log {
		c.printLog(log)
	}

	if c.All || c.Stderr {
		c.printStderr(log)
	}

	return nil
}

func init() {
	_, err := parser.AddCommand("last-experiment-log", "queries the last experiment log on the node", "Queries the last experiment log on the node", lastExperimentCommand)
	if err != nil {
		panic(err.Error())
	}
}
