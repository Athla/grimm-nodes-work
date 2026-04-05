// Package discovery defines the Discoverer contract that all service discovery
// backends (Docker, Kubernetes, etc.) must implement. Each backend lives in its
// own subpackage and returns a uniform []ServiceInfo that the server
// concatenates into the final graph.
package discovery

import (
	"context"

	"binary/internal/adapters"
	"binary/internal/graph/edges"
	"binary/internal/graph/health"
	"binary/internal/graph/nodes"
)

// ServiceInfo is the uniform output of any Discoverer. Docker-style
// discoverers populate Config (for adapter bridging); orchestrator-style
// discoverers populate Nodes/Edges directly for topology rendering.
type ServiceInfo struct {
	Name     string
	Type     string
	Source   string
	Config   adapters.ConnectionConfig
	Nodes    []nodes.Node
	Edges    []edges.Edge
	Health   health.Status
	Metadata map[string]any
}

// Discoverer is implemented by each discovery backend. Implementations must be
// safe to call Discover concurrently with Watch callbacks firing.
type Discoverer interface {
	// Name returns a short identifier used for logging (e.g. "docker", "kubernetes").
	Name() string

	// Discover returns the current snapshot of services known to this backend.
	Discover(ctx context.Context) ([]ServiceInfo, error)

	// Watch starts watching for topology changes and invokes onChange whenever
	// a change is detected. It returns when ctx is cancelled or an unrecoverable
	// error occurs.
	Watch(ctx context.Context, onChange func()) error

	// Close releases resources held by the discoverer (clients, informers, etc.).
	Close() error
}
