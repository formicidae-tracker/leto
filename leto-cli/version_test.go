package main

import "github.com/formicidae-tracker/leto"

func ExampleVersionCommand() {
	leto.LETO_VERSION = "development-tests"
	(&VersionCommand{}).Execute(nil)
	// output: leto-cli version development-tests
}
