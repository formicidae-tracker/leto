package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

// A server is a simple tcp server following the Task interface. It
// can be used to listen to multiple incoming connections, is
// cancelable via a context, and gracefully stop incoming connection.
type Server struct {
	wg          sync.WaitGroup
	connections sync.Map
	ctx         context.Context

	err      chan error
	listener net.Listener
	logger   *log.Logger

	onAccept func(context.Context, net.Conn)
	onClose  func()
}

func NewServer(ctx context.Context, port int, logPrefix string, grace time.Duration) (*Server, error) {
	logger := log.New(os.Stderr, logPrefix, 0)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	logger.Printf("started listening on :%d", port)

	s := &Server{
		ctx:      ctx,
		err:      make(chan error),
		listener: listener,
		logger:   logger,
		onClose:  func() {},
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

func (s *Server) Start() {
	go func() {
		defer close(s.err)
		s.err <- s.loop()
	}()
}

func (s *Server) Done() <-chan error {
	return s.err
}

func (s *Server) loop() error {
	defer func() {
		s.wg.Wait()
		s.onClose()
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
