package main

import (
	"context"
	"net"
	"time"

	"github.com/formicidae-tracker/hermes"
	. "gopkg.in/check.v1"
)

type HermesBroadcasterSuite struct {
	broadcaster HermesBroadcaster
	cancel      func()
	err         <-chan error
}

var _ = Suite(&HermesBroadcasterSuite{})

func (s *HermesBroadcasterSuite) SetUpTest(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	var err error
	s.broadcaster, err = NewHermesBroadcaster(ctx, 12345, 30*time.Millisecond)
	c.Assert(err, IsNil)

	s.err = Start(s.broadcaster)
}

func (s *HermesBroadcasterSuite) TearDownTest(c *C) {
	close(s.broadcaster.Incoming())
	s.cancel()
	err, ok := <-s.err
	c.Check(err, IsNil)
	c.Check(ok, Equals, true)

	err, ok = <-s.err
	c.Check(err, IsNil)
	c.Check(ok, Equals, false)
}

func (s *HermesBroadcasterSuite) TestNoConnections(c *C) {
	// We should not block on any incoming, just drop the data
	for i := 0; i < 100; i++ {
		s.broadcaster.Incoming() <- &hermes.FrameReadout{}
	}

}

func (s *HermesBroadcasterSuite) TestNatsyDisconnectingClientMustNotPanic(c *C) {

	testdata := []*hermes.FrameReadout{
		{
			FrameID: 1,
			Error:   hermes.FrameReadout_ILLUMINATION_ERROR,
		},
		{
			FrameID: 2,
			Error:   hermes.FrameReadout_ILLUMINATION_ERROR,
		},
		{
			FrameID: 3,
			Error:   hermes.FrameReadout_ILLUMINATION_ERROR,
		},
	}
	deadline := time.Now().Add(1 * time.Millisecond)
	for i := 0; i < 10; i++ {
		for _, d := range testdata {
			conn, err := net.Dial("tcp", "localhost:12345")
			c.Assert(err, IsNil)

			h := hermes.Header{}
			ok, err := hermes.ReadDelimitedMessage(conn, &h)
			c.Assert(ok, Equals, true)
			c.Assert(err, IsNil)
			time.Sleep(deadline.Sub(time.Now()))
			deadline = deadline.Add(1 * time.Millisecond)
			s.broadcaster.Incoming() <- d
			ro := hermes.FrameReadout{}
			ok, err = hermes.ReadDelimitedMessage(conn, &ro)
			conn.Close()
		}
	}

}
