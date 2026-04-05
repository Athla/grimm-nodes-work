package docker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"

	"binary/internal/adapters"
	"binary/internal/discovery"
)

// SourceName is the Source tag applied to ServiceInfo produced by this discoverer.
const SourceName = "docker"

// DiscoveredService represents a service found via Docker daemon inspection.
type DiscoveredService struct {
	Name        string
	Type        ServiceType
	Config      adapters.ConnectionConfig
	IPAddress   string
	ContainerID string
}

// DockerDiscoveryConfig holds settings for Docker-based discovery.
type DockerDiscoveryConfig struct {
	Socket       string
	Network      string
	IgnoreImages []string
	SelfImage    string
}

// DockerDiscovery connects to the Docker daemon and discovers running services.
// It implements discovery.Discoverer.
type DockerDiscovery struct {
	client *client.Client
	cfg    DockerDiscoveryConfig
}

// Compile-time check that DockerDiscovery satisfies discovery.Discoverer.
var _ discovery.Discoverer = (*DockerDiscovery)(nil)

// Name returns the discoverer identifier used for logging.
func (d *DockerDiscovery) Name() string { return SourceName }

// NewDockerDiscovery creates a new DockerDiscovery instance and verifies
// connectivity to the daemon via Ping.
func NewDockerDiscovery(cfg DockerDiscoveryConfig) (*DockerDiscovery, error) {
	if cfg.Socket == "" {
		cfg.Socket = "/var/run/docker.sock"
	}

	opts := []client.Opt{
		client.WithHost("unix://" + cfg.Socket),
		client.WithAPIVersionNegotiation(),
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("discovery: failed to create Docker client: %w", err)
	}

	// Verify connectivity
	if _, err := cli.Ping(context.Background()); err != nil {
		cli.Close()
		return nil, fmt.Errorf("discovery: failed to ping Docker daemon: %w", err)
	}

	return &DockerDiscovery{client: cli, cfg: cfg}, nil
}

// Close releases the Docker client resources.
func (d *DockerDiscovery) Close() error {
	if d.client != nil {
		return d.client.Close()
	}
	return nil
}

// Client returns the underlying Docker client (used by EventWatcher).
func (d *DockerDiscovery) Client() *client.Client {
	return d.client
}

// Discover lists running containers and inspects each to build a list of
// discovered services. Each service is returned as a discovery.ServiceInfo
// with Source="docker"; Docker-specific fields (IP, container ID) are
// preserved in Metadata.
func (d *DockerDiscovery) Discover(ctx context.Context) ([]discovery.ServiceInfo, error) {
	listOpts := container.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("status", "running"),
		),
	}

	containers, err := d.client.ContainerList(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("discovery: failed to list containers: %w", err)
	}

	services := make([]discovery.ServiceInfo, 0, len(containers))
	for _, ctr := range containers {
		svc, ok := d.processContainer(ctx, ctr)
		if !ok {
			continue
		}
		services = append(services, discovery.ServiceInfo{
			Name:   svc.Name,
			Type:   string(svc.Type),
			Source: SourceName,
			Config: svc.Config,
			Metadata: map[string]any{
				"ip_address":   svc.IPAddress,
				"container_id": svc.ContainerID,
			},
		})
	}

	return services, nil
}

// Watch subscribes to Docker container start/stop/die events and invokes
// onChange for each one, debounced to collapse bursts (e.g. docker-compose up
// starting many containers at once). It blocks until ctx is cancelled.
func (d *DockerDiscovery) Watch(ctx context.Context, onChange func()) error {
	deb := discovery.NewDebouncer(onChange, 2*time.Second)
	defer deb.Stop()
	return watchEvents(ctx, d.client, deb.Trigger)
}

func (d *DockerDiscovery) processContainer(ctx context.Context, ctr types.Container) (DiscoveredService, bool) {
	// Inspect for full details (env vars, network settings)
	inspect, err := d.client.ContainerInspect(ctx, ctr.ID)
	if err != nil {
		log.Printf("WARNING: failed to inspect container %s: %v", truncateID(ctr.ID), err)
		return DiscoveredService{}, false
	}

	labels := ctr.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	// 1. Check ignore label
	if ShouldIgnore(labels) {
		return DiscoveredService{}, false
	}

	// 2. Derive name
	name := deriveContainerName(labels, ctr.Names)

	// 3. Check ignore images list
	if isIgnoredImage(ctr.Image, d.cfg.IgnoreImages) {
		return DiscoveredService{}, false
	}

	// 4. Classify image
	svcType := ClassifyImage(ctr.Image)

	// 5. Resolve container hostname
	host := resolveContainerHost(inspect, d.cfg.Network)

	// 6. Extract credentials
	var envVars []string
	if inspect.Config != nil {
		envVars = inspect.Config.Env
	}
	connCfg := ExtractCredentials(svcType, envVars, host, name)

	// 7. Apply label overrides
	svcType, connCfg = ApplyLabelOverrides(labels, svcType, connCfg)

	// Resolve IP address for metadata
	ip := resolveContainerIP(inspect, d.cfg.Network)

	return DiscoveredService{
		Name:        name,
		Type:        svcType,
		Config:      connCfg,
		IPAddress:   ip,
		ContainerID: ctr.ID,
	}, true
}

// deriveContainerName prefers the Docker Compose service label, then
// falls back to the container name (stripped of leading /).
func deriveContainerName(labels map[string]string, names []string) string {
	if svc, ok := labels["com.docker.compose.service"]; ok && svc != "" {
		return svc
	}

	if len(names) > 0 {
		name := names[0]
		return strings.TrimPrefix(name, "/")
	}

	return "unknown"
}

// resolveContainerHost returns the hostname for connecting to the container.
// Prefers the Compose service name, then IP address on the specified network,
// then the first available network IP.
func resolveContainerHost(inspect types.ContainerJSON, network string) string {
	// Prefer Compose service name as hostname (Docker DNS)
	if inspect.Config != nil && inspect.Config.Labels != nil {
		if svc, ok := inspect.Config.Labels["com.docker.compose.service"]; ok && svc != "" {
			return svc
		}
	}

	return resolveContainerIP(inspect, network)
}

// resolveContainerIP returns the container's IP address, preferring the
// specified network if set.
func resolveContainerIP(inspect types.ContainerJSON, network string) string {
	if inspect.NetworkSettings == nil || inspect.NetworkSettings.Networks == nil {
		return "localhost"
	}

	// If a specific network is requested, look for it
	if network != "" {
		for netName, netSettings := range inspect.NetworkSettings.Networks {
			if netName == network && netSettings.IPAddress != "" {
				return netSettings.IPAddress
			}
		}
	}

	// Fall back to first available network
	for _, netSettings := range inspect.NetworkSettings.Networks {
		if netSettings.IPAddress != "" {
			return netSettings.IPAddress
		}
	}

	return "localhost"
}

// truncateID safely truncates a container ID to 12 characters.
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// isIgnoredImage checks if the image matches any of the ignore patterns.
func isIgnoredImage(image string, ignoreList []string) bool {
	normalized := strings.ToLower(image)
	for _, pattern := range ignoreList {
		if strings.Contains(normalized, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
