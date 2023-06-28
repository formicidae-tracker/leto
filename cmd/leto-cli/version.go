package main

import (
	"fmt"
	"os"

	"github.com/formicidae-tracker/leto/internal/leto"
)

type VersionCommand struct {
}

var versionCommand = &VersionCommand{}

func (c *VersionCommand) Execute(args []string) error {
	fmt.Fprintf(os.Stdout, "leto-cli version %s\n", leto.LETO_VERSION)
	return nil
}

func init() {
	_, err := parser.AddCommand("version", "prints current version", "Prints on stdout the current version", versionCommand)
	if err != nil {
		panic(err.Error())
	}
}
