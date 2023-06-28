package main

import "github.com/formicidae-tracker/leto/pkg/letopb"

type LinkingOptions struct {
	Args struct {
		Master Nodename
		Slave  Nodename
	} `positional-args:"yes" required:"yes"`

	command string
}

var linkCommand = &LinkingOptions{command: "Leto.Link"}
var unlinkCommand = &LinkingOptions{command: "Leto.Unlink"}

func (c *LinkingOptions) Execute(args []string) error {
	master, err := c.Args.Master.GetNode()
	if err != nil {
		return err
	}

	slave, err := c.Args.Slave.GetNode()
	if err != nil {
		return err
	}

	link := &letopb.TrackingLink{
		Master: master.Name,
		Slave:  slave.Name,
	}
	if c.command == "Leto.Link" {
		err = master.Link(link)
	} else if c.command == "Leto.Link" {
		err = master.Unlink(link)
	}
	return err
}

func init() {
	_, err := parser.AddCommand("link", "link two nodes together", "link a master to a slave", linkCommand)
	if err != nil {
		panic(err.Error())
	}
	_, err = parser.AddCommand("unlink", "unlink two linked nodes", "unlink a master and one of its slaves", unlinkCommand)
	if err != nil {
		panic(err.Error())
	}

}
