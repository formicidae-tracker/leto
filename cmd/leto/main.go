package main

import (
	"context"
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
	OtelEndpoint string `long:"otel-endpoint" description:"Open telemetry endoint to use" env:"LETO_OTEL_ENDPOINT"`
	Version      bool   `short:"V" long:"version" description:"Print version and exists"`
	Verbose      []bool `short:"v" long:"verbose" description:"Enable more verbose output (can be set multiple times)"`
	RPCPort      *int   `long:"rpc-port" description:"Port to use for RPC incoming call"`
	Devmode      bool   `long:"dev" description:"development mode to bypass some checks"`
	DiskLimit    int64  `long:"disk-limit" description:"minimum space to leave on disk"`
}

func (o *Options) LetoConfig() leto.Config {
	res := leto.DefaultConfig
	if o.RPCPort != nil {
		res.LetoPort = *o.RPCPort
	}
	if o.DiskLimit > 0 {
		res.DiskLimit = o.DiskLimit
	}

	res.DevMode = o.Devmode
	return res
}

func setUpLogger(opts *Options) {
	if len(opts.OtelEndpoint) > 0 {
		tm.SetUpTelemetry(tm.OtelProviderArgs{
			CollectorURL:   opts.OtelEndpoint,
			ServiceName:    "leto",
			ServiceVersion: leto.LETO_VERSION,
			Level:          tm.VerboseLevel(len(opts.Verbose)),
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
	defer tm.Shutdown(context.Background())

	return (&LetoGRPCWrapper{}).Run(opts.LetoConfig())
}
