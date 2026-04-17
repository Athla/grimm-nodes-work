package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
	"github.com/guilherme-grimm/graph-go/internal/config"
	"github.com/guilherme-grimm/graph-go/internal/discovery"
	"github.com/guilherme-grimm/graph-go/internal/discovery/docker"
	"github.com/guilherme-grimm/graph-go/internal/discovery/kubernetes"
	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"

	// Self-registering adapters
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/elasticsearch"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/http"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/mongodb"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/mysql"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/postgres"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/redis"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/s3"
)

type Server struct {
	port           int
	allowedOrigins []string
	registry       adapters.Registry
	logger         *zap.SugaredLogger
}

// NewServer returns the HTTP server and a cleanup function that should
// be called during graceful shutdown to close adapter connections.
func NewServer(cfg *config.Config, logger *zap.SugaredLogger) (*http.Server, func()) {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	allowedOrigins := cfg.Server.AllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:3000", "http://localhost:5173"}
	}

	reg := adapters.NewRegistry(logger)

	// Build the list of active discoverers. Each one is responsible for its
	// own auto-detect (typically returning nil if its backing system is
	// unavailable), so wiring here is uniform.
	var discoverers []discovery.Discoverer
	if dd := buildDockerDiscovery(cfg, logger); dd != nil {
		discoverers = append(discoverers, dd)
	}
	if kd := buildKubernetesDiscovery(cfg, logger); kd != nil {
		discoverers = append(discoverers, kd)
	}

	// Run Discover() in parallel across all active discoverers.
	services := discoverAll(discoverers, logger)

	// Merge with YAML config
	yamlEntries := make([]discovery.YAMLEntry, len(cfg.Connections))
	for i, entry := range cfg.Connections {
		yamlEntries[i] = discovery.YAMLEntry{
			Name:   entry.Name,
			Type:   entry.Type,
			Config: entry.ToConnectionConfig(),
		}
	}
	services = discovery.MergeWithYAML(services, yamlEntries)

	// Split: topology-bearing ServiceInfo (e.g. K8s resources) go directly
	// into the registry's topology set; adapter-bound ones get registered.
	applyServices(reg, services, logger)

	// Start a Watch() goroutine for each discoverer. Topology-oriented
	// discoverers (those whose Discover returns ServiceInfo with Nodes)
	// refresh their topology on each callback; adapter-oriented ones just
	// invalidate the cache so next DiscoverAll re-queries adapters.
	watchCtx, watchCancel := context.WithCancel(context.Background())
	for _, d := range discoverers {
		d := d
		onChange := buildOnChange(watchCtx, d, reg, logger)
		go func() {
			if err := d.Watch(watchCtx, onChange); err != nil {
				logger.Warnw("discoverer watch stopped", "discoverer", d.Name(), "err", err)
			}
		}()
		logger.Infow("discoverer watch started", "discoverer", d.Name())
	}

	s := &Server{
		port:           port,
		allowedOrigins: allowedOrigins,
		registry:       reg,
		logger:         logger,
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      s.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	cleanup := func() {
		watchCancel()
		for _, d := range discoverers {
			if err := d.Close(); err != nil {
				logger.Warnw("error closing discoverer", "discoverer", d.Name(), "err", err)
			}
		}
		if err := reg.CloseAll(); err != nil {
			logger.Warnw("error closing adapters", "err", err)
		}
	}

	return server, cleanup
}

// applyServices splits ServiceInfo entries into topology (ones carrying
// pre-built Nodes, like Kubernetes resources) and adapter-bound ones, then
// pushes each into the registry appropriately. Topology entries are grouped
// by Source so a later refresh can replace them atomically.
func applyServices(reg adapters.Registry, services []discovery.ServiceInfo, logger *zap.SugaredLogger) {
	topologyBySource := make(map[string]*struct {
		nodes []nodes.Node
		edges []edges.Edge
	})
	for _, svc := range services {
		if len(svc.Nodes) > 0 || len(svc.Edges) > 0 {
			bucket, ok := topologyBySource[svc.Source]
			if !ok {
				bucket = &struct {
					nodes []nodes.Node
					edges []edges.Edge
				}{}
				topologyBySource[svc.Source] = bucket
			}
			bucket.nodes = append(bucket.nodes, svc.Nodes...)
			bucket.edges = append(bucket.edges, svc.Edges...)
			continue
		}

		adapter, err := adapters.NewAdapter(svc.Type, logger)
		if err != nil {
			logger.Warnw("skipping service: unknown adapter type", "service", svc.Name, "type", svc.Type, "err", err)
			continue
		}
		if err := reg.Register(svc.Name, svc.Type, adapter, svc.Config); err != nil {
			logger.Warnw("adapter register failed", "service", svc.Name, "type", svc.Type, "err", err)
		} else {
			logger.Infow("adapter registered", "service", svc.Name, "type", svc.Type)
		}
	}
	for source, bucket := range topologyBySource {
		reg.SetTopology(source, bucket.nodes, bucket.edges)
		logger.Infow("topology applied", "source", source, "nodes", len(bucket.nodes), "edges", len(bucket.edges))
	}
}

// buildOnChange returns the callback invoked on discoverer events. For
// discoverers that produce topology ServiceInfo, it re-reads their snapshot
// and replaces the registry topology before invalidating caches. For pure
// adapter discoverers, it just invalidates the cache.
func buildOnChange(ctx context.Context, d discovery.Discoverer, reg adapters.Registry, logger *zap.SugaredLogger) func() {
	return func() {
		fresh, err := d.Discover(ctx)
		if err != nil {
			logger.Warnw("discoverer refresh failed", "discoverer", d.Name(), "err", err)
			reg.InvalidateCache()
			return
		}
		// Determine if this discoverer contributes topology at all.
		var hasTopology bool
		var topoNodes []nodes.Node
		var topoEdges []edges.Edge
		for _, svc := range fresh {
			if len(svc.Nodes) > 0 || len(svc.Edges) > 0 {
				hasTopology = true
				topoNodes = append(topoNodes, svc.Nodes...)
				topoEdges = append(topoEdges, svc.Edges...)
			}
		}
		if hasTopology {
			reg.SetTopology(d.Name(), topoNodes, topoEdges)
			return // SetTopology already invalidates the cache
		}
		// Re-apply all services so new containers get adapters registered
		// (and removed containers are handled gracefully on next DiscoverAll).
		applyServices(reg, fresh, logger)
	}
}

// discoverAll runs Discover() on every discoverer in parallel and returns
// the concatenated ServiceInfo list. A failure from one discoverer is
// logged but does not block the others.
func discoverAll(discoverers []discovery.Discoverer, logger *zap.SugaredLogger) []discovery.ServiceInfo {
	if len(discoverers) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := make([][]discovery.ServiceInfo, len(discoverers))
	var wg sync.WaitGroup
	for i, d := range discoverers {
		i, d := i, d
		wg.Add(1)
		go func() {
			defer wg.Done()
			discovered, err := d.Discover(ctx)
			if err != nil {
				logger.Warnw("discovery failed", "discoverer", d.Name(), "err", err)
				return
			}
			logger.Infow("discovery complete", "discoverer", d.Name(), "services", len(discovered))
			results[i] = discovered
		}()
	}
	wg.Wait()

	var all []discovery.ServiceInfo
	for _, r := range results {
		all = append(all, r...)
	}
	return all
}

func buildDockerDiscovery(cfg *config.Config, logger *zap.SugaredLogger) *docker.DockerDiscovery {
	if !shouldEnableDocker(cfg) {
		return nil
	}

	socket := "/var/run/docker.sock"
	if cfg.Docker.Socket != "" {
		socket = cfg.Docker.Socket
	}
	network := cfg.Docker.Network
	ignoreImages := cfg.Docker.IgnoreImages

	dd, err := docker.NewDockerDiscovery(docker.DockerDiscoveryConfig{
		Socket:       socket,
		Network:      network,
		IgnoreImages: ignoreImages,
	}, logger)
	if err != nil {
		logger.Warnw("docker discovery unavailable, falling back to YAML-only", "err", err)
		return nil
	}
	logger.Infow("docker discovery enabled")
	return dd
}

func buildKubernetesDiscovery(cfg *config.Config, logger *zap.SugaredLogger) *kubernetes.Discovery {
	if cfg.Kubernetes.Enabled != nil && !*cfg.Kubernetes.Enabled {
		return nil
	}

	k8sCfg := kubernetes.Config{
		KubeconfigPath: cfg.Kubernetes.Kubeconfig,
		Context:        cfg.Kubernetes.Context,
		Namespaces:     cfg.Kubernetes.Namespaces,
	}

	kd, err := kubernetes.New(k8sCfg)
	if err != nil {
		logger.Warnw("kubernetes discovery unavailable", "err", err)
		return nil
	}
	if kd == nil {
		// No in-cluster config and no kubeconfig — silent skip.
		return nil
	}
	logger.Infow("kubernetes discovery enabled")
	return kd
}

// shouldEnableDocker determines if Docker discovery should be attempted.
// If cfg.Docker.Enabled is explicitly set, use that. Otherwise auto-detect
// by checking if the Docker socket exists.
func shouldEnableDocker(cfg *config.Config) bool {
	if cfg.Docker.Enabled != nil {
		return *cfg.Docker.Enabled
	}

	// Auto-detect: check if Docker socket exists
	socket := "/var/run/docker.sock"
	if cfg.Docker.Socket != "" {
		socket = cfg.Docker.Socket
	}

	_, err := os.Stat(socket)
	return err == nil
}
