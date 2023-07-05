package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/jessevdk/go-flags"
	"github.com/sirupsen/logrus"
)

type Options struct {
}

type Nodename string

var nodes map[string]leto.Node

func (n *Nodename) GetNode() (*leto.Node, error) {
	if len(*n) == 0 {
		return nil, fmt.Errorf("Missing mandatory node name")
	}
	node, ok := nodes[string(*n)]
	if ok == false {
		return nil, fmt.Errorf("Could not find node '%s'", *n)
	}
	return &node, nil
}

func (n *Nodename) Complete(match string) []flags.Completion {
	res := make([]flags.Completion, 0, len(nodes))
	for nodeName, node := range nodes {
		if strings.HasPrefix(nodeName, match) == false {
			continue
		}
		res = append(res, flags.Completion{
			Item:        nodeName,
			Description: fmt.Sprintf("%s:%d", node.Address, node.Port),
		})
	}
	return res
}

var opts = &Options{}

var parser = flags.NewParser(opts, flags.Default)

func setUpLogger() {
	var err error
	defer func() {
		if err != nil {
			logrus.WithError(err).Error("could not load telemetry config file")
		}
	}()

	content, err := ioutil.ReadFile(telemetryConfigPath())
	if err != nil {
		if os.IsNotExist(err) == true {
			err = nil
		} else {
			err = fmt.Errorf("could not read telemetry config file: %w", err)
		}
		return
	}

	args := tm.OtelProviderArgs{}
	err = json.Unmarshal(content, &args)
	if err != nil {
		return
	}
	args.ForceFlushOnShutdown = true

	tm.SetUpTelemetry(args)
}

func Execute() error {
	setUpLogger()
	defer tm.Shutdown(context.Background())

	var err error
	nodes, err = leto.NewNodeLister().ListNodes()
	if err != nil {
		return fmt.Errorf("Could not list nodes on local network: %s", err)
	}

	_, err = parser.Parse()
	if err != nil &&
		flags.WroteHelp(err) == true {
		return nil
	}

	return err
}

func main() {
	if err := Execute(); err != nil {
		os.Exit(2)
	}
}
