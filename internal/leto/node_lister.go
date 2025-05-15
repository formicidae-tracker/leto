package leto

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/formicidae-tracker/leto/pkg/letopb"
	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/hashicorp/mdns"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v2"
)

type NodeLister struct {
	CacheDate time.Time       `yaml:"date"`
	Cache     map[string]Node `yaml:"nodes"`
}

type Node struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

func (n Node) DialAddress() string {
	return fmt.Sprintf("%s:%d", n.Address, n.Port)
}

func (n Node) Connect() (*grpc.ClientConn, letopb.LetoClient, error) {
	options := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(
			grpc.ConnectParams{
				MinConnectTimeout: 2 * time.Second,
				Backoff: backoff.Config{
					BaseDelay:  10 * time.Millisecond,
					Multiplier: backoff.DefaultConfig.Multiplier,
					Jitter:     backoff.DefaultConfig.Jitter,
					MaxDelay:   200 * time.Millisecond,
				},
			}),
	}

	if tm.Enabled() {
		options = append(options,
			grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		)
	}

	conn, err := grpc.Dial(
		n.DialAddress(),
		options...,
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, letopb.NewLetoClient(conn), nil
}

func closeAndLogError(c io.Closer) {
	err := c.Close()
	if err == nil {
		return
	}
	log.Printf("gRPC close() failure: %s", err)
}

func (n Node) Link(link *letopb.TrackingLink) error {
	conn, client, err := n.Connect()
	if err != nil {
		return err
	}
	defer closeAndLogError(conn)
	_, err = client.Link(context.Background(), link)
	return err
}

func (n Node) Unlink(link *letopb.TrackingLink) error {
	conn, client, err := n.Connect()
	if err != nil {
		return err
	}
	defer closeAndLogError(conn)
	_, err = client.Unlink(context.Background(), link)
	return err
}

func (n Node) StartTracking(request *letopb.StartRequest) error {
	conn, client, err := n.Connect()
	if err != nil {
		return err
	}
	defer closeAndLogError(conn)
	_, err = client.StartTracking(context.Background(), request)
	return err
}

func (n Node) StopTracking() error {
	conn, client, err := n.Connect()
	if err != nil {
		return err
	}
	defer closeAndLogError(conn)
	_, err = client.StopTracking(context.Background(), &letopb.Empty{})
	return err
}

func (n Node) GetStatus() (*letopb.Status, error) {
	conn, client, err := n.Connect()
	if err != nil {
		return nil, err
	}
	defer closeAndLogError(conn)
	return client.GetStatus(context.Background(), &letopb.Empty{})
}

func (n Node) GetLastExperimentLog() (*letopb.ExperimentLog, error) {
	conn, client, err := n.Connect()
	if err != nil {
		return nil, err
	}
	defer closeAndLogError(conn)
	return client.GetLastExperimentLog(context.Background(), &letopb.Empty{})
}

func NewNodeLister() *NodeLister {
	res := &NodeLister{}
	res.load()
	return res
}

func (n *NodeLister) cacheFilePath() string {
	return filepath.Join(xdg.CacheHome, "fort/leto/node.cache")
}

func (n *NodeLister) load() {
	cachedData, err := ioutil.ReadFile(n.cacheFilePath())
	if err != nil {
		return
	}
	err = yaml.Unmarshal(cachedData, n)
	if err != nil {
		n.CacheDate = time.Now().Add(-10 * time.Hour)
	}
}

func (n *NodeLister) save() {
	if err := os.MkdirAll(filepath.Dir(n.cacheFilePath()), 0755); err != nil {
		return
	}
	yamlData, err := yaml.Marshal(n)
	if err != nil {
		return
	}

	os.WriteFile(n.cacheFilePath(), yamlData, 0644)
}

func mdnsLookupContext(ctx context.Context, service string, entries chan<- *mdns.ServiceEntry) error {
	params := mdns.DefaultParams(service)
	params.Entries = entries
	return mdns.QueryContext(ctx, params)
}

func (n *NodeLister) ListNodes() (map[string]Node, error) {
	if time.Now().Before(n.CacheDate.Add(NODE_CACHE_TTL)) == true {
		return n.Cache, nil
	}

	entries := make(chan *mdns.ServiceEntry, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go func() {
		mdnsLookupContext(ctx, "_leto._tcp", entries)
		close(entries)
	}()

	res := make(map[string]Node)

	for e := range entries {
		name := strings.TrimPrefix(e.Name, "leto.")
		address := strings.TrimSuffix(e.Host, ".")
		port := e.Port
		res[name] = Node{Name: name, Address: address, Port: port}
	}
	n.Cache = res
	n.CacheDate = time.Now()

	n.save()

	return res, nil
}
