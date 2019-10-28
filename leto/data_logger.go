package main

import (
	"container/ring"
	"time"
)

type DataLogger struct {
	buffer *ring.Ring
	size   int
	window time.Duration
}

func NewDataLogger(w time.Duration) *DataLogger {
	return &DataLogger{window: w, size: 0, buffer: nil}
}

type dataLoggerLog struct {
	value interface{}
	time  time.Time
}

func (l *DataLogger) Push(v interface{}) {
	t := time.Now()
	s := ring.New(1)
	s.Value = dataLoggerLog{
		time:  t,
		value: v,
	}

	if l.buffer == nil {
		l.buffer = s
	} else {
		l.buffer.Prev().Link(s)
	}
	l.size += 1
	for l.size > 0 {
		ll, ok := l.buffer.Value.(dataLoggerLog)
		if ok == false || ll.time.Add(l.window).After(t) == true {
			break
		}
		l.size -= 1
		l.buffer = l.buffer.Unlink(l.size)
	}
}

func (l *DataLogger) Do(do func(v interface{})) {
	l.buffer.Do(func(vv interface{}) {
		ll, ok := vv.(dataLoggerLog)
		if ok == true {
			do(ll.value)
		}
	})
}
