package main

import (
	"time"

	. "gopkg.in/check.v1"
)

type ByteRateEstimatorSuite struct {
}

var _ = Suite(&ByteRateEstimatorSuite{})

func (s *ByteRateEstimatorSuite) TestEstimation(c *C) {
	e := NewByteRateEstimator(10*1024*1024, time.Unix(0, 0))
	testdata := []struct {
		Free     int64
		Time     time.Time
		Expected int64
	}{
		{Free: 10*1024*1024 - 100, Time: time.Unix(1, 0), Expected: 100},
		{Free: 10*1024*1024 - 150, Time: time.Unix(2, 0), Expected: 80},
		{Free: 10*1024*1024 - 200, Time: time.Unix(3, 0), Expected: 69},
		{Free: 10*1024*1024 - 250, Time: time.Unix(4, 0), Expected: 63},
		{Free: 10*1024*1024 - 300, Time: time.Unix(5, 0), Expected: 60},
		{Free: 10*1024*1024 - 350, Time: time.Unix(6, 0), Expected: 58},
		{Free: 10*1024*1024 - 400, Time: time.Unix(7, 0), Expected: 57},
		{Free: 10*1024*1024 - 450, Time: time.Unix(8, 0), Expected: 56},
		// some process liberated a lot of space in another dir.
		{Free: 100 * 1024 * 1024, Time: time.Unix(9, 0), Expected: 56},
		// the mean bps is reset from here
		{Free: 100*1024*1024 - 50, Time: time.Unix(10, 0), Expected: 50},
		{Free: 100*1024*1024 - 100, Time: time.Unix(11, 0), Expected: 50},
		{Free: 100*1024*1024 - 150, Time: time.Unix(12, 0), Expected: 50},
		{Free: 100*1024*1024 - 200, Time: time.Unix(13, 0), Expected: 50},
	}

	for _, d := range testdata {
		c.Check(e.Estimate(d.Free, d.Time), Equals, d.Expected)
	}

}
