package main

import (
	"fmt"

	"github.com/formicidae-tracker/hermes"
)

type FrameDispatcher interface {
	Task
	Incoming() chan<- *hermes.FrameReadout
}

type frameDispatcher struct {
	incoming chan *hermes.FrameReadout
	outgoing []chan<- *hermes.FrameReadout
}

func NewFrameDispatcher(outgoing ...chan<- *hermes.FrameReadout) FrameDispatcher {
	return &frameDispatcher{
		incoming: make(chan *hermes.FrameReadout, 10),
		outgoing: outgoing,
	}
}

func (d *frameDispatcher) Run() (err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("dispatch did panic: %s", r)
		}
	}()
	defer d.closeOutgoing()
	for r := range d.incoming {
		for _, o := range d.outgoing {
			select {
			case o <- r:
			default:
			}
		}
	}
	return nil
}

func (d *frameDispatcher) Incoming() chan<- *hermes.FrameReadout {
	return d.incoming
}

func (d *frameDispatcher) closeOutgoing() {
	for _, o := range d.outgoing {
		close(o)
	}
}
