package server

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"binary/internal/adapters"
	"binary/internal/discovery"
	"binary/internal/graph"
	"binary/internal/graph/edges"
	"binary/internal/graph/health"
	"binary/internal/graph/nodes"
)

// trackingRegistry extends mockRegistry with call tracking for testing
// applyServices and buildOnChange.
type trackingRegistry struct {
	mu               sync.Mutex
	registered       []string
	registerErr      error
	topologyBySource map[string]int
	cacheInvalidated int
	graph            *graph.Graph
	health           []health.HealthMetrics
}

func newTrackingRegistry() *trackingRegistry {
	return &trackingRegistry{
		topologyBySource: make(map[string]int),
		graph:            &graph.Graph{},
	}
}

func (r *trackingRegistry) Register(name, connType string, adapter adapters.Adapter, config adapters.ConnectionConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.registerErr != nil {
		return r.registerErr
	}
	r.registered = append(r.registered, name)
	return nil
}

func (r *trackingRegistry) SetTopology(source string, n []nodes.Node, e []edges.Edge) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.topologyBySource[source] += len(n)
}

func (r *trackingRegistry) InvalidateCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cacheInvalidated++
}

func (r *trackingRegistry) Get(string) (adapters.Adapter, bool)  { return nil, false }
func (r *trackingRegistry) Names() []string                      { return nil }
func (r *trackingRegistry) DiscoverAll() (*graph.Graph, error)   { return r.graph, nil }
func (r *trackingRegistry) HealthAll() []health.HealthMetrics    { return r.health }
func (r *trackingRegistry) CloseAll() error                      { return nil }

// stubAdapter for factory registration.
type testAdapter struct{}

func (a *testAdapter) Connect(adapters.ConnectionConfig) error               { return nil }
func (a *testAdapter) Discover() ([]nodes.Node, []edges.Edge, error)         { return nil, nil, nil }
func (a *testAdapter) Health() (adapters.HealthMetrics, error)               { return nil, nil }
func (a *testAdapter) Close() error                                          { return nil }

func init() {
	// Register a test adapter factory so applyServices can call NewAdapter.
	adapters.RegisterFactory("test-type", func() adapters.Adapter { return &testAdapter{} })
}

// ── applyServices tests ──────────────────────────────────────────────

func TestApplyServices_AdapterOnly(t *testing.T) {
	reg := newTrackingRegistry()

	services := []discovery.ServiceInfo{
		{Name: "db1", Type: "test-type", Source: "docker", Config: adapters.ConnectionConfig{}},
		{Name: "db2", Type: "test-type", Source: "docker", Config: adapters.ConnectionConfig{}},
	}

	applyServices(reg, services)

	if len(reg.registered) != 2 {
		t.Errorf("expected 2 registrations, got %d", len(reg.registered))
	}
	if len(reg.topologyBySource) != 0 {
		t.Errorf("expected no topology calls, got %d sources", len(reg.topologyBySource))
	}
}

func TestApplyServices_TopologyOnly(t *testing.T) {
	reg := newTrackingRegistry()

	services := []discovery.ServiceInfo{
		{
			Name:   "k8s-ns1",
			Type:   "namespace",
			Source: "kubernetes",
			Nodes:  []nodes.Node{{Id: "n1"}, {Id: "n2"}},
			Edges:  []edges.Edge{{Id: "e1"}},
		},
	}

	applyServices(reg, services)

	if len(reg.registered) != 0 {
		t.Errorf("expected no registrations, got %d", len(reg.registered))
	}
	if reg.topologyBySource["kubernetes"] != 2 {
		t.Errorf("expected 2 topology nodes for kubernetes, got %d", reg.topologyBySource["kubernetes"])
	}
}

func TestApplyServices_Mixed(t *testing.T) {
	reg := newTrackingRegistry()

	services := []discovery.ServiceInfo{
		{Name: "db1", Type: "test-type", Source: "docker"},
		{Name: "ns", Type: "namespace", Source: "kubernetes", Nodes: []nodes.Node{{Id: "n1"}}},
	}

	applyServices(reg, services)

	if len(reg.registered) != 1 {
		t.Errorf("expected 1 registration, got %d", len(reg.registered))
	}
	if reg.topologyBySource["kubernetes"] != 1 {
		t.Errorf("expected 1 topology node, got %d", reg.topologyBySource["kubernetes"])
	}
}

func TestApplyServices_UnknownAdapterType(t *testing.T) {
	reg := newTrackingRegistry()

	services := []discovery.ServiceInfo{
		{Name: "db1", Type: "nonexistent-adapter-type", Source: "docker"},
		{Name: "db2", Type: "test-type", Source: "docker"},
	}

	// Should not panic; unknown type is logged and skipped.
	applyServices(reg, services)

	if len(reg.registered) != 1 {
		t.Errorf("expected 1 registration (unknown type skipped), got %d", len(reg.registered))
	}
}

// ── buildOnChange tests ──────────────────────────────────────────────

// fakeDiscoverer implements discovery.Discoverer for testing buildOnChange.
type fakeDiscoverer struct {
	name     string
	services []discovery.ServiceInfo
	err      error
}

func (f *fakeDiscoverer) Name() string { return f.name }
func (f *fakeDiscoverer) Discover(_ context.Context) ([]discovery.ServiceInfo, error) {
	return f.services, f.err
}
func (f *fakeDiscoverer) Watch(_ context.Context, _ func()) error { return nil }
func (f *fakeDiscoverer) Close() error                            { return nil }

func TestBuildOnChange_TopologyDiscoverer(t *testing.T) {
	reg := newTrackingRegistry()
	d := &fakeDiscoverer{
		name: "kubernetes",
		services: []discovery.ServiceInfo{
			{Name: "ns1", Source: "kubernetes", Nodes: []nodes.Node{{Id: "n1"}, {Id: "n2"}}},
		},
	}

	onChange := buildOnChange(context.Background(), d, reg)
	onChange()

	if reg.topologyBySource["kubernetes"] != 2 {
		t.Errorf("expected SetTopology with 2 nodes, got %d", reg.topologyBySource["kubernetes"])
	}
}

func TestBuildOnChange_AdapterDiscoverer(t *testing.T) {
	reg := newTrackingRegistry()
	d := &fakeDiscoverer{
		name: "docker",
		services: []discovery.ServiceInfo{
			{Name: "db1", Type: "test-type", Source: "docker"},
		},
	}

	onChange := buildOnChange(context.Background(), d, reg)
	onChange()

	// MF-1 fix: buildOnChange should call applyServices for adapter discoverers.
	if len(reg.registered) != 1 {
		t.Errorf("expected applyServices to register 1 adapter, got %d", len(reg.registered))
	}
}

func TestBuildOnChange_DiscoverError(t *testing.T) {
	reg := newTrackingRegistry()
	d := &fakeDiscoverer{
		name: "docker",
		err:  fmt.Errorf("daemon not running"),
	}

	onChange := buildOnChange(context.Background(), d, reg)
	onChange()

	if reg.cacheInvalidated != 1 {
		t.Errorf("expected cache invalidated on error, got %d", reg.cacheInvalidated)
	}
}
