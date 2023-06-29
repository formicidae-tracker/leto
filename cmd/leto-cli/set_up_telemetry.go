package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/olympus/pkg/tm"
)

type SetUpTelemetryCommand struct {
	LogstashPort int    `long:"logstash-port" description:"port to connect to logstash" default:"3333"`
	OtelPort     int    `long:"otel-port" description:"port to connect to otel collector" default:"4317"`
	Verbose      []bool `short:"v" long:"verbose" description:"verbose level, can be set multiple time"`
	Args         struct {
		Hostname string `description:"hostname to send telemetry data to"`
	} `positional-args:"yes" required:"yes"`
}

func telemetryConfigPath() string {
	return filepath.Join(xdg.ConfigHome, "fort", "leto-cli", "telemetry.json")
}

func (c *SetUpTelemetryCommand) Execute([]string) error {
	args := tm.OtelProviderArgs{
		LogstashEndpoint: fmt.Sprintf("%s:%d", c.Args.Hostname, c.LogstashPort),
		CollectorURL:     fmt.Sprintf("%s:%d", c.Args.Hostname, c.OtelPort),
		ServiceName:      "leto-cli",
		ServiceVersion:   leto.LETO_VERSION,
		Level:            tm.VerboseLevel(len(c.Verbose)),
	}

	content, err := json.Marshal(args)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(telemetryConfigPath(), content, 0644)
}

func init() {
	parser.AddCommand("set-up-telemetry",
		"sets up telemetry configuration locally",
		"sets up telemetry configuration locally and save it to $XDG_CONFIG_HOME",
		&SetUpTelemetryCommand{})
}
