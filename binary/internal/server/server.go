package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"binary/internal/adapters"
	"binary/internal/config"
	"binary/internal/discovery"
	"binary/internal/discovery/docker"
	"binary/internal/discovery/kubernetes"
	"binary/internal/graph/edges"
	"binary/internal/graph/nodes"

	// Self-registering adapters
	_ "binary/internal/adapters/elasticsearch"
	_ "binary/internal/adapters/http"
	_ "binary/internal/adapters/mongodb"
	_ "binary/internal/adapters/mysql"
	_ "binary/internal/adapters/postgres"
	_ "binary/internal/adapters/redis"
	_ "binary/internal/adapters/s3"
)

type Server struct {
	port     int
	registry adapters.Registry
}

// NewServer returns the HTTP server and a cleanup function that should
// be called during graceful shutdown to close adapter connections.
func NewServer(cfg *config.Config) (*http.Server, func()) {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil || port == 0 {
		port = 8080
		log.Printf("PORT not set or invalid, defaulting to %d", port)
	}

	reg := adapters.NewRegistry()

	// Build the list of active discoverers. Each one is responsible for its
	// own auto-detect (typically returning nil if its backing system is
	// unavailable), so wiring here is uniform.
	var discoverers []discovery.Discoverer
	if dd := buildDockerDiscovery(cfg); dd != nil {
		discoverers = append(discoverers, dd)
	}
	if kd := buildKubernetesDiscovery(cfg); kd != nil {
		discoverers = append(discoverers, kd)
	}

	// Run Discover() in parallel across all active discoverers.
	services := discoverAll(discoverers)

	// Merge with YAML config
	var yamlEntries []discovery.YAMLEntry
	if cfg != nil {
		yamlEntries = make([]discovery.YAMLEntry, len(cfg.Connections))
		for i, entry := range cfg.Connections {
			yamlEntries[i] = discovery.YAMLEntry{
				Name:   entry.Name,
				Type:   entry.Type,
				Config: entry.ToConnectionConfig(),
			}
		}
	}
	services = discovery.MergeWithYAML(services, yamlEntries)

	// Split: topology-bearing ServiceInfo (e.g. K8s resources) go directly
	// into the registry's topology set; adapter-bound ones get registered.
	applyServices(reg, services)

	// Start a Watch() goroutine for each discoverer. Topology-oriented
	// discoverers (those whose Discover returns ServiceInfo with Nodes)
	// refresh their topology on each callback; adapter-oriented ones just
	// invalidate the cache so next DiscoverAll re-queries adapters.
	watchCtx, watchCancel := context.WithCancel(context.Background())
	for _, d := range discoverers {
		d := d
		onChange := buildOnChange(watchCtx, d, reg)
		go func() {
			if err := d.Watch(watchCtx, onChange); err != nil {
				log.Printf("%s watch stopped: %v", d.Name(), err)
			}
		}()
		log.Printf("%s event watcher started", d.Name())
	}

	s := &Server{
		port:     port,
		registry: reg,
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
				log.Printf("error closing %s discoverer: %v", d.Name(), err)
			}
		}
		if err := reg.CloseAll(); err != nil {
			log.Printf("error closing adapters: %v", err)
		}
	}

	return server, cleanup
}

// applyServices splits ServiceInfo entries into topology (ones carrying
// pre-built Nodes, like Kubernetes resources) and adapter-bound ones, then
// pushes each into the registry appropriately. Topology entries are grouped
// by Source so a later refresh can replace them atomically.
func applyServices(reg adapters.Registry, services []discovery.ServiceInfo) {
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

		adapter, err := adapters.NewAdapter(svc.Type)
		if err != nil {
			log.Printf("WARNING: %v (skipping %q)", err, svc.Name)
			continue
		}
		if err := reg.Register(svc.Name, svc.Type, adapter, svc.Config); err != nil {
			log.Printf("WARNING: failed to register %q adapter: %v", svc.Name, err)
		} else {
			log.Printf("%s adapter %q registered successfully", svc.Type, svc.Name)
		}
	}
	for source, bucket := range topologyBySource {
		reg.SetTopology(source, bucket.nodes, bucket.edges)
		log.Printf("%s topology: %d nodes, %d edges", source, len(bucket.nodes), len(bucket.edges))
	}
}

// buildOnChange returns the callback invoked on discoverer events. For
// discoverers that produce topology ServiceInfo, it re-reads their snapshot
// and replaces the registry topology before invalidating caches. For pure
// adapter discoverers, it just invalidates the cache.
func buildOnChange(ctx context.Context, d discovery.Discoverer, reg adapters.Registry) func() {
	return func() {
		fresh, err := d.Discover(ctx)
		if err != nil {
			log.Printf("%s refresh failed: %v", d.Name(), err)
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
		applyServices(reg, fresh)
	}
}

// discoverAll runs Discover() on every discoverer in parallel and returns
// the concatenated ServiceInfo list. A failure from one discoverer is
// logged but does not block the others.
func discoverAll(discoverers []discovery.Discoverer) []discovery.ServiceInfo {
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
				log.Printf("WARNING: %s discovery failed: %v", d.Name(), err)
				return
			}
			log.Printf("%s discovery found %d services", d.Name(), len(discovered))
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

func buildDockerDiscovery(cfg *config.Config) *docker.DockerDiscovery {
	if !shouldEnableDocker(cfg) {
		return nil
	}

	socket := "/var/run/docker.sock"
	network := ""
	var ignoreImages []string
	if cfg != nil {
		if cfg.Docker.Socket != "" {
			socket = cfg.Docker.Socket
		}
		network = cfg.Docker.Network
		ignoreImages = cfg.Docker.IgnoreImages
	}

	dd, err := docker.NewDockerDiscovery(docker.DockerDiscoveryConfig{
		Socket:       socket,
		Network:      network,
		IgnoreImages: ignoreImages,
	})
	if err != nil {
		log.Printf("WARNING: Docker discovery unavailable: %v (falling back to YAML-only)", err)
		return nil
	}
	log.Println("Docker discovery enabled")
	return dd
}

func buildKubernetesDiscovery(cfg *config.Config) *kubernetes.Discovery {
	if cfg != nil && cfg.Kubernetes.Enabled != nil && !*cfg.Kubernetes.Enabled {
		return nil
	}

	k8sCfg := kubernetes.Config{}
	if cfg != nil {
		k8sCfg.KubeconfigPath = cfg.Kubernetes.Kubeconfig
		k8sCfg.Context = cfg.Kubernetes.Context
		k8sCfg.Namespaces = cfg.Kubernetes.Namespaces
	}

	kd, err := kubernetes.New(k8sCfg)
	if err != nil {
		log.Printf("WARNING: Kubernetes discovery unavailable: %v", err)
		return nil
	}
	if kd == nil {
		// No in-cluster config and no kubeconfig — silent skip.
		return nil
	}
	log.Println("Kubernetes discovery enabled")
	return kd
}

// shouldEnableDocker determines if Docker discovery should be attempted.
// If cfg.Docker.Enabled is explicitly set, use that. Otherwise auto-detect
// by checking if the Docker socket exists.
func shouldEnableDocker(cfg *config.Config) bool {
	if cfg != nil && cfg.Docker.Enabled != nil {
		return *cfg.Docker.Enabled
	}

	// Auto-detect: check if Docker socket exists
	socket := "/var/run/docker.sock"
	if cfg != nil && cfg.Docker.Socket != "" {
		socket = cfg.Docker.Socket
	}

	_, err := os.Stat(socket)
	return err == nil
}
