package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/golang/protobuf/ptypes"
)

type synchronizationPoint struct {
	time        time.Time
	timestampUS int64
}

func (p synchronizationPoint) computeOffset(t time.Time, timestampUs int64) float64 {
	return float64(p.timestampUS) + float64(t.Sub(p.time).Nanoseconds())*1.0e-3 - float64(timestampUs)
}

type WorkloadBalance struct {
	FPS        float64
	Stride     int
	MasterUUID string
	lastPoint  *synchronizationPoint
	offsets    map[string]float64
	IDsByUUID  map[string][]bool
}

func (wb *WorkloadBalance) Check() error {
	if len(wb.MasterUUID) == 0 {
		return fmt.Errorf("Work Balance is missing master UUID")
	}
	wb.offsets = make(map[string]float64)
	wb.lastPoint = nil
	fids := map[int]string{}

	if len(wb.IDsByUUID) > wb.Stride {
		return fmt.Errorf("WorkloadBalance: more Producer than Stride (%d): %v", wb.Stride, wb.IDsByUUID)
	}

	for puuid, ids := range wb.IDsByUUID {
		if len(ids) != wb.Stride {
			return fmt.Errorf("WorkloadBalance: invalid id definition for producer %s, require %d but got %v", puuid, wb.Stride, ids)
		}
		for i, set := range ids {
			if set == false {
				continue
			}
			if other, ok := fids[i]; ok == true {
				return fmt.Errorf("WorkloadBalance: Producer %s: Frame %d mod[%d] are already produced by %s", puuid, i, wb.Stride, other)
			}
			fids[i] = puuid
		}
	}

	for i := 0; i < wb.Stride; i++ {
		if _, ok := fids[i]; ok == false {
			return fmt.Errorf("WorkloadBalance: No producer set for frame %d mod[%d]", i, wb.Stride)
		}
	}

	return nil
}

func (wb *WorkloadBalance) FrameID(ID int64) int {
	return int(ID % int64(wb.Stride))
}

func (wb *WorkloadBalance) CheckFrame(f *hermes.FrameReadout) (int, error) {
	if len(f.ProducerUuid) == 0 {
		return -1, fmt.Errorf("Received frame has no ProducerUUID")
	}
	ids, ok := wb.IDsByUUID[f.ProducerUuid]
	if ok == false {
		return -1, fmt.Errorf("Invalid ProducerUUID %s", f.ProducerUuid)
	}
	if wb.Stride == 1 {
		return -1, nil
	}

	fid := wb.FrameID(f.FrameID)
	if ok := ids[fid]; ok == false {
		return -1, fmt.Errorf("Producer %s is not meant to produce frame %d mod [%d]", f.ProducerUuid, fid, wb.Stride)
	}

	time, err := ptypes.Timestamp(f.Time)
	if err != nil {
		return -1, err
	}

	if f.ProducerUuid == wb.MasterUUID {
		if wb.lastPoint == nil {
			wb.lastPoint = &synchronizationPoint{}
		}
		wb.lastPoint.time = time
		wb.lastPoint.timestampUS = f.Timestamp
	} else {
		if wb.lastPoint == nil {
			return -1, fmt.Errorf("Missing a first master frame to compute offset: dropping frame")
		}
		currentOffset := wb.lastPoint.computeOffset(time, f.Timestamp)
		offset, ok := wb.offsets[f.ProducerUuid]
		if ok == false {
			offset = currentOffset
		} else {
			offset += 0.2 * (currentOffset - offset)
		}
		wb.offsets[f.ProducerUuid] = offset
		f.Timestamp += int64(offset)
	}
	return fid, nil
}

type ReadoutBuffer []*hermes.FrameReadout

func (r ReadoutBuffer) Len() int {
	return len(r)
}

func (r ReadoutBuffer) Swap(i, j int) {
	r[j], r[i] = r[i], r[j]
}

func (r ReadoutBuffer) Less(i, j int) bool {
	return r[i].FrameID < r[j].FrameID
}

func MergeFrameReadout(ctx context.Context, wb *WorkloadBalance, inbound <-chan *hermes.FrameReadout, outbound chan<- *hermes.FrameReadout) error {
	defer close(outbound)

	if err := wb.Check(); err != nil {
		return err
	}

	nextFrameToSend := int64(0)
	maxFrame := int64(-1)
	deadlines := map[int64]time.Time{}
	//we reserve a large value, but with tiemout we should have no relocation
	buffer := make(ReadoutBuffer, 0, 10*wb.Stride)
	betweenFrame := time.Duration(1.0e9/wb.FPS) * time.Nanosecond
	timeout := time.Duration(2*wb.Stride+2) * betweenFrame

	logger := tm.NewLogger("frame-merger")
	for {
		var timer *time.Timer = nil
		var timeoutC <-chan time.Time = nil
		if len(deadlines) > 0 {
			timer = time.NewTimer(timeout)
			timeoutC = timer.C
		}
		var now time.Time
		select {
		case <-ctx.Done():
			return nil
		case frame, ok := <-inbound:
			if ok == false {
				return nil
			}
			if frame.FrameID > maxFrame {
				maxFrame = frame.FrameID
			}

			_, err := wb.CheckFrame(frame)
			if err != nil {
				logger.Printf("%s", err)
				continue
			}
			now = time.Now()
			if len(deadlines) == 0 {
				nextFrameToSend = frame.FrameID
				for i := 1; i <= wb.Stride; i++ {
					deadlines[nextFrameToSend+int64(i)] = now.Add(time.Duration(i) * betweenFrame).Add(timeout)
				}
			}
			if frame.FrameID < nextFrameToSend {
				//we already timeouted the frame
				logger.Printf("received frame %d, but already sent a timeout", frame.FrameID)
				continue
			}
			delete(deadlines, frame.FrameID)
			deadlines[frame.FrameID+int64(wb.Stride)] = now.Add(timeout)
			//log.Printf("received now:%s frame:%#v , deadlines:%#v", now, i, deadlines)
			frame.ProducerUuid = ""
			buffer = append(buffer, frame)

		case t := <-timeoutC:
			now = t
		}
		if timer != nil {
			timer.Stop()
		}
		// we complete the buffer with timeouted values
		end := nextFrameToSend + int64(wb.Stride)
		if maxFrame > end {
			end = maxFrame
		}
		for i := nextFrameToSend; i < end; i++ {
			d, ok := deadlines[i]
			//log.Printf("testing %d now:%s deadline:%s ok:%v", i, now, d, ok)
			if ok == true && now.After(d) == true {
				nowPb, _ := ptypes.TimestampProto(now)
				logger.Printf("marking frame %d as timeouted", i)
				ro := &hermes.FrameReadout{
					Error:   hermes.FrameReadout_PROCESS_TIMEOUT,
					FrameID: i,
					Time:    nowPb,
				}
				buffer = append(buffer, ro)
				delete(deadlines, i)
				deadlines[i+int64(wb.Stride)] = now.Add(timeout)
			}
		}
		//we sort them all
		sort.Sort(buffer)

		//send all frames that we have received or timeouted
		for len(buffer) > 0 {
			if buffer[0].FrameID < nextFrameToSend {
				logger.Printf("inconsistent state, next frame is %d, and has %d buffered", nextFrameToSend, buffer[0].FrameID)
				buffer = buffer[1:]
				continue
			}
			if buffer[0].FrameID > nextFrameToSend {
				//we wait for it to arrive or to timeout
				break
			}
			buffer[0].ProducerUuid = ""
			outbound <- buffer[0]
			buffer = buffer[1:]
			delete(deadlines, nextFrameToSend)
			nextFrameToSend++
		}

	}

}
