package adapters

import (
	"fmt"
	"sync"

	"binary/internal/graph/edges"
	"binary/internal/graph/nodes"
)

type ConnectionConfig map[string]any
type HealthMetrics map[string]any

// Adapter defines the contract that all service adapters must satisfy.
type Adapter interface {
	// Connect establishes a connection using the provided configuration.
	Connect(config ConnectionConfig) error

	// Discover performs recursive discovery (BFS) returning nodes and edges.
	Discover() ([]nodes.Node, []edges.Edge, error)

	// Health returns health metrics. By convention, adapters include a "status"
	// key ("healthy", "degraded", "unhealthy") in the returned map and return
	// nil error. The error return is reserved for cases where health cannot be
	// determined at all (e.g., adapter not initialized).
	Health() (HealthMetrics, error)

	// Close releases resources upon shutting down the service.
	Close() error
}

// AdapterConstructor is a factory function that creates a new Adapter instance.
type AdapterConstructor func() Adapter

var (
	factoryMu sync.RWMutex
	factories = make(map[string]AdapterConstructor)
)

// RegisterFactory registers an adapter constructor for the given connection type.
// Adapter packages call this from init() to self-register.
func RegisterFactory(connType string, ctor AdapterConstructor) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	factories[connType] = ctor
}

// NewAdapter creates a new adapter instance for the given connection type.
func NewAdapter(connType string) (Adapter, error) {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	ctor, ok := factories[connType]
	if !ok {
		return nil, fmt.Errorf("unknown adapter type %q", connType)
	}
	return ctor(), nil
}
