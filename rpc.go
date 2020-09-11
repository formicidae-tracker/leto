package leto

import (
	"errors"
	"time"
)

type Response struct {
	Error string
}

type Status struct {
	Running        bool
	Since          time.Time
	ExperimentName string
	Configuration  TrackingConfiguration
}

func (r Response) ToError() error {
	if len(r.Error) == 0 {
		return nil
	}
	return errors.New(r.Error)
}

type SlaveTrackingStart struct {
	Stride int
	IDs    []int
	Remote string
	UUID   string
}

type TrackingStop struct {
}

type Link struct {
	Master string
	Slave  string
}

type Unlink struct {
	Master string
	Slave  string
}
