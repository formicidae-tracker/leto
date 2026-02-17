package leto

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/formicidae-tracker/olympus/pkg/tm"
)

// A server is a simple tcp server following the Task interface. It
// can be used to listen to multiple incoming connections, is
// cancelable via a context, and gracefully stop incoming connection.
type Server struct {
	wg          sync.WaitGroup
	connections sync.Map
	ctx         context.Context

	listener net.Listener
	Logger   *slog.Logger

	OnAccept func(context.Context, net.Conn)
}

func NewServer(ctx context.Context, port int, domain string, grace time.Duration) (*Server, error) {
	logger := tm.NewLogger(domain)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	logger.With(slog.Int("port", port)).InfoContext(ctx, "started listening")

	s := &Server{
		ctx:      ctx,
		listener: listener,
		Logger:   logger,
		OnAccept: func(context.Context, net.Conn) {},
	}

	go func() {
		<-ctx.Done()
		s.Logger.With(slog.Int("port", port)).InfoContext(ctx, "stop listening")
		s.gracefulStop(grace)
	}()

	return s, nil
}

func (s *Server) gracefulStop(grace time.Duration) {
	if err := s.listener.Close(); err != nil {
		s.Logger.With("error", err).ErrorContext(s.ctx, "closing error")
	}

	if s.waitAllDone(grace) == true {
		return
	}

	s.Logger.WarnContext(s.ctx, "force closing remaining connections")

	s.connections.Range(func(key, value any) bool {
		if err := value.(net.Conn).Close(); err != nil {
			s.Logger.With("error", err).ErrorContext(s.ctx, "connection closing error")
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
		s.Logger.With(slog.Duration("period", grace)).WarnContext(s.ctx, "grace expired")
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
		s.OnAccept(s.ctx, conn)
		s.connections.Delete(conn)
	}()
}
