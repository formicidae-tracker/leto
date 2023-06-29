package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/sirupsen/logrus"
)

// A server is a simple tcp server following the Task interface. It
// can be used to listen to multiple incoming connections, is
// cancelable via a context, and gracefully stop incoming connection.
type Server struct {
	wg          sync.WaitGroup
	connections sync.Map
	ctx         context.Context

	listener net.Listener
	logger   *logrus.Entry

	onAccept func(context.Context, net.Conn)
}

func NewServer(ctx context.Context, port int, domain string, grace time.Duration) (*Server, error) {
	logger := tm.NewLogger(domain)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	logger.Printf("started listening on :%d", port)

	s := &Server{
		ctx:      ctx,
		listener: listener,
		logger:   logger,
		onAccept: func(context.Context, net.Conn) {},
	}

	go func() {
		<-ctx.Done()
		s.logger.Printf("stop listening on :%d", port)
		s.gracefulStop(grace)
	}()

	return s, nil
}

func (s *Server) gracefulStop(grace time.Duration) {
	if err := s.listener.Close(); err != nil {
		s.logger.Printf("closing error: %s", err)
	}

	if s.waitAllDone(grace) == true {
		return
	}

	s.logger.Printf("force closing remaining connections")

	s.connections.Range(func(key, value any) bool {
		if err := value.(net.Conn).Close(); err != nil {
			s.logger.Printf("connection closing error: %s", err)
		}
		return true
	})
}

func (s *Server) waitAllDone(grace time.Duration) bool {
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()

	select {
	case <-done:
		return true
	case <-time.After(grace):
		s.logger.Printf("grace (%s) expired", grace)
		return false
	}
}

// Run loops over all incoming connections and call onAccept on them
// in a new go routine. Run() will returns after the ctx will be
// cancelled, and all onAccept returned.
func (s *Server) Run() error {
	defer func() {
		s.wg.Wait()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
				return err
			}
		}
		s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	s.wg.Add(1)
	s.connections.Store(conn, conn)
	go func() {
		defer s.wg.Done()
		s.onAccept(s.ctx, conn)
		s.connections.Delete(conn)
	}()
}
