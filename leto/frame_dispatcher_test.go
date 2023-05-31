package main

import (
	"sync"
	"time"

	"github.com/formicidae-tracker/hermes"
	. "gopkg.in/check.v1"
)

type FrameDispatcherSuite struct {
	dispatcher FrameDispatcher
	out1, out2 chan *hermes.FrameReadout
	err        <-chan error
}

var _ = Suite(&FrameDispatcherSuite{})

func (s *FrameDispatcherSuite) SetUpTest(c *C) {
	s.out1 = make(chan *hermes.FrameReadout, 1)
	s.out2 = make(chan *hermes.FrameReadout, 1)
	s.dispatcher = NewFrameDispatcher(s.out1, s.out2)
	s.err = Start(s.dispatcher)
}

func (s *FrameDispatcherSuite) TearDownTest(c *C) {
	err := <-s.err
	c.Check(err, IsNil)
}

func (s *FrameDispatcherSuite) TestCloseChannelOnTermination(c *C) {
	close(s.dispatcher.Incoming())
	_, ok := <-s.out1
	c.Check(ok, Equals, false)
	_, ok = <-s.out2
	c.Check(ok, Equals, false)
}

func (s *FrameDispatcherSuite) TestE2E(c *C) {
	var wg sync.WaitGroup

	var res1, res2 []int64

	wg.Add(2)
	go func() {
		defer wg.Done()
		r1 := <-s.out1
		r2 := <-s.out1
		res1 = append(res1, r1.FrameID, r2.FrameID)
	}()
	go func() {
		defer wg.Done()
		r1 := <-s.out2
		res2 = append(res2, r1.FrameID)
	}()

	time.Sleep(10 * time.Millisecond)
	s.dispatcher.Incoming() <- &hermes.FrameReadout{FrameID: 1}
	s.dispatcher.Incoming() <- &hermes.FrameReadout{FrameID: 2}

	wg.Wait()

	wg.Add(2)
	go func() {
		defer wg.Done()
		for r := range s.out1 {
			res1 = append(res1, r.FrameID)
		}
	}()
	go func() {
		defer wg.Done()
		for r := range s.out2 {
			res2 = append(res2, r.FrameID)
		}
	}()

	s.dispatcher.Incoming() <- &hermes.FrameReadout{FrameID: 3}
	close(s.dispatcher.Incoming())

	wg.Wait()

	c.Check(res1, DeepEquals, []int64{1, 2, 3})
	c.Check(res2, DeepEquals, []int64{1, 2})

}

func (s *FrameDispatcherSuite) TestHandleChannelPanics(c *C) {
	close(s.out1) // closing will make the routine panic
	s.dispatcher.Incoming() <- &hermes.FrameReadout{}
	err, ok := <-s.err
	c.Check(ok, Equals, true)
	c.Check(err, ErrorMatches, "dispatch did panic:.*")
}
