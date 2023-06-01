package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/formicidae-tracker/leto"
	"github.com/jessevdk/go-flags"
)

type Options struct {
	Version bool `long:"version"`
}

func main() {
	if err := execute(); err != nil {
		log.Fatalf("unhandled error: %s", err)
	}
}

func execute() error {
	opt := &Options{}
	parser := flags.NewParser(opt, flags.IgnoreUnknown)
	args, err := parser.Parse()
	if err != nil {
		return err
	}

	if opt.Version == true {
		printVersion()
		return nil
	}

	fmt.Printf("%v", args)

	return errors.New("not yet implemented")
}

func printVersion() {
	fmt.Printf("artemis %s\n", leto.ARTEMIS_MIN_VERSION)
}
