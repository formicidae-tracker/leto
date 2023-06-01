package main

import (
	"context"
	"sync"
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/golang/protobuf/ptypes"
	. "gopkg.in/check.v1"
)

type FrameReadoutMergerSuite struct{}

var _ = Suite(&FrameReadoutMergerSuite{})

func (s *FrameReadoutMergerSuite) TestEnd2End(c *C) {
	fps := 1000.0
	period := time.Duration(float64(time.Second.Nanoseconds()) / fps)
	baseTimeStamp := map[string]int64{
		"foo": 1000,
		"bar": 500,
	}

	jitters := map[int64]time.Duration{
		1:  -1 * time.Microsecond,
		2:  1 * time.Microsecond,
		3:  -1 * time.Microsecond,
		5:  -3 * time.Microsecond,
		4:  -1 * time.Microsecond,
		7:  -1 * time.Microsecond,
		9:  1 * time.Microsecond,
		12: 2 * time.Microsecond,
		6:  3 * time.Microsecond,
	}
	frameIDs := []int64{0, 1, 2, 3, 5, 4, 7, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 6}

	expected := make([]*hermes.FrameReadout, len(frameIDs)+1)
	for i := range expected {

		jitter, _ := jitters[int64(i)]
		expected[i] = &hermes.FrameReadout{
			FrameID:   int64(i),
			Timestamp: int64(1000) + int64(i)*period.Microseconds() + jitter.Microseconds(),
			Error:     hermes.FrameReadout_NO_ERROR,
		}
	}
	expected[6].Error = hermes.FrameReadout_PROCESS_TIMEOUT
	expected[6].Timestamp = 0
	expected[8].Error = hermes.FrameReadout_PROCESS_TIMEOUT
	expected[8].Timestamp = 0

	inbound := make(chan *hermes.FrameReadout, len(frameIDs))
	outbound := make(chan *hermes.FrameReadout, len(frameIDs))

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		start := time.Now()
		for _, ID := range frameIDs {
			producerUuid := "foo"
			if ID%2 == 1 {
				producerUuid = "bar"
			}
			jitter, _ := jitters[ID]
			delta := time.Duration(ID)*period + jitter
			deadline := start.Add(delta)
			ts := baseTimeStamp[producerUuid] + int64(delta.Microseconds())
			frame := &hermes.FrameReadout{
				FrameID:      ID,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: producerUuid,
				Timestamp:    ts,
			}
			frame.Time, _ = ptypes.TimestampProto(deadline)

			time.Sleep(deadline.Sub(time.Now()))
			inbound <- frame
		}
		close(inbound)
		wg.Done()
	}()
	wb := &WorkloadBalance{
		FPS:        fps,
		Stride:     2,
		MasterUUID: "foo",
		IDsByUUID: map[string][]bool{
			"foo": {true, false},
			"bar": {false, true},
		},
	}

	go func() {
		err := MergeFrameReadout(context.TODO(), wb, inbound, outbound)
		c.Check(err, IsNil)
		if err != nil {
			for range inbound {
			}
		}
		wg.Done()
	}()

	i := 0
	for r := range outbound {
		if c.Check(i < len(expected), Equals, true) == false {
			i += 1
			continue
		}
		comment := Commentf("Expected[%d]: %+v", i, expected[i])

		c.Check(r.FrameID, Equals, expected[i].FrameID, comment)
		c.Check(r.Error, Equals, expected[i].Error, comment)
		c.Check(r.ProducerUuid, Equals, expected[i].ProducerUuid, comment)
		if r.Error != hermes.FrameReadout_PROCESS_TIMEOUT {
			c.Check(r.Timestamp, Equals, expected[i].Timestamp, comment)
		} else {
			c.Check(r.Timestamp, Equals, int64(0), comment)
		}
		i += 1
	}
	c.Check(expected, HasLen, i)
	wg.Wait()

}
