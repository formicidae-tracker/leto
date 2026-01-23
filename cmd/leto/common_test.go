package main

import (
	"flag"
	"io"
	"log/slog"
	"testing"

	. "gopkg.in/check.v1"
)

var logstostderr = flag.Bool("logstostderr", false, "leaves module log to stderr, otherwise it will be discarded")

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) {
	if *logstostderr == false {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	TestingT(t)
}
