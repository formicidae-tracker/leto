//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

func main() {
	if err := execute(); err != nil {
		log.Fatalf("could not generate version: %s", err)
	}
}

func execute() error {
	version, err := fetch_version_from_git()
	if err != nil {
		return err
	}
	fmt.Printf("LETO_VERSION:%s\n", version)
	return write_version_file(version)
}

func fetch_version_from_git() (string, error) {
	cmd := exec.Command("git", "describe")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("`git describe` failed: %w, %s", err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

var version_go_template = template.Must(template.New("").Parse(`// Code generated by go generate DO NOT EDIT.

package leto

// Current package version
var LETO_VERSION = "{{.Version}}"
`))

func write_version_file(version string) error {
	f, err := os.Create("version.go")
	if err != nil {
		return err
	}
	defer f.Close()
	return version_go_template.Execute(f, struct {
		Timestamp time.Time
		Version   string
	}{Timestamp: time.Now(), Version: version})
}