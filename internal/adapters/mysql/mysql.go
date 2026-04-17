package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

var _ adapters.Adapter = (*adapter)(nil)

func init() {
	adapters.RegisterFactory("mysql", func() adapters.Adapter { return New() })
}

type adapter struct {
	db     *sql.DB
	dsn    string
	dbName string
}

func New() *adapter {
	return &adapter{}
}

func (a *adapter) Connect(config adapters.ConnectionConfig) error {
	dsn, ok := config["dsn"].(string)
	if !ok || dsn == "" {
		return fmt.Errorf("mysql: missing or invalid 'dsn' in config")
	}
	a.dsn = dsn

	// Parse database name from DSN (format: user:pass@tcp(host:port)/dbname?params)
	a.dbName = parseMySQLDBName(dsn)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("mysql: failed to open: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("mysql: failed to ping: %w", err)
	}

	a.db = db
	return nil
}

func (a *adapter) Discover() ([]nodes.Node, []edges.Edge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var allNodes []nodes.Node
	var allEdges []edges.Edge

	// Root node for the database
	rootID := fmt.Sprintf("mysql-%s", a.dbName)
	allNodes = append(allNodes, nodes.Node{
		Id:       rootID,
		Type:     string(nodes.TypeDatabase),
		Name:     a.dbName,
		Metadata: map[string]any{"adapter": "mysql", "database": a.dbName},
		Health:   "healthy",
	})

	// Discover tables
	rows, err := a.db.QueryContext(ctx,
		`SELECT TABLE_NAME FROM information_schema.TABLES
		 WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'`, a.dbName)
	if err != nil {
		return nil, nil, fmt.Errorf("mysql: failed to query tables: %w", err)
	}
	defer rows.Close()

	tableIDs := make(map[string]string)
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, nil, fmt.Errorf("mysql: failed to scan table row: %w", err)
		}
		nodeID := fmt.Sprintf("mysql-%s-%s", a.dbName, tableName)
		tableIDs[tableName] = nodeID

		allNodes = append(allNodes, nodes.Node{
			Id:       nodeID,
			Type:     string(nodes.TypeTable),
			Name:     tableName,
			Parent:   rootID,
			Metadata: map[string]any{"adapter": "mysql", "schema": a.dbName},
			Health:   "healthy",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("mysql: rows error: %w", err)
	}

	// Discover foreign keys
	fkRows, err := a.db.QueryContext(ctx,
		`SELECT CONSTRAINT_NAME, TABLE_NAME, COLUMN_NAME,
		        REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
		 FROM information_schema.KEY_COLUMN_USAGE
		 WHERE TABLE_SCHEMA = ? AND REFERENCED_TABLE_NAME IS NOT NULL`, a.dbName)
	if err != nil {
		return nil, nil, fmt.Errorf("mysql: failed to query foreign keys: %w", err)
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var constraintName, srcTable, srcCol, tgtTable, tgtCol string
		if err := fkRows.Scan(&constraintName, &srcTable, &srcCol, &tgtTable, &tgtCol); err != nil {
			return nil, nil, fmt.Errorf("mysql: failed to scan FK row: %w", err)
		}

		srcID, srcOK := tableIDs[srcTable]
		tgtID, tgtOK := tableIDs[tgtTable]
		if !srcOK || !tgtOK {
			continue
		}

		allEdges = append(allEdges, edges.Edge{
			Id:     fmt.Sprintf("fk-%s", constraintName),
			Source: srcID,
			Target: tgtID,
			Type:   "foreign_key",
			Label:  fmt.Sprintf("%s.%s → %s.%s", srcTable, srcCol, tgtTable, tgtCol),
		})
	}
	if err := fkRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("mysql: FK rows error: %w", err)
	}

	return allNodes, allEdges, nil
}

func (a *adapter) Health() (adapters.HealthMetrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if a.db == nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": "not connected"}, nil
	}

	if err := a.db.PingContext(ctx); err != nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": err.Error()}, nil
	}

	var threadsConnected int
	if err := a.db.QueryRowContext(ctx,
		"SELECT VARIABLE_VALUE FROM performance_schema.global_status WHERE VARIABLE_NAME = 'Threads_connected'",
	).Scan(&threadsConnected); err != nil {
		log.Printf("mysql: failed to query thread count: %v", err)
	}

	stats := a.db.Stats()

	return adapters.HealthMetrics{
		"status":            "healthy",
		"threads_connected": threadsConnected,
		"pool_open":         stats.OpenConnections,
		"pool_in_use":       stats.InUse,
		"pool_idle":         stats.Idle,
	}, nil
}

func (a *adapter) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// parseMySQLDBName extracts the database name from a MySQL DSN.
// Format: user:pass@tcp(host:port)/dbname?params
func parseMySQLDBName(dsn string) string {
	// Find the part after the last /
	if idx := strings.LastIndex(dsn, "/"); idx != -1 {
		dbPart := dsn[idx+1:]
		// Strip query parameters
		if qIdx := strings.Index(dbPart, "?"); qIdx != -1 {
			dbPart = dbPart[:qIdx]
		}
		if dbPart != "" {
			return dbPart
		}
	}
	return "mysql"
}
