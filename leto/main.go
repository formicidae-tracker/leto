package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"

	"github.com/formicidae-tracker/leto"
	"github.com/formicidae-tracker/leto/letopb"
	"github.com/grandcat/zeroconf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Leto struct {
	letopb.UnimplementedLetoServer
	artemis *ArtemisManager
	logger  *log.Logger
}

func (l *Leto) StartTracking(c context.Context, request *letopb.StartRequest) (*letopb.Empty, error) {
	config, err := leto.ParseConfiguration([]byte(request.YamlConfiguration))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not parse configuration: %s", err)
	}

	l.logger.Printf("new start request for experiment '%s'", config.ExperimentName)

	err = l.artemis.Start(config)
	if err != nil {
		return nil, err
	}
	return &letopb.Empty{}, nil
}

func (l *Leto) StopTracking(context.Context, *letopb.Empty) (*letopb.Empty, error) {
	l.logger.Printf("new stop request")
	err := l.artemis.Stop()
	if err != nil {
		return nil, err
	}
	return &letopb.Empty{}, nil
}

func (l *Leto) GetStatus(context.Context, *letopb.Empty) (*letopb.Status, error) {
	return l.artemis.Status(), nil
}

func (l *Leto) GetLastExperimentLog(context.Context, *letopb.Empty) (*letopb.ExperimentLog, error) {
	return nil, fmt.Errorf("Not yet implemented")
}

func (l *Leto) checkTrackingLink(link *letopb.TrackingLink) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", status.Errorf(codes.Unavailable, "could not found hostname: %s", err)
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

func (l *Leto) getSlave(name string) (leto.Node, error) {
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

func (l *Leto) Link(c context.Context, link *letopb.TrackingLink) (*letopb.Empty, error) {
	hostname, err := l.checkTrackingLink(link)
	if err != nil {
		return nil, err
	}

	if link.Slave == hostname {
		if err := l.artemis.SetMaster(link.Master); err != nil {
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

	err = l.artemis.AddSlave(link.Slave)
	if err != nil {
		return nil, err
	}
	return &letopb.Empty{}, nil
}

func (l *Leto) Unlink(c context.Context, link *letopb.TrackingLink) (*letopb.Empty, error) {
	hostname, err := l.checkTrackingLink(link)
	if err != nil {
		return nil, err
	}
	if link.Slave == hostname {
		err := l.artemis.SetMaster("")
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

	err = l.artemis.RemoveSlave(link.Slave)
	if err != nil {
		return nil, err
	}
	return &letopb.Empty{}, nil
}

func Execute() error {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("leto %s\n", leto.LETO_VERSION)
		return nil
	}

	host, err := os.Hostname()
	if err != nil {
		return err
	}

	l := &Leto{}
	l.artemis, err = NewArtemisManager()
	if err != nil {
		return err
	}

	l.artemis.LoadFromPersistentFile()

	l.logger = log.New(os.Stderr, "[gRPC] ", 0)

	addr := fmt.Sprintf(":%d", leto.LETO_PORT)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	letopb.RegisterLetoServer(server, l)

	idleConnections := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		server.GracefulStop()
		close(idleConnections)
	}()

	go func() {
		server, err := zeroconf.Register("leto."+host, "_leto._tcp", "local.", leto.LETO_PORT, nil, nil)
		if err != nil {
			log.Printf("[avahi] register error: %s", err)
			return
		}
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		server.Shutdown()
	}()

	l.logger.Printf("listening on %s", addr)
	if err := server.Serve(lis); err != nil {
		return err
	}

	<-idleConnections

	return nil
}

func main() {
	if err := Execute(); err != nil {
		log.Fatalf("Unhandled error: %s", err)
	}
}
