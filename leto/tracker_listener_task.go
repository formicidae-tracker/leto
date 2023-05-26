package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/formicidae-tracker/hermes"
)

type TrackerListenerTask interface {
	Task
	Outbound() <-chan *hermes.FrameReadout
}

type trackerListenerTask struct {
	outbound chan *hermes.FrameReadout
	server   *Server
}

// Returns a TrackerListenerTask that listen for any incoming
// hermes.FrameReadout stream on port, and provide an outbound
// channel. Cancelling the provided context will gracefully stop the
// tracker.
func NewTrackerListenerTask(ctx context.Context, port int) (TrackerListenerTask, error) {
	server, err := NewServer(ctx, port, "[artemis-in]: ", 100*time.Millisecond)
	if err != nil {
		return nil, err
	}
	l := &trackerListenerTask{
		outbound: make(chan *hermes.FrameReadout),
		server:   server,
	}
	l.server.onAccept = l.onAccept
	l.server.onClose = l.onClose

	return l, nil
}

func (l *trackerListenerTask) Outbound() <-chan *hermes.FrameReadout {
	return l.outbound
}

func (l *trackerListenerTask) Start() {
	l.server.Start()
}

func (l *trackerListenerTask) Done() <-chan error {
	return l.server.Done()
}

func (l *trackerListenerTask) onClose() {
	close(l.outbound)
}

func (l *trackerListenerTask) onAccept(ctx context.Context, conn net.Conn) {
	logger := log.New(os.Stderr,
		fmt.Sprintf("[artemis/%s]: ", conn.RemoteAddr().String()),
		log.LstdFlags)
	logger.Printf("start reading incoming frames")
	errors := make(chan error)
	go func() {
		for err := range errors {
			logger.Printf("frame reading error: %s", err)
		}
	}()
	FrameReadoutReadAll(ctx, conn, l.outbound, errors)
	logger.Printf("stop reading incoming frames")
}
