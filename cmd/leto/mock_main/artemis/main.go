package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/formicidae-tracker/hermes/src/go/hermes"
	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/golang/protobuf/proto"
	"github.com/jessevdk/go-flags"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Options_0_4 struct {
	Version     bool    `long:"version"`
	Video       bool    `long:"video-output-to-stdout"`
	VideoHeight int     `long:"video-output-height"`
	Host        string  `long:"host"`
	Port        int     `long:"port"`
	CameraFps   float64 `long:"camera-fps"`
	Uuid        string  `long:"uuid"`
	Family      string  `long:"at-family"`
	version     leto.AVersion
}

type Options_0_5 struct {
	Version bool `long:"version"`

	Apriltag struct {
		Family string `long:"family"`
	} `group:"at" namespace:"at"`

	Camera struct {
		FPS float64 `long:"fps"`
	} `group:"camera" namespace:"camera"`

	Leto struct {
		Host string `long:"host"`
		Port int    `long:"port"`
	} `group:"leto" namespace:"leto"`

	VideoOutput struct {
		Height int `long:"height"`
		Stream struct {
			Height  int    `long:"height"`
			Address string `long:"address"`
		} `group:"stream" namespace:"stream"`
	} `group:"video-output" namespace:"video-output"`

	Process struct {
		Uuid string `long:"uuid"`
	} `group:"process" namespace:"process"`
}

func main() {
	if err := execute(); err != nil {
		slog.With("error", err).Error("unhandled error")
		os.Exit(1)
	}
}

func parseOptions() (*Options_0_4, error) {
	version := os.Getenv("MOCK_ARTEMIS_VERSION")
	if version == "" {
		return nil, fmt.Errorf("Must set the env variable MOCK_ARTEMIS_VERSION for parameters")
	}
	switch version {
	case "0.4":
		return parseOptions_0_4()
	case "0.5":
		return parseOptions_0_5()
	default:
		return nil, fmt.Errorf("Unsupported mocked version %s", version)
	}

}

func parseOptions_0_4() (*Options_0_4, error) {
	opts := &Options_0_4{version: leto.ARTEMIS_0_4}
	parser := flags.NewParser(opts, flags.IgnoreUnknown|flags.PrintErrors|flags.HelpFlag)
	_, err := parser.Parse()
	return opts, err
}

func parseOptions_0_5() (*Options_0_4, error) {
	opts := &Options_0_5{}
	parser := flags.NewParser(opts, flags.IgnoreUnknown|flags.PrintErrors|flags.HelpFlag)
	_, err := parser.Parse()

	return &Options_0_4{
		version:     leto.ARTEMIS_0_5,
		Version:     opts.Version,
		Video:       false, // version 0.5 never send videoframe to stdout
		VideoHeight: opts.VideoOutput.Height,
		Host:        opts.Leto.Host,
		Port:        opts.Leto.Port,
		CameraFps:   opts.Camera.FPS,
		Uuid:        opts.Process.Uuid,
		Family:      opts.Apriltag.Family,
	}, err
}

func execute() error {
	opts, err := parseOptions()
	if flags.WroteHelp(err) == true {
		return nil
	}
	if err != nil {
		return err
	}

	if opts.Version == true {
		opts.printVersion()
		return nil
	}

	//scaling down the size to avoid large image and transfer, as leto
	// always sends 1080
	opts.VideoHeight /= 4

	slog.With("options", opts).Info("starting")

	if opts.Family != "" {
		return fmt.Errorf("this mock artemis does not support tag detection (received: %s)", opts.Family)
	}

	return opts.Run()
}

func (o *Options_0_4) printVersion() {
	fmt.Printf("artemis %s\n", o.version)
}

func (o *Options_0_4) Run() error {
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

func (o *Options_0_4) Period() time.Duration {
	return time.Duration(float64(time.Second) / o.CameraFps)
}

func (o *Options_0_4) GenerateFrame(t time.Time, frameID int, conn net.Conn) {
	log.Printf("got frame %d", frameID)
	defer log.Printf("frame %d done", frameID)
	if conn != nil {
		o.SendFakeFrame(t, frameID, conn)
	}
	if o.Video == true {
		o.WriteFakeFrame(frameID)
	}
}

func (o *Options_0_4) WriteFakeFrame(frameID int) {
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

func (o *Options_0_4) SendFakeFrame(t time.Time, frameID int, conn net.Conn) {
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
