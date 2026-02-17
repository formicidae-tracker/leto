package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"os/signal"

	"github.com/formicidae-tracker/hermes/src/go/hermes"
	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/jessevdk/go-flags"
)

func main() {
	if err := execute(); err != nil {
		slog.Error("unhandled error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

type Options struct {
	Port int     `short:"p" long:"port" default:"4001" description:"port to listen to"`
	FPS  float64 `short:"f" long:"FPS" default:"8.0" description:"FPS for incoming frame"`
	Args struct {
		UUID string `positional-arg-name:"UUID" required:"true" description:"UUID to receive from"`
	} `positional-args:"true"`
}

func displayIncomingSummary(frames <-chan *hermes.FrameReadout) {
	ticker := time.NewTicker(1 * time.Second)
	last := time.Time{}
	count := 0
	first := time.Now()
	for {
		select {
		case t := <-ticker.C:
			fps := math.NaN()
			if count > 2 {
				fps = float64(count) / t.Sub(first).Seconds()
			}
			fmt.Printf("\033[Kreceived: %d FPS:%.2f last: %s\n\033[F", count, fps, last)
			break
		case f, ok := <-frames:
			if ok == false {
				return
			}
			t := f.Time.AsTime()
			if t.After(last) {
				last = t
			}
			if count == 0 {
				first = time.Now()
			}
			count += 1
			break
		}
	}
}

func execute() error {
	var opts Options
	_, err := flags.Parse(&opts)
	if err != nil {
		if flags.WroteHelp(err) == true {
			return nil
		}
		return fmt.Errorf("could not parse options: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

	listener, err := leto.NewArtemisListener(ctx, opts.Port)
	if err != nil {
		return fmt.Errorf("could not create listener: %w", err)
	}

	listenerErrors := leto.StartTask(listener)

	readouts := make(chan *hermes.FrameReadout, 40)

	wb := &leto.WorkloadBalance{
		FPS:        opts.FPS,
		Stride:     1,
		MasterUUID: opts.Args.UUID,
		IDsByUUID:  map[string][]bool{opts.Args.UUID: []bool{true}},
	}

	mergeErrors := leto.StartTaskFunc(func() error {
		return leto.MergeFrameReadout(ctx, wb, listener.Outbound(), readouts)
	})

	go displayIncomingSummary(readouts)
	var ret error = nil
	for {
		select {
		case err, ok := <-listenerErrors:
			if ok {
				if err != nil {
					slog.Error("listen",
						slog.String("error", err.Error()))
					cancel()
					ret = errors.Join(ret, err)
				}
			} else {
				listenerErrors = nil
			}
		case err, ok := <-mergeErrors:
			if ok {
				if err != nil {
					slog.Error("merge",
						slog.String("error", err.Error()))
					cancel()
					ret = errors.Join(ret, err)
				}
			} else {
				mergeErrors = nil
			}
		}
		if mergeErrors == nil && listenerErrors == nil {
			break
		}
	}

	return ret
}
