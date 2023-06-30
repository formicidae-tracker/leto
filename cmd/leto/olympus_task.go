package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	olympuspb "github.com/formicidae-tracker/olympus/pkg/api"
	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:generate mockgen -source=olympus_task.go -aux_files github.com/formicidae-tracker/leto/cmd/leto=task.go -destination=mock_main/olympus_task.go

type OlympusTask interface {
	Task
	PushDiskStatus(*olympuspb.DiskStatus, *olympuspb.AlarmUpdate)
}

type statusAndAlarm struct {
	Status *olympuspb.DiskStatus
	Update *olympuspb.AlarmUpdate
}

type olympusTask struct {
	*olympuspb.ClientTask[olympuspb.TrackingUpStream, olympuspb.TrackingDownStream]

	incoming chan statusAndAlarm
	logger   *logrus.Entry
}

func NewOlympusTask(ctx context.Context, env *TrackingEnvironment) (OlympusTask, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	target := env.Config.Stream.Host
	if target == nil || len(*target) == 0 {
		return nil, errors.New("no olympus host in configuration")
	}

	declaration := &olympuspb.TrackingDeclaration{
		Hostname:       hostname,
		StreamServer:   *target,
		ExperimentName: env.Config.ExperimentName,
		Since:          timestamppb.New(env.Start),
	}
	incoming := make(chan statusAndAlarm, 10)

	var options []grpc.DialOption
	if tm.Enabled() {
		options = append(options,
			grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		)
	}

	address := fmt.Sprintf("%s:%d", *target, env.Leto.OlympusPort)
	return &olympusTask{
		ClientTask: olympuspb.NewTrackingTask(
			ctx, address, declaration, options...),
		incoming: incoming,
		logger:   tm.NewLogger("olympus-registration"),
	}, nil
}

func (t *olympusTask) PushDiskStatus(status *olympuspb.DiskStatus, update *olympuspb.AlarmUpdate) {
	if status == nil && update == nil {
		return
	}
	var updates []*olympuspb.AlarmUpdate
	if update != nil {
		updates = append(updates, update)
	}

	response := t.ClientTask.Request(&olympuspb.TrackingUpStream{
		DiskStatus: status,
		Alarms:     updates,
	})

	go func() {
		res := <-response
		if res.Error != nil {
			t.logger.WithError(res.Error).Error("could not push update to olympus")
		}
	}()
}
