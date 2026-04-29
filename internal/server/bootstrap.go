package server

import (
	"context"
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
)

// BuildRegistry constructs the adapter registry and the list of active
// discoverers based on cfg. It runs one Discover() sweep, applies YAML +
// auto-discovered services to the registry, and returns a cleanup func
// that closes discoverers and the registry in the correct order.
//
// It does NOT start Watch() loops — that's NewServer's job.
func BuildRegistry(cfg *config.Config, logger *zap.SugaredLogger) (adapters.Registry, []discovery.Discoverer, func()) {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	reg := adapters.NewRegistry(logger)

	var discoverers []discovery.Discoverer
	if dd := buildDockerDiscovery(cfg, logger); dd != nil {
		discoverers = append(discoverers, dd)
	}
	if kd := buildKubernetesDiscovery(cfg, logger); kd != nil {
		discoverers = append(discoverers, kd)
	}

	services := discoverAll(discoverers, logger)

	yamlEntries := make([]discovery.YAMLEntry, len(cfg.Connections))
	for i, entry := range cfg.Connections {
		yamlEntries[i] = discovery.YAMLEntry{
			Name:   entry.Name,
			Type:   entry.Type,
			Config: entry.ToConnectionConfig(),
		}
	}
	services = discovery.MergeWithYAML(services, yamlEntries)

	applyServices(reg, services, logger)

	cleanup := func() {
		for _, d := range discoverers {
			if err := d.Close(); err != nil {
				logger.Warnw("error closing discoverer", "discoverer", d.Name(), "err", err)
			}
		}
		if err := reg.CloseAll(); err != nil {
			logger.Warnw("error closing adapters", "err", err)
		}
	}

	return reg, discoverers, cleanup
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

	socket := "/var/run/docker.sock"
	if cfg.Docker.Socket != "" {
		socket = cfg.Docker.Socket
	}

	_, err := os.Stat(socket)
	return err == nil
}
