package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"

	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/leto/pkg/letopb"
	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/grandcat/zeroconf"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type LetoGRPCWrapper struct {
	letopb.UnimplementedLetoServer
	leto   *Leto
	logger *logrus.Entry
}

func (l *LetoGRPCWrapper) StartTracking(ctx context.Context, request *letopb.StartRequest) (*letopb.Empty, error) {
	config, err := leto.ParseConfiguration([]byte(request.YamlConfiguration))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not parse configuration: %s", err)
	}

	l.logger.WithField("experiment", config.ExperimentName).Info("new start request")

	err = l.leto.Start(ctx, config)
	if err != nil {
		return nil, err
	}
	return &letopb.Empty{}, nil
}

func (l *LetoGRPCWrapper) StopTracking(ctx context.Context, _ *letopb.Empty) (*letopb.Empty, error) {
	l.logger.Infof("new stop request")
	err := l.leto.Stop(ctx)
	if err != nil {
		return nil, err
	}
	return &letopb.Empty{}, nil
}

func (l *LetoGRPCWrapper) GetStatus(ctx context.Context, _ *letopb.Empty) (*letopb.Status, error) {
	l.logger.Trace("get status")
	return l.leto.Status(ctx), nil
}

func (l *LetoGRPCWrapper) GetLastExperimentLog(context.Context, *letopb.Empty) (*letopb.ExperimentLog, error) {
	l.logger.Trace("get last experiment log")

	last := l.leto.LastExperimentLog()
	if last == nil {
		return nil, status.Error(codes.FailedPrecondition, "no experiment run on node.")
	}
	return last, nil
}

func (l *LetoGRPCWrapper) checkTrackingLink(link *letopb.TrackingLink) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", status.Errorf(codes.Unavailable, "could not get hostname: %s", err)
	}

	if link.Master != hostname && link.Slave != hostname {
		return "", status.Errorf(codes.InvalidArgument,
			"current hostname (%s) is neither the link.Master:%s or link.Slave:%s",
			hostname,
			link.Master,
			link.Slave)
	}
	return hostname, err
}

func (l *LetoGRPCWrapper) getSlave(name string) (leto.Node, error) {
	nodes, err := leto.NewNodeLister().ListNodes()
	if err != nil {
		return leto.Node{}, status.Errorf(codes.Unavailable, "could not list local nodes: %s", err)
	}
	slave, ok := nodes[name]
	if ok == false {
		return leto.Node{}, status.Errorf(codes.Unavailable, "could not find slave '%s'", name)
	}
	return slave, nil
}

func (l *LetoGRPCWrapper) Link(ctx context.Context, link *letopb.TrackingLink) (*letopb.Empty, error) {
	hostname, err := l.checkTrackingLink(link)
	if err != nil {
		return nil, err
	}

	if link.Slave == hostname {
		if err := l.leto.SetMaster(ctx, link.Master); err != nil {
			return nil, err
		}
		return &letopb.Empty{}, nil
	}

	slave, err := l.getSlave(link.Slave)
	if err != nil {
		return nil, err
	}

	err = slave.Link(link)
	if err != nil {
		return nil, err
	}

	err = l.leto.AddSlave(ctx, link.Slave)
	if err != nil {
		return nil, err
	}

	return &letopb.Empty{}, nil
}

func (l *LetoGRPCWrapper) Unlink(ctx context.Context, link *letopb.TrackingLink) (*letopb.Empty, error) {
	hostname, err := l.checkTrackingLink(link)
	if err != nil {
		return nil, err
	}
	if link.Slave == hostname {
		err := l.leto.SetMaster(ctx, "")
		if err != nil {
			return nil, err
		}
		return &letopb.Empty{}, nil
	}
	slave, err := l.getSlave(link.Slave)
	if err != nil {
		return nil, err
	}

	err = slave.Unlink(link)
	if err != nil {
		return nil, err
	}

	err = l.leto.RemoveSlave(ctx, link.Slave)
	if err != nil {
		return nil, err
	}
	return &letopb.Empty{}, nil
}

func (l *LetoGRPCWrapper) Run(config leto.Config) error {
	host, err := os.Hostname()
	if err != nil {
		return err
	}

	l.leto, err = NewLeto(config)
	if err != nil {
		return err
	}

	l.logger = tm.NewLogger("gRPC")

	options := make([]grpc.ServerOption, 0, 2)
	if tm.Enabled() {
		options = append(options,
			grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
			grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
		)
	}

	server := grpc.NewServer(options...)
	letopb.RegisterLetoServer(server, l)

	addr := fmt.Sprintf(":%d", config.LetoPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	idleConnections := make(chan struct{})
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)

	go func() {
		<-ctx.Done()
		server.GracefulStop()
		close(idleConnections)
	}()

	defer func() { <-idleConnections }()

	go func() {
		server, err := zeroconf.Register("leto."+host, "_leto._tcp", "local.", config.LetoPort, nil, nil)
		if err != nil {
			l.logger.WithError(err).Error("avahi register")
			return
		}
		<-ctx.Done()
		server.Shutdown()
	}()

	l.logger.WithField("address", addr).Info("listening")

	return server.Serve(lis)
}
