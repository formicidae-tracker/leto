package leto

import (
	"errors"
	"time"
)

type Response struct {
	Error string
}

type NoArgs struct {
}

type Status struct {
	Since         time.Time
	ExperimentDir string
	Configuration TrackingConfiguration
}

type LastExperimentStatus struct {
	Log           []byte
	ExperimentDir string
	Start, End    time.Time
	Config        TrackingConfiguration
	HasError      bool
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

type Link struct {
	Master string
	Slave  string
}

type Unlink struct {
	Master string
	Slave  string
}
