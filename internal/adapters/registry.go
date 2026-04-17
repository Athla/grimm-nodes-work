package adapters

import (
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/guilherme-grimm/graph-go/internal/graph"
	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/health"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

const defaultCacheTTL = 30 * time.Second

type Registry interface {
	Register(name string, connType string, adapter Adapter, config ConnectionConfig) error
	Get(name string) (Adapter, bool)
	Names() []string
	DiscoverAll() (*graph.Graph, error)
	HealthAll() []health.HealthMetrics
	InvalidateCache()
	// SetTopology replaces the set of pre-built nodes/edges contributed by a
	// discoverer source (e.g. "kubernetes"). These are merged into DiscoverAll
	// output alongside adapter-derived nodes. Calling with empty slices clears
	// the source's contribution.
	SetTopology(source string, n []nodes.Node, e []edges.Edge)
	CloseAll() error
}

type topologySet struct {
	nodes []nodes.Node
	edges []edges.Edge
}

type registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
	config   map[string]ConnectionConfig
	types    map[string]string

	topoMu   sync.RWMutex
	topology map[string]topologySet

	cacheMu     sync.RWMutex
	cachedGraph *graph.Graph
	cacheTime   time.Time
	cacheTTL    time.Duration

	sf singleflight.Group

	healthCacheMu   sync.RWMutex
	cachedHealth    []health.HealthMetrics
	healthCacheTime time.Time
	healthCacheTTL  time.Duration
	healthSF        singleflight.Group
}

func NewRegistry() Registry {
	return &registry{
		adapters:       make(map[string]Adapter),
		config:         make(map[string]ConnectionConfig),
		types:          make(map[string]string),
		topology:       make(map[string]topologySet),
		cacheTTL:       defaultCacheTTL,
		healthCacheTTL: 10 * time.Second,
	}
}

func (r *registry) SetTopology(source string, n []nodes.Node, e []edges.Edge) {
	r.topoMu.Lock()
	if len(n) == 0 && len(e) == 0 {
		delete(r.topology, source)
	} else {
		// Copy to guard against concurrent mutation of caller's slices.
		nc := make([]nodes.Node, len(n))
		copy(nc, n)
		ec := make([]edges.Edge, len(e))
		copy(ec, e)
		r.topology[source] = topologySet{nodes: nc, edges: ec}
	}
	r.topoMu.Unlock()
	r.InvalidateCache()
}

// snapshotTopology returns a flat copy of all topology contributions,
// sorted by source for deterministic ordering.
func (r *registry) snapshotTopology() ([]nodes.Node, []edges.Edge) {
	r.topoMu.RLock()
	defer r.topoMu.RUnlock()
	if len(r.topology) == 0 {
		return nil, nil
	}
	var allN []nodes.Node
	var allE []edges.Edge
	for _, ts := range r.topology {
		allN = append(allN, ts.nodes...)
		allE = append(allE, ts.edges...)
	}
	return allN, allE
}

func (r *registry) Register(name string, connType string, adapter Adapter, config ConnectionConfig) error {
	// Connect outside the lock — this is a network call (DNS, TCP, auth) that
	// can take seconds. Holding mu during Connect blocks all reads.
	if err := adapter.Connect(config); err != nil {
		return fmt.Errorf("registry: failed to connect adapter %q: %w", name, err)
	}

	r.mu.Lock()
	r.adapters[name] = adapter
	r.config[name] = config
	r.types[name] = connType
	r.mu.Unlock()

	r.InvalidateCache()
	return nil
}

func (r *registry) Get(name string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.adapters[name]
	return a, ok
}

func (r *registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	return names
}

func shallowCopyGraph(g *graph.Graph) *graph.Graph {
	if g == nil {
		return nil
	}
	n := make([]nodes.Node, len(g.Nodes))
	copy(n, g.Nodes)
	e := make([]edges.Edge, len(g.Edges))
	copy(e, g.Edges)
	return &graph.Graph{Nodes: n, Edges: e}
}

func (r *registry) DiscoverAll() (*graph.Graph, error) {
	// Check cache first
	r.cacheMu.RLock()
	if r.cachedGraph != nil && time.Since(r.cacheTime) < r.cacheTTL {
		g := shallowCopyGraph(r.cachedGraph)
		r.cacheMu.RUnlock()
		return g, nil
	}
	r.cacheMu.RUnlock()

	// Use singleflight to prevent cache stampede
	v, err, _ := r.sf.Do("discover", func() (any, error) {
		// Double-check cache inside singleflight callback
		r.cacheMu.RLock()
		if r.cachedGraph != nil && time.Since(r.cacheTime) < r.cacheTTL {
			g := shallowCopyGraph(r.cachedGraph)
			r.cacheMu.RUnlock()
			return g, nil
		}
		r.cacheMu.RUnlock()

		// Snapshot adapters under lock, then release before network calls
		type entry struct {
			name, connType string
			adapter        Adapter
		}
		r.mu.RLock()
		snapshot := make([]entry, 0, len(r.adapters))
		for name, a := range r.adapters {
			snapshot = append(snapshot, entry{name, r.types[name], a})
		}
		r.mu.RUnlock()

		allNodes := make([]nodes.Node, 0)
		allEdges := make([]edges.Edge, 0)

		for _, e := range snapshot {
			n, edg, err := e.adapter.Discover()
			if err != nil {
				log.Printf("WARNING: discover failed for %q: %v (skipping)", e.name, err)
				allNodes = append(allNodes, nodes.Node{
					Id:       fmt.Sprintf("service-%s", e.name),
					Type:     e.connType,
					Name:     e.name,
					Metadata: map[string]any{"adapter": e.name, "error": err.Error()},
					Health:   "unhealthy",
				})
				continue
			}

			if e.connType == "http" {
				// HTTP adapters manage their own top-level node directly
				allNodes = append(allNodes, n...)
				allEdges = append(allEdges, edg...)
				continue
			}

			// Create a service-level parent node from config
			serviceID := fmt.Sprintf("service-%s", e.name)
			allNodes = append(allNodes, nodes.Node{
				Id:       serviceID,
				Type:     e.connType,
				Name:     e.name,
				Metadata: map[string]any{"adapter": e.name},
				Health:   "healthy",
			})

			// Inject registry name into child metadata so WebSocket health
		// matching works even when the adapter hardcodes its own type.
			for i := range n {
				if n[i].Metadata == nil {
					n[i].Metadata = map[string]any{}
				}
				n[i].Metadata["adapter"] = e.name
			}

			// Re-parent top-level nodes under the service node
			for i := range n {
				if n[i].Parent == "" {
					n[i].Parent = serviceID
					allEdges = append(allEdges, edges.Edge{
						Id:     fmt.Sprintf("service-contains-%s-%s", e.name, n[i].Id),
						Source: serviceID,
						Target: n[i].Id,
						Type:   "contains",
						Label:  "contains",
					})
				}
			}

			allNodes = append(allNodes, n...)
			allEdges = append(allEdges, edg...)
		}

		// Merge in topology contributions (e.g. Kubernetes) that don't
		// go through the adapter path.
		topoN, topoE := r.snapshotTopology()
		allNodes = append(allNodes, topoN...)
		allEdges = append(allEdges, topoE...)

		g := &graph.Graph{Nodes: allNodes, Edges: allEdges}

		// Store in cache
		r.cacheMu.Lock()
		r.cachedGraph = g
		r.cacheTime = time.Now()
		r.cacheMu.Unlock()

		return shallowCopyGraph(g), nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*graph.Graph), nil
}

func (r *registry) HealthAll() []health.HealthMetrics {
	// Check cache first
	r.healthCacheMu.RLock()
	if r.cachedHealth != nil && time.Since(r.healthCacheTime) < r.healthCacheTTL {
		result := make([]health.HealthMetrics, len(r.cachedHealth))
		copy(result, r.cachedHealth)
		r.healthCacheMu.RUnlock()
		return result
	}
	r.healthCacheMu.RUnlock()

	v, err, _ := r.healthSF.Do("health", func() (any, error) {
		// Double-check cache inside singleflight
		r.healthCacheMu.RLock()
		if r.cachedHealth != nil && time.Since(r.healthCacheTime) < r.healthCacheTTL {
			result := make([]health.HealthMetrics, len(r.cachedHealth))
			copy(result, r.cachedHealth)
			r.healthCacheMu.RUnlock()
			return result, nil
		}
		r.healthCacheMu.RUnlock()

		// Snapshot adapters under lock
		type entry struct {
			name    string
			adapter Adapter
		}
		r.mu.RLock()
		snapshot := make([]entry, 0, len(r.adapters))
		for name, a := range r.adapters {
			snapshot = append(snapshot, entry{name, a})
		}
		r.mu.RUnlock()

		metrics := make([]health.HealthMetrics, 0, len(snapshot))
		for _, e := range snapshot {
			m, err := e.adapter.Health()
			status := health.Healthy
			if err != nil {
				status = health.Unhealthy
			} else if s, ok := m["status"].(string); ok && s != "healthy" {
				status = health.Status(s)
			}

			metrics = append(metrics, health.HealthMetrics{
				NodeID:    e.name,
				Status:    status,
				Metrics:   m,
				Timestamp: time.Now(),
			})
		}

		// Store in cache
		r.healthCacheMu.Lock()
		r.cachedHealth = metrics
		r.healthCacheTime = time.Now()
		r.healthCacheMu.Unlock()

		result := make([]health.HealthMetrics, len(metrics))
		copy(result, metrics)
		return result, nil
	})

	if err != nil {
		log.Printf("WARNING: health check failed: %v", err)
		return nil
	}
	if v == nil {
		return nil
	}
	return v.([]health.HealthMetrics)
}

func (r *registry) CloseAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, adapter := range r.adapters {
		if err := adapter.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("registry: close errors: %v", errs)
	}
	return nil
}

func (r *registry) InvalidateCache() {
	r.cacheMu.Lock()
	r.cachedGraph = nil
	r.cacheTime = time.Time{}
	r.cacheMu.Unlock()

	r.healthCacheMu.Lock()
	r.cachedHealth = nil
	r.healthCacheTime = time.Time{}
	r.healthCacheMu.Unlock()
}
