package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"math"
	"math/rand"
	"testing"

	"github.com/formicidae-tracker/hermes"
	. "gopkg.in/check.v1"

	"github.com/golang/protobuf/proto"
)

var logstostderr = flag.Bool("logstostderr", false, "leaves module log to stderr, otherwise it will be discarded")

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) {
	if *logstostderr == false {
		logOut = io.Discard
	}
	TestingT(t)
}

type FrameReadoutReaderSuite struct{}

var _ = Suite(&FrameReadoutReaderSuite{})

func (s *FrameReadoutReaderSuite) TestHelloWorld(c *C) {
	testdata := []*hermes.FrameReadout{
		{
			Timestamp:    0,
			FrameID:      0,
			Tags:         nil,
			Time:         nil,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "foo",
		},
		{
			Timestamp:    100000,
			FrameID:      42,
			Tags:         nil,
			Time:         nil,
			Error:        hermes.FrameReadout_PROCESS_OVERFLOW,
			ProducerUuid: "foo",
		},
	}

	for i := 0; i < 1000; i++ {
		a := &hermes.Tag{
			ID:    uint32(rand.Intn(20000)),
			X:     rand.Float64() * 1000.0,
			Y:     rand.Float64() * 1000.0,
			Theta: rand.Float64() * 2.0 * math.Pi,
		}

		testdata[0].Tags = append(testdata[0].Tags, a)
	}

	b := proto.NewBuffer(nil)

	for _, m := range testdata {
		b.EncodeMessage(m)
	}

	C := make(chan *hermes.FrameReadout)
	E := make(chan error)

	data := bytes.NewBuffer(b.Bytes())

	go FrameReadoutReadAll(context.TODO(), data, C, E)

	i := 0
	for {
		select {
		case m, ok := <-C:
			c.Assert(ok, Equals, true)
			c.Assert(i < len(testdata), Equals, true)
			c.Check(m, DeepEquals, testdata[i])
			i += 1
		case err, ok := <-E:
			if ok == false {
				E = nil
				break
			}
			c.Check(err, IsNil)

		}
		if E == nil {
			break
		}
	}
	close(C)
	c.Check(i, Equals, len(testdata))

	//test if we can clear the uuid data when saving to disk (no need
	//to save a lot of unucessary data)
	testdata[len(testdata)-1].ProducerUuid = "foobar"
	b = proto.NewBuffer(nil)
	b.EncodeMessage(testdata[len(testdata)-1])
	sizeWithUuid := len(b.Bytes())
	testdata[len(testdata)-1].ProducerUuid = ""
	b = proto.NewBuffer(nil)
	b.EncodeMessage(testdata[len(testdata)-1])
	sizeWithoutUuid := len(b.Bytes())
	c.Check((sizeWithUuid-sizeWithoutUuid) > len("foobar"), Equals, true)
}
