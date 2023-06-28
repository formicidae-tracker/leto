package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/formicidae-tracker/leto/internal/leto"
)

type DisplayFrameReadoutCommand struct {
	Args struct {
		Node Nodename
	} `positional-args:"yes" required:"yes"`
}

var displayFrameReadoutCommand = &DisplayFrameReadoutCommand{}

type FrameReadoutDisplayer struct {
	Errors  int
	Frames  int
	TagsIDs map[uint32]bool
}

func (d *FrameReadoutDisplayer) DisplayFrameReadout(h *hermes.FrameReadout) string {
	d.Frames += 1
	currentTags := len(h.Tags)
	if h.Error != hermes.FrameReadout_NO_ERROR {
		d.Errors += 1
	} else {
		for _, a := range h.Tags {
			d.TagsIDs[a.ID] = true
		}
	}

	return fmt.Sprintf("%s %06d %06d %04d/%04d    %06d", time.Now().Format("15:04:05"), d.Frames, d.Errors, currentTags, len(d.TagsIDs), h.Quads)
}

func (c *DisplayFrameReadoutCommand) Execute(args []string) error {
	n, err := c.Args.Node.GetNode()
	if err != nil {
		return err
	}

	status, err := n.GetStatus()
	if err != nil {
		return fmt.Errorf("Could not query '%s' status: %s", n.Name, err)
	}
	if status.Experiment == nil {
		return fmt.Errorf("'%s' is not running", n.Name)
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", n.Address, leto.DefaultConfig.HermesBroadcastPort))
	if err != nil {
		return fmt.Errorf("Could not connect to '%s': %s", n.Name, err)
	}

	version := &hermes.Header{}
	ok, err := hermes.ReadDelimitedMessage(conn, version)
	if err != nil {
		conn.Close()
		return err
	}
	if ok == false {
		conn.Close()
		return fmt.Errorf("Did not receive an expected version header")
	}

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		conn.Close()
	}()

	d := FrameReadoutDisplayer{
		TagsIDs: make(map[uint32]bool),
	}

	fmt.Fprintf(os.Stdout, "Time    Frames  Errors Cur/Tot Tags Quads \n\n")
	for {
		m := &hermes.FrameReadout{}
		ok, err := hermes.ReadDelimitedMessage(conn, m)
		if err != nil {
			return err
		}
		if ok == true {
			fmt.Fprintf(os.Stdout, "\033[A%s\n", d.DisplayFrameReadout(m))
		}
	}

}

func init() {
	parser.AddCommand("display-frame-readout", "display current frame readout", "Displays frame readout information on the standard output", displayFrameReadoutCommand)

}
