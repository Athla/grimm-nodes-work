package adapters

import (
	"binary/internal/graph/edges"
	"binary/internal/graph/nodes"
)

type ConnectionConfig map[string]any
type HealthMetrics map[string]any

// Contracts that bind the adapters and what they should do
type Adapter interface {
	// Simple connection to the service, fetches from the config file
	Connect(config ConnectionConfig) error

	// Recursive method using BFS  to find and map everything
	Discover() ([]nodes.Node, []edges.Edge, error)

	// Just a abstract method of how to get basic health metrics
	Health() (HealthMetrics, error)

	// Entrypoint for connection closing upon shutting down the service
	Close() error
}
