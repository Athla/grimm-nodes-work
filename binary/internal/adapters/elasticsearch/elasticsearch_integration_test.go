//go:build integration

package elasticsearch

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"
	tcelastic "github.com/testcontainers/testcontainers-go/modules/elasticsearch"

	"binary/internal/adapters"
	"binary/internal/adapters/adaptertest"
)

var (
	testAdapter adapters.Adapter
	testConfig  adapters.ConnectionConfig
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	container, err := tcelastic.Run(ctx, "elasticsearch:8.12.0",
		tcelastic.WithPassword(""),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start elasticsearch container: %v\n", err)
		os.Exit(1)
	}
	defer container.Terminate(ctx)

	endpoint := container.Settings.Address

	// Seed indices using the ES client directly
	client, err := es.NewClient(
		es.Config{Addresses: []string{endpoint}},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create seed client: %v\n", err)
		os.Exit(1)
	}

	indices := []string{"test-index-1", "test-index-2", "test-index-3"}
	for _, idx := range indices {
		res, err := client.Indices.Create(idx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create index %s: %v\n", idx, err)
			os.Exit(1)
		}
		res.Body.Close()

		res, err = client.Index(idx, strings.NewReader(`{"field":"value"}`))
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to index document in %s: %v\n", idx, err)
			os.Exit(1)
		}
		res.Body.Close()
	}

	// Refresh to make documents searchable
	res, err := client.Indices.Refresh()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to refresh indices: %v\n", err)
		os.Exit(1)
	}
	res.Body.Close()

	// Connect the adapter under test
	testConfig = adapters.ConnectionConfig{"endpoint": endpoint}
	testAdapter = New()
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
		MinNodes:       4, // 1 root + 3 indices
		MinEdges:       3, // 3 contains
		RootNodeType:   "elasticsearch",
		ChildNodeTypes: []string{"index"},
		RequiredHealthKeys: []string{
			"status",
			"cluster_name",
			"cluster_status",
			"number_of_nodes",
			"active_shards",
		},
	})
}

func TestDiscover_SkipsSystemIndices(t *testing.T) {
	nodes, _, err := testAdapter.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	for _, n := range nodes {
		if n.Parent != "" && strings.HasPrefix(n.Name, ".") {
			t.Errorf("system index %q should have been filtered out", n.Name)
		}
	}
}

func TestHealth_ClusterMetrics(t *testing.T) {
	metrics, err := testAdapter.Health()
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	numNodes, ok := metrics["number_of_nodes"].(int)
	if !ok {
		t.Fatal("number_of_nodes missing or not an int")
	}
	if numNodes != 1 {
		t.Errorf("number_of_nodes = %d, want 1 (single-node cluster)", numNodes)
	}

	clusterStatus, ok := metrics["cluster_status"].(string)
	if !ok {
		t.Fatal("cluster_status missing or not a string")
	}
	if clusterStatus != "green" && clusterStatus != "yellow" {
		t.Errorf("cluster_status = %q, want green or yellow", clusterStatus)
	}
}
