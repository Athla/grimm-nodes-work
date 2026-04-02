//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go/modules/mysql"

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

	container, err := mysql.Run(ctx, "mysql:8.0",
		mysql.WithDatabase("testdb"),
		mysql.WithUsername("root"),
		mysql.WithPassword("testpass"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start mysql container: %v\n", err)
		os.Exit(1)
	}
	defer container.Terminate(ctx)

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container host: %v\n", err)
		os.Exit(1)
	}
	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container port: %v\n", err)
		os.Exit(1)
	}

	dsn := fmt.Sprintf("root:testpass@tcp(%s:%s)/testdb", host, port.Port())

	// Seed schema
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open seed connection: %v\n", err)
		os.Exit(1)
	}

	seedSQL := []string{
		`CREATE TABLE authors (
			id INT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(100) NOT NULL
		)`,
		`CREATE TABLE books (
			id INT PRIMARY KEY AUTO_INCREMENT,
			title VARCHAR(200) NOT NULL,
			author_id INT,
			FOREIGN KEY (author_id) REFERENCES authors(id)
		)`,
		`CREATE TABLE reviews (
			id INT PRIMARY KEY AUTO_INCREMENT,
			book_id INT,
			rating INT NOT NULL,
			FOREIGN KEY (book_id) REFERENCES books(id)
		)`,
	}
	for _, stmt := range seedSQL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			fmt.Fprintf(os.Stderr, "failed to seed: %v\n", err)
			os.Exit(1)
		}
	}
	db.Close()

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
			"threads_connected",
			"pool_open",
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
		// Label should contain "table.column → table.column"
		if !strings.Contains(e.Label, "→") {
			t.Errorf("FK edge %q label missing arrow: %q", e.Id, e.Label)
		}
		parts := strings.Split(e.Label, "→")
		if len(parts) != 2 {
			t.Errorf("FK edge %q label has unexpected format: %q", e.Id, e.Label)
			continue
		}
		src := strings.TrimSpace(parts[0])
		tgt := strings.TrimSpace(parts[1])
		if !strings.Contains(src, ".") || !strings.Contains(tgt, ".") {
			t.Errorf("FK edge %q label parts should be table.column: %q", e.Id, e.Label)
		}
	}
	if fkCount == 0 {
		t.Fatal("expected at least one foreign_key edge")
	}
}

func TestDiscover_NodeIDFormat(t *testing.T) {
	allNodes, _, err := testAdapter.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	for _, n := range allNodes {
		if !strings.HasPrefix(n.Id, "mysql-") {
			t.Errorf("node ID %q does not start with \"mysql-\"", n.Id)
		}
		if n.Parent != "" && !strings.HasPrefix(n.Id, "mysql-testdb-") {
			t.Errorf("child node ID %q does not follow mysql-{db}-{table} pattern", n.Id)
		}
	}
}
