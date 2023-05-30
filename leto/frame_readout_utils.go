package main

import (
	"context"
	"io"

	"github.com/formicidae-tracker/hermes"
)

func ReadAllFrameReadout(ctx context.Context,
	stream io.Reader,
	readouts chan<- *hermes.FrameReadout,
	errors chan<- error) {
	defer func() {
		//Do not close readouts, it is shared by many connections.
		close(errors)
	}()

	for {
		m := &hermes.FrameReadout{}
		ok, err := hermes.ReadDelimitedMessage(stream, m)
		if err != nil {
			if err == io.EOF {
				return
			}
			select {
			case errors <- err:
			case <-ctx.Done():
				return
			}
		}
		if ok == true {
			select {
			case readouts <- m:
			case <-ctx.Done():
				return
			}
		}
	}
}
