package main

import (
	"sync"
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/golang/protobuf/ptypes"
	. "gopkg.in/check.v1"
)

type FrameReadoutMergerSuite struct{}

var _ = Suite(&FrameReadoutMergerSuite{})

func (s *FrameReadoutMergerSuite) TestEnd2End(c *C) {
	testdata := []struct {
		T  time.Duration
		TS int64
		R  *hermes.FrameReadout
	}{
		{
			T:  0 * time.Microsecond,
			TS: 1000,
			R: &hermes.FrameReadout{
				FrameID:      0,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "foo",
			},
		},
		{
			T:  9 * time.Microsecond,
			TS: 509,
			R: &hermes.FrameReadout{
				FrameID:      1,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  21 * time.Microsecond,
			TS: 1021,
			R: &hermes.FrameReadout{
				FrameID:      2,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "foo",
			},
		},
		{
			T:  29 * time.Microsecond,
			TS: 529,
			R: &hermes.FrameReadout{
				FrameID:      3,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  47 * time.Microsecond,
			TS: 547,
			R: &hermes.FrameReadout{
				FrameID:      5,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  49 * time.Microsecond,
			TS: 1049,
			R: &hermes.FrameReadout{
				FrameID:      4,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "foo",
			},
		},
		{
			T:  69 * time.Microsecond,
			TS: 569,
			R: &hermes.FrameReadout{
				FrameID:      7,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  90 * time.Microsecond,
			TS: 590,
			R: &hermes.FrameReadout{
				FrameID:      9,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  100 * time.Microsecond,
			TS: 1100,
			R: &hermes.FrameReadout{
				FrameID:      10,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "foo",
			},
		},
		{
			T:  110 * time.Microsecond,
			TS: 610,
			R: &hermes.FrameReadout{
				FrameID:      11,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  120 * time.Microsecond,
			TS: 1120,
			R: &hermes.FrameReadout{
				FrameID:      12,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "foo",
			},
		},
		{
			T:  130 * time.Microsecond,
			TS: 630,
			R: &hermes.FrameReadout{
				FrameID:      13,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  140 * time.Microsecond,
			TS: 1140,
			R: &hermes.FrameReadout{
				FrameID:      14,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "foo",
			},
		},
		{
			T:  150 * time.Microsecond,
			TS: 650,
			R: &hermes.FrameReadout{
				FrameID:      15,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  160 * time.Microsecond,
			TS: 1160,
			R: &hermes.FrameReadout{
				FrameID:      16,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "foo",
			},
		},
		{
			T:  170 * time.Microsecond,
			TS: 670,
			R: &hermes.FrameReadout{
				FrameID:      17,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  180 * time.Microsecond,
			TS: 1180,
			R: &hermes.FrameReadout{
				FrameID:      18,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "foo",
			},
		},
		{
			T:  190 * time.Microsecond,
			TS: 690,
			R: &hermes.FrameReadout{
				FrameID:      19,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "bar",
			},
		},
		{
			T:  191 * time.Microsecond,
			TS: 1191,
			R: &hermes.FrameReadout{
				FrameID:      6,
				Error:        hermes.FrameReadout_NO_ERROR,
				ProducerUuid: "foo",
			},
		},
	}

	expected := []*hermes.FrameReadout{
		{
			FrameID:      0,
			Timestamp:    1000,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      1,
			Timestamp:    1009,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      2,
			Timestamp:    1021,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      3,
			Timestamp:    1029,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      4,
			Timestamp:    1049,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      5,
			Timestamp:    1047,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      6,
			Timestamp:    0,
			Error:        hermes.FrameReadout_PROCESS_TIMEOUT,
			ProducerUuid: "",
		},
		{
			FrameID:      7,
			Timestamp:    1069,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      8,
			Timestamp:    0,
			Error:        hermes.FrameReadout_PROCESS_TIMEOUT,
			ProducerUuid: "",
		},
		{
			FrameID:      9,
			Timestamp:    1090,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      10,
			Timestamp:    1100,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      11,
			Timestamp:    1110,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      12,
			Timestamp:    1120,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      13,
			Timestamp:    1130,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      14,
			Timestamp:    1140,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      15,
			Timestamp:    1150,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      16,
			Timestamp:    1160,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      17,
			Timestamp:    1170,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      18,
			Timestamp:    1180,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
		{
			FrameID:      19,
			Timestamp:    1190,
			Error:        hermes.FrameReadout_NO_ERROR,
			ProducerUuid: "",
		},
	}

	inbound := make(chan *hermes.FrameReadout, 100)
	outbound := make(chan *hermes.FrameReadout, len(testdata))

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		start := time.Now()
		for _, d := range testdata {
			time.Sleep(start.Add(d.T).Sub(time.Now()))
			d.R.Time, _ = ptypes.TimestampProto(start.Add(d.T))
			d.R.Timestamp = d.TS
			inbound <- d.R
		}
		close(inbound)
		wg.Done()
	}()
	wb := &WorkloadBalance{
		FPS:        10000.0,
		Stride:     2,
		MasterUUID: "foo",
		IDsByUUID: map[string][]bool{
			"foo": {true, false},
			"bar": {false, true},
		},
	}

	go func() {
		err := MergeFrameReadout(wb, inbound, outbound)
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
