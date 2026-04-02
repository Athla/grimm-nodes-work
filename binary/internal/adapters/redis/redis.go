package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"binary/internal/adapters"
	"binary/internal/graph/edges"
	"binary/internal/graph/nodes"
)

var _ adapters.Adapter = (*adapter)(nil)

func init() {
	adapters.RegisterFactory("redis", func() adapters.Adapter { return New() })
}

type adapter struct {
	client *redis.Client
	addr   string
}

func New() *adapter {
	return &adapter{}
}

func (a *adapter) Connect(config adapters.ConnectionConfig) error {
	// Support URI-based connection (redis://...)
	if uri, ok := config["uri"].(string); ok && uri != "" {
		opts, err := redis.ParseURL(uri)
		if err != nil {
			return fmt.Errorf("redis: failed to parse URI: %w", err)
		}
		a.client = redis.NewClient(opts)
		a.addr = opts.Addr
	} else {
		// Support host/port/password config (from Docker discovery)
		host, _ := config["host"].(string)
		if host == "" {
			host = "localhost"
		}

		port := uint16(6379)
		if p, ok := config["port"].(uint16); ok {
			port = p
		}

		password, _ := config["password"].(string)

		a.addr = fmt.Sprintf("%s:%d", host, port)
		a.client = redis.NewClient(&redis.Options{
			Addr:     a.addr,
			Password: password,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.client.Ping(ctx).Err(); err != nil {
		a.client.Close()
		return fmt.Errorf("redis: failed to ping %s: %w", a.addr, err)
	}

	return nil
}

func (a *adapter) Discover() ([]nodes.Node, []edges.Edge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var allNodes []nodes.Node
	var allEdges []edges.Edge

	// Root node for this Redis instance
	rootID := fmt.Sprintf("redis-%s", sanitizeID(a.addr))
	allNodes = append(allNodes, nodes.Node{
		Id:       rootID,
		Type:     string(nodes.TypeDatabase),
		Name:     a.addr,
		Metadata: map[string]any{"adapter": "redis"},
		Health:   "healthy",
	})

	// Discover active keyspaces via INFO keyspace
	info, err := a.client.Info(ctx, "keyspace").Result()
	if err != nil {
		return allNodes, allEdges, nil
	}

	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "db") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		dbName := parts[0]
		dbMeta := parseKeyspaceInfo(parts[1])

		nodeID := fmt.Sprintf("redis-%s-%s", sanitizeID(a.addr), dbName)
		allNodes = append(allNodes, nodes.Node{
			Id:       nodeID,
			Type:     string(nodes.TypeDatabase),
			Name:     dbName,
			Parent:   rootID,
			Metadata: map[string]any{"adapter": "redis", "keys": dbMeta["keys"], "expires": dbMeta["expires"]},
			Health:   "healthy",
		})

		allEdges = append(allEdges, edges.Edge{
			Id:     fmt.Sprintf("redis-contains-%s-%s", sanitizeID(a.addr), dbName),
			Source: rootID,
			Target: nodeID,
			Type:   "contains",
			Label:  "contains",
		})
	}

	return allNodes, allEdges, nil
}

func (a *adapter) Health() (adapters.HealthMetrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if a.client == nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": "not connected"}, nil
	}

	if err := a.client.Ping(ctx).Err(); err != nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": err.Error()}, nil
	}

	metrics := adapters.HealthMetrics{
		"status": "healthy",
	}

	// Gather server info
	info, err := a.client.Info(ctx, "server", "memory", "clients").Result()
	if err == nil {
		parsed := parseInfoSections(info)
		if v, ok := parsed["redis_version"]; ok {
			metrics["version"] = v
		}
		if v, ok := parsed["used_memory"]; ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				metrics["used_memory"] = n
			}
		}
		if v, ok := parsed["connected_clients"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				metrics["connected_clients"] = n
			}
		}
		if v, ok := parsed["uptime_in_seconds"]; ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				metrics["uptime_seconds"] = n
			}
		}
	}

	return metrics, nil
}

func (a *adapter) Close() error {
	if a.client != nil {
		return a.client.Close()
	}
	return nil
}

// sanitizeID replaces characters unsuitable for node IDs.
func sanitizeID(s string) string {
	return strings.NewReplacer(":", "-", "/", "-").Replace(s)
}

// parseKeyspaceInfo parses "keys=4,expires=0,avg_ttl=0" into a map.
func parseKeyspaceInfo(s string) map[string]int {
	result := make(map[string]int)
	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			if n, err := strconv.Atoi(kv[1]); err == nil {
				result[kv[0]] = n
			}
		}
	}
	return result
}

// parseInfoSections parses Redis INFO output into key-value pairs.
func parseInfoSections(info string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			result[k] = v
		}
	}
	return result
}
