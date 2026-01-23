package main

import (
	"bytes"
	"context"
	"math"
	"math/rand"

	"github.com/formicidae-tracker/hermes/src/go/hermes"
	. "gopkg.in/check.v1"

	"github.com/golang/protobuf/proto"
)

type FrameReadoutUtilsSuite struct{}

var _ = Suite(&FrameReadoutUtilsSuite{})

func checkReadoutEqual(c *C, obtained, expected *hermes.FrameReadout) {
	c.Check(obtained.Timestamp, Equals, expected.Timestamp)
	c.Check(obtained.FrameID, Equals, expected.FrameID)
	c.Check(obtained.Time, DeepEquals, expected.Time)
	c.Check(obtained.Error, Equals, expected.Error)
	c.Check(obtained.ProducerUuid, Equals, expected.ProducerUuid)
	c.Check(obtained.Quads, Equals, expected.Quads)
	c.Check(obtained.Width, Equals, expected.Width)
	c.Check(obtained.Height, Equals, expected.Height)
	c.Check(obtained.Cuestart, DeepEquals, expected.Cuestart)
	c.Check(obtained.Cueend, DeepEquals, expected.Cueend)
	c.Check(len(obtained.Tags), Equals, len(expected.Tags))
	size := Min(len(obtained.Tags), len(expected.Tags))
	for i := 0; i < size; i++ {
		checkTagEqual(c, obtained.Tags[i], expected.Tags[i], Commentf("idx=%d", i))
	}
}

func checkTagEqual(c *C, obtained, expected *hermes.Tag, cmt interface{}) {
	c.Check(obtained.ID, Equals, expected.ID, cmt)
	c.Check(obtained.X, Equals, expected.X, cmt)
	c.Check(obtained.Y, Equals, expected.Y, cmt)
	c.Check(obtained.Theta, Equals, expected.Theta, cmt)
}

func (s *FrameReadoutUtilsSuite) TestHelloWorld(c *C) {
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

	go ReadAllFrameReadout(context.TODO(), data, C, E)

	i := 0
	for {
		select {
		case m, ok := <-C:
			c.Assert(ok, Equals, true)
			c.Assert(i < len(testdata), Equals, true)

			checkReadoutEqual(c, m, testdata[i])
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
