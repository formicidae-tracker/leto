package main

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/golang/protobuf/proto"
)

type HermesBroadcaster interface {
	Task
	Incoming() chan<- *hermes.FrameReadout
}

type hermesBroadcaster struct {
	mx sync.RWMutex

	server   *Server
	incoming chan *hermes.FrameReadout
	outgoing map[int]chan []byte
	idle     time.Duration
	nextId   int
}

func (b *hermesBroadcaster) Incoming() chan<- *hermes.FrameReadout {
	return b.incoming
}

func (b *hermesBroadcaster) Run() error {
	go b.incomingLoop()
	return b.server.Run()
}

func (b *hermesBroadcaster) incomingLoop() {
	defer b.closeAllOutgoing()
	for r := range b.incoming {
		buf := proto.NewBuffer(nil)
		buf.EncodeMessage(r)
		b.broadcastToAll(buf.Bytes())
	}
}

func (b *hermesBroadcaster) broadcastToAll(data []byte) {
	b.mx.RLock()
	defer b.mx.RUnlock()

	for _, ch := range b.outgoing {
		select {
		case ch <- data:
		default:
			continue
		}
	}
}

func (b *hermesBroadcaster) closeAllOutgoing() {
	b.mx.Lock()
	defer b.mx.Unlock()

	for _, ch := range b.outgoing {
		close(ch)
	}
	b.outgoing = nil
}

func NewHermesBroadcaster(ctx context.Context, port int, idle time.Duration) (HermesBroadcaster, error) {
	server, err := NewServer(ctx, port, "broadcast", 1*time.Second)
	if err != nil {
		return nil, err
	}
	res := &hermesBroadcaster{
		server:   server,
		incoming: make(chan *hermes.FrameReadout, 10),
		outgoing: make(map[int]chan []byte),
		idle:     idle,
	}
	res.server.onAccept = res.onAccept
	return res, nil
}

func (h *hermesBroadcaster) registerNew() (int, <-chan []byte) {
	h.mx.Lock()
	defer h.mx.Unlock()
	id := h.nextId
	h.nextId += 1

	h.outgoing[id] = make(chan []byte, 10)
	return id, h.outgoing[id]
}

func (h *hermesBroadcaster) unregister(id int) {
	h.mx.Lock()
	defer h.mx.Unlock()
	delete(h.outgoing, id)
}

func (h *hermesBroadcaster) onAccept(ctx context.Context, conn net.Conn) {
	logger := h.server.logger.WithField("address", conn.RemoteAddr())
	defer func() {
		if err := conn.Close(); err != nil {
			logger.WithError(err).Error("could not close connection")
		}
	}()

	if err := h.writeHeader(conn); err != nil {
		logger.WithError(err).Error("could not write header")
		return
	}

	id, outgoing := h.registerNew()
	defer h.unregister(id)

	logger.Info("started data stream")

	for data := range outgoing {
		conn.SetDeadline(time.Now().Add(h.idle))
		_, err := conn.Write(data)
		if err != nil {
			logger.WithError(err).Error("could not write data")
			logger.Warn("stopping stream early")
			return
		}
	}

	logger.Info("stopping stream")

}

func (h *hermesBroadcaster) writeHeader(w io.Writer) error {
	buf := proto.NewBuffer(nil)
	buf.EncodeMessage(&hermes.Header{
		Type: hermes.Header_Network,
		Version: &hermes.Version{
			Vmajor: 0,
			Vminor: 5,
		},
	})
	_, err := w.Write(buf.Bytes())
	return err
}
