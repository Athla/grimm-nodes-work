package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"binary/internal/adapters"
	"binary/internal/graph/edges"
	"binary/internal/graph/nodes"
)

var systemDBs = map[string]bool{
	"admin":  true,
	"config": true,
	"local":  true,
}

var _ adapters.Adapter = (*adapter)(nil)

func init() {
	adapters.RegisterFactory("mongodb", func() adapters.Adapter { return New() })
}

type adapter struct {
	client *mongo.Client
	uri    string
}

func New() *adapter {
	return &adapter{}
}

func (a *adapter) Connect(config adapters.ConnectionConfig) error {
	uri, ok := config["uri"].(string)
	if !ok || uri == "" {
		return fmt.Errorf("mongodb: missing or invalid 'uri' in config")
	}
	a.uri = uri

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("mongodb: failed to connect: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return fmt.Errorf("mongodb: failed to ping: %w", err)
	}

	a.client = client
	return nil
}

func (a *adapter) Discover() ([]nodes.Node, []edges.Edge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var allNodes []nodes.Node
	var allEdges []edges.Edge

	dbNames, err := a.client.ListDatabaseNames(ctx, map[string]any{})
	if err != nil {
		return nil, nil, fmt.Errorf("mongodb: failed to list databases: %w", err)
	}

	for _, dbName := range dbNames {
		if systemDBs[dbName] {
			continue
		}

		dbID := fmt.Sprintf("mongo-%s", dbName)
		allNodes = append(allNodes, nodes.Node{
			Id:       dbID,
			Type:     string(nodes.TypeDatabase),
			Name:     dbName,
			Metadata: map[string]any{"adapter": "mongodb"},
			Health:   "healthy",
		})

		db := a.client.Database(dbName)
		collNames, err := db.ListCollectionNames(ctx, map[string]any{})
		if err != nil {
			return nil, nil, fmt.Errorf("mongodb: failed to list collections for %s: %w", dbName, err)
		}

		for _, collName := range collNames {
			collID := fmt.Sprintf("mongo-%s-%s", dbName, collName)
			allNodes = append(allNodes, nodes.Node{
				Id:       collID,
				Type:     string(nodes.TypeCollection),
				Name:     collName,
				Parent:   dbID,
				Metadata: map[string]any{"adapter": "mongodb", "database": dbName},
				Health:   "healthy",
			})

			allEdges = append(allEdges, edges.Edge{
				Id:     fmt.Sprintf("mongo-contains-%s-%s", dbName, collName),
				Source: dbID,
				Target: collID,
				Type:   "contains",
				Label:  "contains",
			})
		}
	}

	return allNodes, allEdges, nil
}

func (a *adapter) Health() (adapters.HealthMetrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if a.client == nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": "not connected"}, nil
	}

	if err := a.client.Ping(ctx, nil); err != nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": err.Error()}, nil
	}

	dbNames, err := a.client.ListDatabaseNames(ctx, map[string]any{})
	if err != nil {
		return adapters.HealthMetrics{"status": "degraded", "error": err.Error()}, nil
	}

	count := 0
	for _, name := range dbNames {
		if !systemDBs[name] {
			count++
		}
	}

	return adapters.HealthMetrics{
		"status":         "healthy",
		"database_count": count,
	}, nil
}

func (a *adapter) Close() error {
	if a.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.client.Disconnect(ctx)
	}
	return nil
}
