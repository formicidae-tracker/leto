package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/formicidae-tracker/leto"
	"github.com/golang/protobuf/proto"
	"github.com/jessevdk/go-flags"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Options struct {
	Version     bool    `long:"version"`
	Video       bool    `long:"video-output-to-stdout"`
	VideoHeight int     `long:"video-output-height"`
	Host        string  `long:"host"`
	Port        int     `long:"port"`
	CameraFps   float64 `long:"camera-fps"`
	Uuid        string  `long:"uuid"`
	Family      string  `long:"at-family"`
}

func main() {
	if err := execute(); err != nil {
		log.Fatalf("unhandled error: %s", err)
	}
}

func execute() error {
	opt := &Options{}
	parser := flags.NewParser(opt, flags.IgnoreUnknown)
	_, err := parser.Parse()
	if err != nil {
		return err
	}

	if opt.Version == true {
		printVersion()
		return nil
	}

	//scaling down the size to avoid large image and transfer, as leto
	// always sends 1080
	opt.VideoHeight /= 4

	log.Printf("%+v", opt)

	if opt.Family != "" {
		return fmt.Errorf("this mock artemis does not support tag detection (received: %s)", opt.Family)
	}

	return opt.Run()
}

func printVersion() {
	fmt.Printf("artemis %s\n", leto.ARTEMIS_MIN_VERSION)
}

func (o *Options) Run() error {
	defer log.Printf("artemis is done")
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)

	ticker := time.NewTicker(o.Period())
	defer ticker.Stop()

	var conn net.Conn
	if o.Host != "" && o.Port != 0 {
		var err error
		conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", o.Host, o.Port))
		if err != nil {
			return err
		}
		defer conn.Close()
	}

	current := 4230

	for {
		select {
		case <-ctx.Done():
			return nil
		case t := <-ticker.C:
			current += 1
			o.GenerateFrame(t, current, conn)
		}
	}

}

func (o *Options) Period() time.Duration {
	return time.Duration(float64(time.Second) / o.CameraFps)
}

func (o *Options) GenerateFrame(t time.Time, frameID int, conn net.Conn) {
	log.Printf("got frame %d", frameID)
	defer log.Printf("frame %d done", frameID)
	if conn != nil {
		o.SendFakeFrame(t, frameID, conn)
	}
	if o.Video == true {
		o.WriteFakeFrame(frameID)
	}
}

func (o *Options) WriteFakeFrame(frameID int) {
	header := make([]byte, 0, 3*8)
	width := int(float64(o.VideoHeight) / 3.0 * 4.0)

	header = binary.LittleEndian.AppendUint64(header, uint64(frameID))
	header = binary.LittleEndian.AppendUint64(header, uint64(width))
	header = binary.LittleEndian.AppendUint64(header, uint64(o.VideoHeight))

	frame := make([]byte, width*o.VideoHeight*3)
	v := uint8(frameID % 10)
	for i := range frame {
		frame[i] = v
	}

	n, err := os.Stdout.Write(header)
	if err != nil {
		log.Printf("could not write frame header to stdout ( %d / %d ): %s",
			n, len(header), err)
	}
	n, err = os.Stdout.Write(frame)
	if err != nil {
		log.Printf("could not write frame data to stdout ( %d / %d ): %s",
			n, len(frame), err)
	}

}

var start = time.Now()

func (o *Options) SendFakeFrame(t time.Time, frameID int, conn net.Conn) {
	buf := proto.NewBuffer(nil)

	message := &hermes.FrameReadout{
		Timestamp:    int64(t.Sub(start).Microseconds()),
		FrameID:      int64(frameID),
		Time:         timestamppb.New(t),
		ProducerUuid: o.Uuid,
		Height:       int32(3 * o.VideoHeight),
		Width:        int32(4 * o.VideoHeight),
	}

	buf.EncodeMessage(message)
	conn.SetWriteDeadline(time.Now().Add(20 * time.Millisecond))
	_, err := conn.Write(buf.Bytes())
	if err != nil {
		log.Printf("connection write error: %s", err)
	}

}
