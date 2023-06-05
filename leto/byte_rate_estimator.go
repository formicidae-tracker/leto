package main

import (
	"math"
	"sync"
	"time"
)

// byteRateEstimator estimate the mean write speed of a long process
// (several minute / hours / days ) on the disk. It is able to discard
// punctual events.
type byteRateEstimator struct {
	mx             sync.Mutex
	freeStartBytes int64
	startTime      time.Time
	mean           float64
}

func NewByteRateEstimator(freeBytes int64, t time.Time) *byteRateEstimator {
	return &byteRateEstimator{
		freeStartBytes: freeBytes,
		startTime:      t,
		mean:           math.NaN(),
	}
}

func (e *byteRateEstimator) Estimate(freeBytes int64, t time.Time) int64 {
	e.mx.Lock()
	defer e.mx.Unlock()
	ellapsed := t.Sub(e.startTime)
	written := e.freeStartBytes - freeBytes

	bps := float64(written) / ellapsed.Seconds()

	if math.IsNaN(e.mean) {
		e.mean = bps
		return int64(bps)
	}

	if math.Abs((bps-e.mean)/e.mean) > 0.5 {
		// more than 50% relative variation, an external event
		// happened. We restart the start point and mean estimation.
		bps = e.mean
		e.startTime = t
		e.freeStartBytes = freeBytes
		e.mean = math.NaN()
	} else {
		// we slowly update the mean.
		e.mean += 0.8 * (bps - e.mean)
		bps = e.mean
	}

	return int64(bps)
}
