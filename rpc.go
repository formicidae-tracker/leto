package leto

import (
	"errors"
	"time"
)

//go:generate protoc --experimental_allow_proto3_optional --go_out=letopb --go-grpc_out=letopb letopb/leto_service.proto

type Response struct {
	Error string
}

type NoArgs struct {
}

type Status struct {
	Master     string
	Slaves     []string
	Experiment *ExperimentStatus
}

type ExperimentStatus struct {
	Since             time.Time
	ExperimentDir     string
	YamlConfiguration string
}

type ExperimentLog struct {
	Log               string
	Stderr            string
	ExperimentDir     string
	Start, End        time.Time
	YamlConfiguration string
	HasError          bool
}

func (r Response) ToError() error {
	if len(r.Error) == 0 {
		return nil
	}
	return errors.New(r.Error)
}

// type SlaveTrackingStart struct {
// 	Stride int
// 	IDs    []int
// 	Remote string
// 	UUID   string
// }

type Link struct {
	Master string
	Slave  string
}

type RegisterTrackerArgs struct {
	Hostname       string
	StreamServer   string
	ExperimentName string
}
