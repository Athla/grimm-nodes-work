package registry

import (
	"binary/internal/adapters"
	"binary/internal/graph"
	"binary/internal/graph/health"
	"sync"
)

type Registry interface {
	Register(name string, adapter adapters.Adapter, config adapters.ConnectionConfig)
	Get(name string) (adapters.Adapter, bool)
	Names() []string
	DiscoverAll() (*graph.Graph, error)
	HealthAll() []health.HealthMetrics
	CloseAll() error
}

type registry struct {
	mu       sync.RWMutex
	adapters map[string]adapters.Adapter
	config   map[string]adapters.ConnectionConfig
}

func NewRegistry() *registry {
	return &registry{}
}

func (r *registry) Register(name string, adapter adapters.Adapter, config adapters.ConnectionConfig) error {

	return nil
}
func (r *registry) Get(name string) (adapters.Adapter, bool) {

	return nil, false
}
func (r *registry) Names() []string {

	return nil
}
func (r *registry) DiscoverAll() (*graph.Graph, error) {

	return nil, nil
}
func (r *registry) HealthAll() []health.HealthMetrics {

	return nil
}
func (r *registry) CloseAll() error {

	return nil
}
