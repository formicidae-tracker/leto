package main

import (
	"github.com/formicidae-tracker/hermes"
	. "gopkg.in/check.v1"
)

type FrameReadoutBroadcasterSuite struct {
	C chan *hermes.FrameReadout
}

var _ = Suite(&FrameReadoutBroadcasterSuite{})

func (s *FrameReadoutBroadcasterSuite) TestNatsyDisconnectingClientMustNotPanic(c *C) {
	// s.C = make(chan *hermes.FrameReadout, 10)
	// wg := sync.WaitGroup{}
	// wg.Add(1)
	// go func() {
	// 	c.Assert(BroadcastFrameReadout(":4002", s.C, 100*time.Microsecond), IsNil)
	// 	wg.Done()
	// }()

	// testdata := []*hermes.FrameReadout{
	// 	&hermes.FrameReadout{
	// 		FrameID: 1,
	// 		Error:   hermes.FrameReadout_ILLUMINATION_ERROR,
	// 	},
	// 	&hermes.FrameReadout{
	// 		FrameID: 2,
	// 		Error:   hermes.FrameReadout_ILLUMINATION_ERROR,
	// 	},
	// 	&hermes.FrameReadout{
	// 		FrameID: 3,
	// 		Error:   hermes.FrameReadout_ILLUMINATION_ERROR,
	// 	},
	// }
	// deadline := time.Now().Add(1 * time.Millisecond)
	// for i := 0; i < 10; i++ {
	// 	for _, d := range testdata {
	// 		conn, err := net.Dial("tcp", "localhost:4002")
	// 		c.Assert(err, IsNil)

	// 		h := hermes.Header{}
	// 		ok, err := hermes.ReadDelimitedMessage(conn, &h)
	// 		c.Assert(ok, Equals, true)
	// 		c.Assert(err, IsNil)
	// 		time.Sleep(deadline.Sub(time.Now()))
	// 		deadline = deadline.Add(1 * time.Millisecond)
	// 		s.C <- d
	// 		ro := hermes.FrameReadout{}
	// 		ok, err = hermes.ReadDelimitedMessage(conn, &ro)
	// 		conn.Close()
	// 	}
	// }

	// log.Printf("closing")
	// close(s.C)
	// log.Printf("waiting graceful shutdown")
	// wg.Wait()
	// log.Printf("done")

}
