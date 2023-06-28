package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"

	"github.com/formicidae-tracker/leto/internal/leto"
	"github.com/formicidae-tracker/leto/pkg/letopb"
	"github.com/grandcat/zeroconf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type LetoGRPCWrapper struct {
	letopb.UnimplementedLetoServer
	leto   *Leto
	logger *log.Logger
}

func (l *LetoGRPCWrapper) StartTracking(c context.Context, request *letopb.StartRequest) (*letopb.Empty, error) {
	config, err := leto.ParseConfiguration([]byte(request.YamlConfiguration))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not parse configuration: %s", err)
	}

	l.logger.Printf("new start request for experiment '%s'", config.ExperimentName)

	err = l.leto.Start(config)
	if err != nil {
		return nil, err
	}
	return &letopb.Empty{}, nil
}

func (l *LetoGRPCWrapper) StopTracking(context.Context, *letopb.Empty) (*letopb.Empty, error) {
	l.logger.Printf("new stop request")
	err := l.leto.Stop()
	if err != nil {
		return nil, err
	}
	return &letopb.Empty{}, nil
}

func (l *LetoGRPCWrapper) GetStatus(context.Context, *letopb.Empty) (*letopb.Status, error) {
	return l.leto.Status(), nil
}

func (l *LetoGRPCWrapper) GetLastExperimentLog(context.Context, *letopb.Empty) (*letopb.ExperimentLog, error) {
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

func (l *LetoGRPCWrapper) Link(c context.Context, link *letopb.TrackingLink) (*letopb.Empty, error) {
	hostname, err := l.checkTrackingLink(link)
	if err != nil {
		return nil, err
	}

	if link.Slave == hostname {
		if err := l.leto.SetMaster(link.Master); err != nil {
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

	err = l.leto.AddSlave(link.Slave)
	if err != nil {
		return nil, err
	}

	return &letopb.Empty{}, nil
}

func (l *LetoGRPCWrapper) Unlink(c context.Context, link *letopb.TrackingLink) (*letopb.Empty, error) {
	hostname, err := l.checkTrackingLink(link)
	if err != nil {
		return nil, err
	}
	if link.Slave == hostname {
		err := l.leto.SetMaster("")
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

	err = l.leto.RemoveSlave(link.Slave)
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

	l.logger = NewLogger("gRPC")

	server := grpc.NewServer()
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
			l.logger.Printf("avahi register error: %s", err)
			return
		}
		<-ctx.Done()
		server.Shutdown()
	}()

	l.logger.Printf("listening on %s", addr)

	return server.Serve(lis)
}
