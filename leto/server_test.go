package main

import (
	"context"
	"net"
	"time"

	. "gopkg.in/check.v1"
)

type ServerSuite struct {
	server *Server
	cancel func()
	err    <-chan error
}

var _ = Suite(&ServerSuite{})

func (s *ServerSuite) SetUpTest(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	var err error
	s.server, err = NewServer(ctx, 12345, "[leto-tests]: ", 20*time.Millisecond)
	c.Assert(err, IsNil)
	s.err = Start(s.server)
}

func (s *ServerSuite) TearDownTest(c *C) {
	s.cancel()
	err, ok := <-s.err
	if ok == false {
		// already closed by test, nothing to check
		return
	}
	// if not closed by tests, no error should happen
	c.Check(err, IsNil)
}

func (s *ServerSuite) TestDoesNotWaitOnAllClosedConnection(c *C) {
	conn, err := net.Dial("tcp", "localhost:12345")
	c.Assert(err, IsNil)
	conn.Close()
	s.cancel()
	select {
	case <-time.After(1 * time.Millisecond):
		c.Fatalf("server waited on closed connection")
	case err := <-s.err:
		c.Check(err, IsNil)
	}
}

func (s *ServerSuite) TestClosesAllConnectionAfterGrace(c *C) {
	connected := make(chan struct{})
	done := make(chan struct{})
	s.server.onAccept = func(ctx context.Context, conn net.Conn) {
		close(connected)
		data := make([]byte, 10)
		_, err := conn.Read(data)
		c.Check(err, ErrorMatches, ".*use of closed network connection")
		close(done)
	}
	conn, err := net.Dial("tcp", "localhost:12345")
	<-connected
	c.Check(conn, Not(IsNil))
	c.Assert(err, IsNil)
	select {
	case <-done:
		c.Fatalf("done before cancel")
	default:
	}

	s.cancel()

	select {
	case <-s.err:
		c.Errorf("server done before connection closed")
	case <-done:
	case <-time.After(40 * time.Millisecond):
		c.Fatalf("server never close")
	}
	err = <-s.err
	c.Check(err, IsNil)
}

// func (s *ServerSuite) TestManager(c *C) {
// 	readouts := make(chan *hermes.FrameReadout)
// 	closed := make(chan struct{})
// 	quit := make(chan struct{})

// 	m := NewRemoteManager()

// 	go m.Listen(":12345",
// 		func(conn net.Conn) {
// 			errors := make(chan error)
// 			go func() {
// 				for e := range errors {
// 					c.Assert(e, IsNil)
// 				}
// 			}()
// 			FrameReadoutReadAll(conn, readouts, errors)
// 		},
// 		func() {
// 			close(readouts)
// 		})
// 	go func() {
// 		conn, err := net.Dial("tcp", "localhost:12345")
// 		c.Assert(err, IsNil)
// 		time.Sleep(10 * time.Millisecond)
// 		conn.Close()
// 		close(closed)
// 	}()
// 	<-closed
// 	wg := sync.WaitGroup{}
// 	wg.Add(2)
// 	for i := 0; i < 2; i++ {
// 		go func() {
// 			conn, err := net.Dial("tcp", "localhost:12345")
// 			c.Assert(err, IsNil)
// 			<-quit
// 			conn.Close()
// 			wg.Done()
// 		}()
// 	}

// 	time.Sleep(70 * time.Millisecond)
// 	m.mx.Lock()
// 	c.Check(len(m.connections), Equals, 3)
// 	m.mx.Unlock()
// 	close(quit)
// 	wg.Wait()
// 	c.Check(m.Close(), IsNil)
// 	m.mx.Lock()
// 	for _, conn := range m.connections {
// 		c.Check(conn, IsNil)
// 	}
// 	m.mx.Unlock()
// 	_, ok := <-readouts
// 	c.Check(ok, Equals, false)

// }
