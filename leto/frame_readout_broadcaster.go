package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/formicidae-tracker/hermes"
	"github.com/golang/protobuf/proto"
	"github.com/grandcat/zeroconf"
)

func BroadcastFrameReadout(address string, readouts <-chan *hermes.FrameReadout) error {
	m := NewRemoteManager()

	mx := sync.RWMutex{}
	outgoing := map[int]chan []byte{}

	go func() {
		for r := range readouts {
			b := proto.NewBuffer(nil)
			b.EncodeMessage(r)
			mx.RLock()
			for _, o := range outgoing {
				o <- b.Bytes()
			}
			mx.RUnlock()
		}
		m.Close()
		mx.Lock()
		defer mx.Unlock()
		for _, o := range outgoing {
			close(o)
		}
	}()
	i := 0
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	srv, err := zeroconf.Register(fmt.Sprintf("artemis.%s", hostname), "_artemis._tcp", "local.", 4001, nil, nil)
	if err != nil {
		return err
	}
	defer srv.Shutdown()
	log.Printf("Broadcasting on %s", address)
	return m.Listen(address, func(c net.Conn) {
		defer c.Close()
		logger := log.New(os.Stderr, fmt.Sprintf("[broadcast/%s]", c.RemoteAddr().String()), log.LstdFlags)

		b := proto.NewBuffer(nil)
		header := &hermes.Version{Major: 0, Minor: 5}
		b.EncodeMessage(header)

		_, err := c.Write(b.Bytes())
		if err != nil {
			logger.Printf("could not write header: %s", err)
			return
		}
		o := make(chan []byte, 10)
		mx.Lock()
		idx := i
		outgoing[idx] = o
		i += 1
		mx.Unlock()
		for buf := range o {
			_, err := c.Write(buf)
			if err != nil {
				logger.Printf("Could not write frame: %s", err)
				mx.Lock()
				close(o)
				delete(outgoing, idx)
				mx.Unlock()
			}
		}
	}, func() {
		log.Printf("Stopped broadcasting on %s", address)
	})
}
