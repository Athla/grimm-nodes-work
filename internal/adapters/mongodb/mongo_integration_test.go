//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
	"github.com/guilherme-grimm/graph-go/internal/adapters/adaptertest"
)

var (
	testAdapter adapters.Adapter
	testConfig  adapters.ConnectionConfig
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := tcmongo.Run(ctx, "mongo:7")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start mongodb container: %v\n", err)
		os.Exit(1)
	}
	defer container.Terminate(ctx)

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}

	// Seed data: create 2 databases with collections
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect seed client: %v\n", err)
		os.Exit(1)
	}

	// Database "appdb" with 2 collections
	appDB := client.Database("appdb")
	if _, err := appDB.Collection("users").InsertOne(ctx, bson.M{"name": "test"}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to seed appdb.users: %v\n", err)
		os.Exit(1)
	}
	if _, err := appDB.Collection("orders").InsertOne(ctx, bson.M{"item": "test"}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to seed appdb.orders: %v\n", err)
		os.Exit(1)
	}

	// Database "analytics" with 1 collection
	analyticsDB := client.Database("analytics")
	if _, err := analyticsDB.Collection("events").InsertOne(ctx, bson.M{"type": "click"}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to seed analytics.events: %v\n", err)
		os.Exit(1)
	}

	client.Disconnect(ctx)

	// Connect the adapter under test
	testConfig = adapters.ConnectionConfig{"uri": uri}
	testAdapter = New(zap.NewNop().Sugar())
	if err := testAdapter.Connect(testConfig); err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect adapter: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	testAdapter.Close()
	os.Exit(code)
}

func TestContract(t *testing.T) {
	adaptertest.RunContractTests(t, testAdapter, func() adapters.Adapter { return New() }, testConfig, adaptertest.ContractOpts{
		MinNodes:       5, // 2 db roots + 3 collections
		MinEdges:       3, // 3 contains
		RootNodeType:   "database",
		ChildNodeTypes: []string{"collection"},
		MultiRoot:      true,
		RequiredHealthKeys: []string{
			"status",
			"database_count",
		},
	})
}

func TestDiscover_FiltersSystemDBs(t *testing.T) {
	allNodes, _, err := testAdapter.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	for _, n := range allNodes {
		if systemDBs[n.Name] {
			t.Errorf("system database %q should have been filtered out", n.Name)
		}
	}
}

func TestHealth_DatabaseCount(t *testing.T) {
	metrics, err := testAdapter.Health()
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	count, ok := metrics["database_count"].(int)
	if !ok {
		t.Fatal("database_count missing or not an int")
	}
	if count < 2 {
		t.Errorf("database_count = %d, want at least 2 (appdb + analytics)", count)
	}
}
