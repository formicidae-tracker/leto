package main

import (
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/golang/protobuf/proto"
)

func BroadcastFrameReadout(address string, readouts <-chan *hermes.FrameReadout, idle time.Duration) error {
	toBroadcast := make(chan []byte, 10)
	go func() {
		for r := range readouts {
			b := proto.NewBuffer(nil)
			b.EncodeMessage(r)
			toBroadcast <- b.Bytes()
		}
		close(toBroadcast)
	}()

	b := BinaryDataBroadcaster{
		Address: address,
		Name:    "broadcast",
		Idle:    idle,
		Backlog: 0,
	}

	henc := proto.NewBuffer(nil)
	header := &hermes.Header{
		Type: hermes.Header_Network,
		Version: &hermes.Version{
			Vmajor: 0,
			Vminor: 5,
		},
	}
	henc.EncodeMessage(header)

	return b.Broadcast(toBroadcast, func() []byte {
		return henc.Bytes()
	})
}
