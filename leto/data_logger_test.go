package main

import (
	"time"

	. "gopkg.in/check.v1"
)

type DataLoggerSuite struct {
}

var _ = Suite(&DataLoggerSuite{})

func (s *DataLoggerSuite) Test(c *C) {

	l := NewDataLogger(1 * time.Millisecond)

	for i := 0; i < 10; i++ {
		l.Push(i)
		if i == 0 {
			time.Sleep(500 * time.Microsecond)
		}
	}
	time.Sleep(600 * time.Microsecond)
	l.Push(10)

	size := 0
	expected := 1
	l.Do(func(v interface{}) {
		vv := v.(int)
		size += 1
		c.Check(vv, Equals, expected)
		expected += 1
	})

	c.Check(size, Equals, 10)
}
