package main

import "github.com/formicidae-tracker/leto/internal/leto"

func ExampleVersionCommand() {
	leto.LETO_VERSION = "development-tests"
	(&VersionCommand{}).Execute(nil)
	// output: leto-cli version development-tests
}
