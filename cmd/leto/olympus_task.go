package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"time"

	olympuspb "github.com/formicidae-tracker/olympus/pkg/api"
	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
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
	address     string
	declaration *olympuspb.TrackingDeclaration

	incoming   chan statusAndAlarm
	connection *olympuspb.TrackingConnection
	logger     *logrus.Entry
	ctx        context.Context
}

func NewOlympusTask(ctx context.Context, env *TrackingEnvironment) (OlympusTask, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	target := env.Config.Stream.Host
	if target == nil || len(*target) == 0 {
		return nil, errors.New("no olympus host in configuraton")
	}

	declaration := &olympuspb.TrackingDeclaration{
		Hostname:       hostname,
		StreamServer:   *target,
		ExperimentName: env.Config.ExperimentName,
		Since:          timestamppb.New(env.Start),
	}
	incoming := make(chan statusAndAlarm, 10)

	return &olympusTask{
		address:     fmt.Sprintf("%s:%d", *target, env.Leto.OlympusPort),
		declaration: declaration,
		incoming:    incoming,
		logger:      tm.NewLogger("olympus-registration"),
		connection:  &olympuspb.TrackingConnection{},
		ctx:         ctx,
	}, nil
}

func (t *olympusTask) PushDiskStatus(status *olympuspb.DiskStatus, update *olympuspb.AlarmUpdate) {
	defer func() {
		err := recover()
		if err != nil {
			t.logger.Printf("push failure: %s", err)
		}
	}()

	t.incoming <- statusAndAlarm{Status: status, Update: update}
}

func (t *olympusTask) Run() error {
	go func() {
		<-t.ctx.Done()
		close(t.incoming)
	}()

	defer t.connection.CloseAll(nil)

	connections, connErrors := t.asyncConnect(nil)

	for {
		if t.connection.Established() == false && connErrors == nil && connections == nil {
			t.connection.CloseAll(nil)
			time.Sleep(time.Duration(float64(1*time.Second) * (1.0 + 0.2*rand.Float64())))
			t.logger.Printf("reconnection")
			connections, connErrors = t.asyncConnect(t.connection.ClienConn())
		}
		select {
		case err, ok := <-connErrors:
			if ok == false {
				connErrors = nil
			} else {
				t.logger.Printf("gRPC connection failure: %s", err)
				t.connection.CloseAll(nil)
			}
		case newConn, ok := <-connections:
			if ok == false {
				connections = nil
			} else {
				t.connection = newConn
			}
		case st, ok := <-t.incoming:
			if ok == false {
				return nil
			}
			err := t.handleStatus(st)
			if err != nil {
				t.logger.Printf("gRPC failure: %s", err)
				t.connection.CloseStream(nil)
			}
		}
	}
}

func (t *olympusTask) asyncConnect(conn *grpc.ClientConn) (<-chan *olympuspb.TrackingConnection, <-chan error) {
	dialOptions := []grpc.DialOption{
		grpc.WithConnectParams(
			grpc.ConnectParams{
				MinConnectTimeout: 20 * time.Second,
				Backoff: backoff.Config{
					BaseDelay:  500 * time.Millisecond,
					Multiplier: backoff.DefaultConfig.Multiplier,
					Jitter:     backoff.DefaultConfig.Jitter,
					MaxDelay:   2 * time.Second,
				},
			}),
	}

	if tm.Enabled() {
		dialOptions = append(dialOptions,
			grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		)
	}

	return olympuspb.ConnectTrackingAsync(conn,
		t.address, t.declaration, nil, dialOptions...)
}

func (t *olympusTask) handleStatus(st statusAndAlarm) error {
	if t.connection == nil {
		return nil
	}
	if st.Status == nil && st.Update == nil {
		return nil
	}

	m := &olympuspb.TrackingUpStream{
		DiskStatus: st.Status,
	}

	if st.Update != nil {
		m.Alarms = []*olympuspb.AlarmUpdate{st.Update}
	}

	_, err := t.connection.Send(m)
	return err
}
