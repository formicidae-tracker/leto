package main

import (
	"fmt"
	"log"

	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/jessevdk/go-flags"
)

func main() {
	if err := execute(); err != nil {
		log.Fatalf("Unhandled error: %s", err)
	}
}

type Options struct {
	Version bool `short:"V" long:"version" description:"Print version and exists"`
	RPCPort *int `long:"rpc-port" description:"Port to use for RPC incoming call"`
	Devmode bool `long:"dev" description:"development mode to bypass some checks"`
}

func (o *Options) LetoConfig() leto.Config {
	res := leto.DefaultConfig
	if o.RPCPort != nil {
		res.LetoPort = *o.RPCPort
	}
	res.DevMode = o.Devmode
	return res
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

	return (&LetoGRPCWrapper{}).Run(opts.LetoConfig())
}
