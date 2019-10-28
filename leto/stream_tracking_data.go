package main

import (
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/golang/protobuf/proto"
)

func BroadCastStreanSynchronizedFrameReadout(address string,
	readouts <-chan *hermes.FrameReadout,
	correspondances <-chan FrameCorrespondance) error {

	toBroadcast := make(chan []byte, 10)

	go func() {
		cBuff := []FrameCorrespondance{}
		rBuff := []*hermes.FrameReadout{}

		var streamStart int64
		var lastStreamFrame int64 = int64((^uint64(0)) >> 1)

		for readouts != nil && correspondances != nil {
			select {
			case c, ok := <-correspondances:
				if ok == false {
					readouts = nil
				}
				cBuff = append(cBuff, c)

			case r, ok := <-readouts:
				if ok == false {
					correspondances = nil
				}

				//makes a semi-shallow copy to avoid data race conditions
				toSend := *r
				r.ProducerUuid = ""

				rBuff = append(rBuff, &toSend)
			}

			rIdxToKeep := 0
			cIdxToKeep := 0
			for rIdx, readout := range rBuff {
				for cIdx, c := range cBuff {
					if c.TrackingFrameID != readout.FrameID {
						continue
					}
					if lastStreamFrame > c.StreamFrameID {
						streamStart = readout.Timestamp
					}
					lastStreamFrame = c.StreamFrameID
					readout.Timestamp = readout.Timestamp - streamStart

					b := proto.NewBuffer(nil)
					b.EncodeMessage(readout)
					select {
					case toBroadcast <- b.Bytes():
					default:
					}

					rIdxToKeep = rIdx + 1
					cIdxToKeep = cIdx + 1
				}
			}

			rBuff = rBuff[rIdxToKeep:]
			cBuff = cBuff[cIdxToKeep:]
		}
		close(toBroadcast)
	}()

	b := BinaryDataBroadcaster{
		Address: address,
		Name:    "stream-sync-broadcast",
		Idle:    1 * time.Second,
		Backlog: 2 * time.Minute,
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
