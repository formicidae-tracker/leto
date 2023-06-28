package main

import (
	"fmt"

	"github.com/atuleu/go-humanize"
	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/leto/pkg/letopb"
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
		status = "\033[31m⚠\033[m"
	}

	start := log.Start.AsTime()
	end := log.End.AsTime()
	ellapsed := end.Sub(start)

	timeFmt := "Monday _2 Jan 15:04:05 2006"

	fmt.Printf("Name       : %s\n", name)
	fmt.Printf("Output Dir : %s\n", log.ExperimentDir)
	fmt.Printf("Start Date : %s\n", start.Local().Format(timeFmt))
	fmt.Printf("End Date   : %s\n", end.Local().Format(timeFmt))
	fmt.Printf("Duration   : %s\n", humanize.Duration(ellapsed))
	fmt.Printf("Status     : %s\n", status)
	if log.HasError == true {
		fmt.Printf("Error      : %s\n", log.Error)
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
	fmt.Printf("\n=== End of %s ===\n", section)
}

func (c *LastExperimentLogCommand) printConfiguration(log *letopb.ExperimentLog) {
	c.printHeader("Experiment YAML Configuration")
	fmt.Printf(log.YamlConfiguration)
	c.printFooter("Experiment YAML Configuration")
}

func (c *LastExperimentLogCommand) printArtemisLog(log *letopb.ExperimentLog) {
	c.printHeader("Artemis INFO Log")
	fmt.Println(log.Log)
	c.printFooter("Artemis INFO Log")
}

func (c *LastExperimentLogCommand) printStderr(log *letopb.ExperimentLog) {
	c.printHeader("Artemis STDERR")
	fmt.Println(log.Stderr)
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

	c.printLog(log, config)

	return nil
}

func (c *LastExperimentLogCommand) printLog(log *letopb.ExperimentLog,
	config leto.TrackingConfiguration) {

	if c.All || c.None() {
		c.printSummary(config.ExperimentName, log)
	}

	if c.All || c.Configuration {
		c.printConfiguration(log)
	}

	if c.All || c.Log {
		c.printArtemisLog(log)
	}

	if c.All || c.Stderr {
		c.printStderr(log)
	}
}

func init() {
	_, err := parser.AddCommand("last-experiment-log", "queries the last experiment log on the node", "Queries the last experiment log on the node", lastExperimentCommand)
	if err != nil {
		panic(err.Error())
	}
}
