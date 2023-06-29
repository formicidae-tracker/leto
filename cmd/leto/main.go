package main

import (
	"fmt"
	"log"

	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/jessevdk/go-flags"
)

func main() {
	if err := execute(); err != nil {
		log.Fatalf("Unhandled error: %s", err)
	}
}

type Options struct {
	OtelEndpoint     string `long:"otel-endpoint" description:"Open telemetry endoint to use" env:"LETO_OTEL_ENDPOINT"`
	LogstashEndpoint string `long:"logstash-endpoint" description:"Logstash endoint to use" env:"LETO_LOGSTASH_ENDPOINT"`
	Version          bool   `short:"V" long:"version" description:"Print version and exists"`
	Verbose          []bool `short:"v" long:"verbose" description:"Enable more verbose output (can be set multiple times)"`
	RPCPort          *int   `long:"rpc-port" description:"Port to use for RPC incoming call"`
	Devmode          bool   `long:"dev" description:"development mode to bypass some checks"`
}

func (o *Options) LetoConfig() leto.Config {
	res := leto.DefaultConfig
	if o.RPCPort != nil {
		res.LetoPort = *o.RPCPort
	}
	res.DevMode = o.Devmode
	return res
}

func setUpLogger(opts *Options) {
	if len(opts.OtelEndpoint) > 0 || len(opts.LogstashEndpoint) > 0 {
		tm.SetUpTelemetry(tm.OtelProviderArgs{
			LogstashEndpoint: opts.LogstashEndpoint,
			CollectorURL:     opts.OtelEndpoint,
			ServiceName:      "leto",
			ServiceVersion:   leto.LETO_VERSION,
			Level:            tm.VerboseLevel(len(opts.Verbose)),
		})
	} else {
		tm.SetUpLocal(tm.VerboseLevel(len(opts.Verbose)))
	}
}

func execute() error {
	opts := &Options{}
	_, err := flags.Parse(opts)
	if err != nil {
		return err
	}

	if opts.Version {
		fmt.Printf("leto %s\n", leto.LETO_VERSION)
		return nil
	}

	setUpLogger(opts)

	return (&LetoGRPCWrapper{}).Run(opts.LetoConfig())
}
