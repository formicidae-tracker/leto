package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type BinaryDataBroadcaster struct {
	Address string
	Name    string
	Idle    time.Duration
	Backlog time.Duration
}

func (b BinaryDataBroadcaster) Broadcast(stream <-chan []byte, onNewConnection func() []byte) error {
	logger := log.New(os.Stderr, fmt.Sprintf("[%s] ", b.Name), log.LstdFlags)
	m := NewRemoteManager()
	mx := sync.RWMutex{}
	outgoing := map[int]chan []byte{}

	var dlogger *DataLogger

	if b.Backlog > 0 {
		dlogger = NewDataLogger(b.Backlog)
	}

	go func() {
		for b := range stream {
			mx.RLock()
			if dlogger != nil {
				dlogger.Push(b)
			}

			for _, o := range outgoing {
				o <- b
			}
			mx.RUnlock()
		}
		m.Close()
		mx.Lock()
		defer mx.Unlock()
		for _, o := range outgoing {
			close(o)
		}
		outgoing = nil
	}()

	logger.Printf("Listening on %s", b.Address)

	i := 0
	return m.Listen(b.Address, func(c net.Conn) {
		defer c.Close()
		logger := log.New(os.Stderr, fmt.Sprintf("[%s/%s] ", b.Name, c.RemoteAddr().String()), log.LstdFlags)
		logger.Printf("new connection")

		_, err := c.Write(onNewConnection())
		if err != nil {
			logger.Printf("could not write header: %s", err)
			return
		}
		o := make(chan []byte, 10)
		mx.Lock()
		idx := i
		outgoing[idx] = o
		i += 1
		if dlogger != nil {
			dlogger.Do(func(v interface{}) {
				c.SetWriteDeadline(time.Now().Add(b.Idle))
				_, err := c.Write(v.([]byte))
				if err != nil {
					logger.Printf("Could not write backlogged data: %s", err)
				}
			})
		}
		mx.Unlock()

		for buf := range o {
			c.SetWriteDeadline(time.Now().Add(b.Idle))
			_, err := c.Write(buf)
			if err != nil {
				logger.Printf("Could not write data: %s", err)
				mx.Lock()
				close(o)
				delete(outgoing, idx)
				mx.Unlock()
				return // need an explicit return as otherwise it may loop again and close it twice
			}
		}
	}, func() {
		logger.Printf("Stopped listening")
	})
}
