//go:build integration

package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"binary/internal/adapters"
	"binary/internal/adapters/adaptertest"
)

var (
	testAdapter adapters.Adapter
	testConfig  adapters.ConnectionConfig
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}
	defer container.Terminate(ctx)

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}

	// Seed schema
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open seed connection: %v\n", err)
		os.Exit(1)
	}

	seedSQL := []string{
		`CREATE TABLE authors (
			id SERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL
		)`,
		`CREATE TABLE books (
			id SERIAL PRIMARY KEY,
			title VARCHAR(200) NOT NULL,
			author_id INT REFERENCES authors(id)
		)`,
		`CREATE TABLE reviews (
			id SERIAL PRIMARY KEY,
			book_id INT REFERENCES books(id),
			rating INT NOT NULL
		)`,
	}
	for _, stmt := range seedSQL {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			fmt.Fprintf(os.Stderr, "failed to seed: %v\n", err)
			os.Exit(1)
		}
	}
	pool.Close()

	// Connect the adapter under test
	testConfig = adapters.ConnectionConfig{"dsn": dsn}
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
		MinNodes:       4, // 1 root + 3 tables
		MinEdges:       2, // 2 FK edges
		RootNodeType:   "database",
		ChildNodeTypes: []string{"table"},
		ExpectFKEdges:  true,
		RequiredHealthKeys: []string{
			"status",
			"active_connections",
			"pool_total",
			"pool_idle",
		},
	})
}

func TestDiscover_ForeignKeyLabels(t *testing.T) {
	_, allEdges, err := testAdapter.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	fkCount := 0
	for _, e := range allEdges {
		if e.Type != "foreign_key" {
			continue
		}
		fkCount++
		if !strings.Contains(e.Label, "→") {
			t.Errorf("FK edge %q label missing arrow: %q", e.Id, e.Label)
		}
	}
	if fkCount < 2 {
		t.Errorf("got %d FK edges, want at least 2", fkCount)
	}
}

func TestDiscover_NodeIDFormat(t *testing.T) {
	allNodes, _, err := testAdapter.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	for _, n := range allNodes {
		if !strings.HasPrefix(n.Id, "pg-") {
			t.Errorf("node ID %q does not start with \"pg-\"", n.Id)
		}
	}
}
