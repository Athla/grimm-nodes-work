//go:build integration

package redis_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.uber.org/zap"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
	"github.com/guilherme-grimm/graph-go/internal/adapters/adaptertest"
	redisadapter "github.com/guilherme-grimm/graph-go/internal/adapters/redis"
)

var (
	testAdapter adapters.Adapter
	testConfig  adapters.ConnectionConfig
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start redis container: %v\n", err)
		os.Exit(1)
	}
	defer container.Terminate(ctx)

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container host: %v\n", err)
		os.Exit(1)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container port: %v\n", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", host, port.Port())

	// Seed db0 with 3 keys
	client0 := goredis.NewClient(&goredis.Options{Addr: addr, DB: 0})
	for i := range 3 {
		if err := client0.Set(ctx, fmt.Sprintf("key%d", i), "value", 0).Err(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to seed db0: %v\n", err)
			os.Exit(1)
		}
	}
	client0.Close()

	// Seed db1 with 2 keys
	client1 := goredis.NewClient(&goredis.Options{Addr: addr, DB: 1})
	for i := range 2 {
		if err := client1.Set(ctx, fmt.Sprintf("key%d", i), "value", 0).Err(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to seed db1: %v\n", err)
			os.Exit(1)
		}
	}
	client1.Close()

	// Connect the adapter under test using host/port config path
	testConfig = adapters.ConnectionConfig{
		"host": host,
		"port": uint16(port.Int()),
	}

	testAdapter = redisadapter.New(zap.NewNop().Sugar())
	if err := testAdapter.Connect(testConfig); err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect adapter: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	testAdapter.Close()
	os.Exit(code)
}

func TestContract(t *testing.T) {
	adaptertest.RunContractTests(t, testAdapter, func() adapters.Adapter { return redisadapter.New(zap.NewNop().Sugar()) }, testConfig, adaptertest.ContractOpts{
		MinNodes:       3, // 1 root + 2 keyspaces
		MinEdges:       2, // 2 contains
		RootNodeType:   "database",
		ChildNodeTypes: []string{"database"},
		RequiredHealthKeys: []string{
			"status",
			"version",
			"used_memory",
			"connected_clients",
			"uptime_seconds",
		},
	})
}

func TestConnect_URI(t *testing.T) {
	host, _ := testConfig["host"].(string)
	port, _ := testConfig["port"].(uint16)
	uri := fmt.Sprintf("redis://%s:%d", host, port)

	a := redisadapter.New(zap.NewNop().Sugar())
	if err := a.Connect(adapters.ConnectionConfig{"uri": uri}); err != nil {
		t.Fatalf("Connect with URI failed: %v", err)
	}
	defer a.Close()

	_, err := a.Health()
	if err != nil {
		t.Errorf("Health after URI connect returned error: %v", err)
	}
}

func TestDiscover_KeyspaceMetadata(t *testing.T) {
	nodes, _, err := testAdapter.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	for _, n := range nodes {
		if n.Parent == "" {
			continue // skip root
		}
		keys, ok := n.Metadata["keys"]
		if !ok {
			t.Errorf("keyspace node %q missing \"keys\" in Metadata", n.Id)
		}
		if keys == 0 {
			t.Errorf("keyspace node %q has 0 keys, expected seeded data", n.Id)
		}
		if _, ok := n.Metadata["expires"]; !ok {
			t.Errorf("keyspace node %q missing \"expires\" in Metadata", n.Id)
		}
	}
}
