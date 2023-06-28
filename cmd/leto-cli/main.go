package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/jessevdk/go-flags"
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

func Execute() error {
	var err error
	nodes, err = leto.NewNodeLister().ListNodes()
	if err != nil {
		return fmt.Errorf("Could not list nodes on local network: %s", err)
	}

	_, err = parser.Parse()
	if ferr, ok := err.(*flags.Error); ok == true && ferr.Type == flags.ErrHelp {
		err = nil
	}
	return err
}

func main() {
	if err := Execute(); err != nil {
		log.Fatalf("Unhandled error")
	}
}
