package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/formicidae-tracker/hermes"
)

type ArtemisListener interface {
	Task
	Outbound() <-chan *hermes.FrameReadout
}

type artemisListener struct {
	outbound chan *hermes.FrameReadout
	server   *Server
}

// Returns an ArtemisListener that listen for any incoming
// hermes.FrameReadout stream on port, and provide an outbound
// channel. Cancelling the provided context will gracefully stop the
// Listener and incoming connections.
func NewArtemisListener(ctx context.Context, port int) (ArtemisListener, error) {
	server, err := NewServer(ctx, port, "artemis-in", 100*time.Millisecond)
	if err != nil {
		return nil, err
	}
	l := &artemisListener{
		outbound: make(chan *hermes.FrameReadout),
		server:   server,
	}
	l.server.onAccept = l.onAccept

	return l, nil
}

func (l *artemisListener) Outbound() <-chan *hermes.FrameReadout {
	return l.outbound
}

func (l *artemisListener) Run() error {
	defer close(l.outbound)
	return l.server.Run()
}

func (l *artemisListener) onAccept(ctx context.Context, conn net.Conn) {
	logger := NewLogger(fmt.Sprintf("artemis-in/%s", conn.RemoteAddr()))
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
