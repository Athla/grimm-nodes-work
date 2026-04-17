package postgres

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

var _ adapters.Adapter = (*adapter)(nil)

func init() {
	adapters.RegisterFactory("postgres", func(l *zap.SugaredLogger) adapters.Adapter { return New(l) })
}

type adapter struct {
	pool   *pgxpool.Pool
	dsn    string
	dbName string
	logger *zap.SugaredLogger
}

func New(logger *zap.SugaredLogger) *adapter {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	return &adapter{logger: logger}
}

func (a *adapter) Connect(config adapters.ConnectionConfig) error {
	dsn, ok := config["dsn"].(string)
	if !ok || dsn == "" {
		return fmt.Errorf("postgres: missing or invalid 'dsn' in config")
	}
	a.dsn = dsn

	// Parse database name from DSN
	u, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("postgres: failed to parse DSN: %w", err)
	}
	a.dbName = u.Path
	if len(a.dbName) > 0 && a.dbName[0] == '/' {
		a.dbName = a.dbName[1:]
	}
	if a.dbName == "" {
		a.dbName = "postgres"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("postgres: failed to create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("postgres: failed to ping: %w", err)
	}

	a.pool = pool
	return nil
}

func (a *adapter) Discover() ([]nodes.Node, []edges.Edge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var allNodes []nodes.Node
	var allEdges []edges.Edge

	// Root node for the database
	rootID := fmt.Sprintf("pg-%s", a.dbName)
	allNodes = append(allNodes, nodes.Node{
		Id:       rootID,
		Type:     string(nodes.TypeDatabase),
		Name:     a.dbName,
		Metadata: map[string]any{"adapter": "postgres", "database": a.dbName},
		Health:   "healthy",
	})

	// Discover tables
	rows, err := a.pool.Query(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: failed to query tables: %w", err)
	}
	defer rows.Close()

	tableIDs := map[string]string{} // table_name -> node id
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, nil, fmt.Errorf("postgres: failed to scan table row: %w", err)
		}
		nodeID := fmt.Sprintf("pg-%s-%s", a.dbName, tableName)
		tableIDs[tableName] = nodeID

		allNodes = append(allNodes, nodes.Node{
			Id:       nodeID,
			Type:     string(nodes.TypeTable),
			Name:     tableName,
			Parent:   rootID,
			Metadata: map[string]any{"adapter": "postgres", "schema": "public"},
			Health:   "healthy",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("postgres: rows error: %w", err)
	}

	// Discover foreign keys
	fkRows, err := a.pool.Query(ctx,
		`SELECT
			tc.constraint_name,
			kcu.table_name AS source_table,
			kcu.column_name AS source_column,
			ccu.table_name AS target_table,
			ccu.column_name AS target_column
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
			AND tc.table_schema = ccu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = 'public'`)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: failed to query foreign keys: %w", err)
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var constraintName, srcTable, srcCol, tgtTable, tgtCol string
		if err := fkRows.Scan(&constraintName, &srcTable, &srcCol, &tgtTable, &tgtCol); err != nil {
			return nil, nil, fmt.Errorf("postgres: failed to scan FK row: %w", err)
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
		return nil, nil, fmt.Errorf("postgres: FK rows error: %w", err)
	}

	return allNodes, allEdges, nil
}

func (a *adapter) Health() (adapters.HealthMetrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if a.pool == nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": "not connected"}, nil
	}

	if err := a.pool.Ping(ctx); err != nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": err.Error()}, nil
	}

	var activeConns int
	if err := a.pool.QueryRow(ctx,
		"SELECT count(*) FROM pg_stat_activity WHERE datname = $1", a.dbName).Scan(&activeConns); err != nil {
		a.logger.Warnw("postgres: failed to query active connections", "err", err)
	}

	return adapters.HealthMetrics{
		"status":             "healthy",
		"active_connections": activeConns,
		"pool_total":         int32(a.pool.Stat().TotalConns()),
		"pool_idle":          int32(a.pool.Stat().IdleConns()),
	}, nil
}

func (a *adapter) Close() error {
	if a.pool != nil {
		a.pool.Close()
	}
	return nil
}
