package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"binary/internal/adapters"
	httpAdapter "binary/internal/adapters/http"
	"binary/internal/adapters/mongodb"
	"binary/internal/adapters/postgres"
	"binary/internal/adapters/s3"
	"binary/internal/config"
	"binary/internal/discovery"
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

	var dockerDiscovery *discovery.DockerDiscovery
	var eventWatcher *discovery.EventWatcher

	// Docker discovery
	dockerEnabled := shouldEnableDocker(cfg)
	if dockerEnabled {
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

		dd, err := discovery.NewDockerDiscovery(discovery.DockerDiscoveryConfig{
			Socket:       socket,
			Network:      network,
			IgnoreImages: ignoreImages,
		})
		if err != nil {
			log.Printf("WARNING: Docker discovery unavailable: %v (falling back to YAML-only)", err)
		} else {
			dockerDiscovery = dd
			log.Println("Docker discovery enabled")
		}
	}

	// Discover services
	var services []discovery.DiscoveredService

	if dockerDiscovery != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		discovered, err := dockerDiscovery.Discover(ctx)
		if err != nil {
			log.Printf("WARNING: Docker discovery failed: %v (falling back to YAML-only)", err)
		} else {
			log.Printf("Docker discovery found %d services", len(discovered))
			services = discovered
		}
	}

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
	services = discovery.MergeServices(services, yamlEntries)

	// Register all services
	for _, svc := range services {
		adapter, err := adapterFactory(string(svc.Type))
		if err != nil {
			log.Printf("WARNING: %v (skipping %q)", err, svc.Name)
			continue
		}

		if err := reg.Register(svc.Name, string(svc.Type), adapter, svc.Config); err != nil {
			log.Printf("WARNING: failed to register %q adapter: %v", svc.Name, err)
		} else {
			log.Printf("%s adapter %q registered successfully", svc.Type, svc.Name)
		}
	}

	// Start event watcher for cache invalidation
	if dockerDiscovery != nil {
		eventWatcher = discovery.NewEventWatcher(dockerDiscovery.Client(), reg)
		eventWatcher.Start(context.Background())
		log.Println("Docker event watcher started")
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
		if eventWatcher != nil {
			eventWatcher.Stop()
		}
		if dockerDiscovery != nil {
			if err := dockerDiscovery.Close(); err != nil {
				log.Printf("error closing Docker client: %v", err)
			}
		}
		if err := reg.CloseAll(); err != nil {
			log.Printf("error closing adapters: %v", err)
		}
	}

	return server, cleanup
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

func adapterFactory(connType string) (adapters.Adapter, error) {
	switch connType {
	case "postgres":
		return postgres.New(), nil
	case "mongodb":
		return mongodb.New(), nil
	case "s3":
		return s3.New(), nil
	case "http":
		return httpAdapter.New(), nil
	case "redis":
		return nil, fmt.Errorf("redis adapter not yet implemented")
	default:
		return nil, fmt.Errorf("unknown adapter type %q", connType)
	}
}
