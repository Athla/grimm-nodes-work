package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"

	"binary/internal/adapters"
	"binary/internal/graph/edges"
	"binary/internal/graph/nodes"
)

var _ adapters.Adapter = (*adapter)(nil)

func init() {
	adapters.RegisterFactory("elasticsearch", func() adapters.Adapter { return New() })
}

type adapter struct {
	client   *elasticsearch.Client
	endpoint string
}

func New() *adapter {
	return &adapter{}
}

func (a *adapter) Connect(config adapters.ConnectionConfig) error {
	endpoint, _ := config["endpoint"].(string)
	if endpoint == "" {
		return fmt.Errorf("elasticsearch: 'endpoint' is required")
	}
	a.endpoint = endpoint

	cfg := elasticsearch.Config{
		Addresses: []string{endpoint},
	}

	if username, ok := config["username"].(string); ok && username != "" {
		cfg.Username = username
	}
	if password, ok := config["password"].(string); ok && password != "" {
		cfg.Password = password
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("elasticsearch: failed to create client: %w", err)
	}

	// Verify connectivity
	res, err := client.Info()
	if err != nil {
		return fmt.Errorf("elasticsearch: failed to connect to %s: %w", endpoint, err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch: server returned %s", res.Status())
	}

	a.client = client
	return nil
}

func (a *adapter) Discover() ([]nodes.Node, []edges.Edge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var allNodes []nodes.Node
	var allEdges []edges.Edge

	// Root node for this cluster
	rootID := fmt.Sprintf("es-%s", sanitizeID(a.endpoint))
	allNodes = append(allNodes, nodes.Node{
		Id:       rootID,
		Type:     string(nodes.TypeElasticsearch),
		Name:     a.endpoint,
		Metadata: map[string]any{"adapter": "elasticsearch"},
		Health:   "healthy",
	})

	// List indices via _cat/indices
	res, err := a.client.Cat.Indices(
		a.client.Cat.Indices.WithContext(ctx),
		a.client.Cat.Indices.WithFormat("json"),
	)
	if err != nil {
		return allNodes, allEdges, nil
	}
	defer res.Body.Close()

	if res.IsError() {
		return allNodes, allEdges, nil
	}

	var indices []catIndex
	if err := json.NewDecoder(res.Body).Decode(&indices); err != nil {
		return allNodes, allEdges, nil
	}

	for _, idx := range indices {
		// Skip system indices
		if strings.HasPrefix(idx.Index, ".") {
			continue
		}

		nodeID := fmt.Sprintf("es-%s-%s", sanitizeID(a.endpoint), idx.Index)
		allNodes = append(allNodes, nodes.Node{
			Id:     nodeID,
			Type:   string(nodes.TypeIndex),
			Name:   idx.Index,
			Parent: rootID,
			Metadata: map[string]any{
				"adapter":   "elasticsearch",
				"health":    idx.Health,
				"doc_count": idx.DocsCount,
				"size":      idx.StoreSize,
			},
			Health: mapIndexHealth(idx.Health),
		})

		allEdges = append(allEdges, edges.Edge{
			Id:     fmt.Sprintf("es-contains-%s-%s", sanitizeID(a.endpoint), idx.Index),
			Source: rootID,
			Target: nodeID,
			Type:   "contains",
			Label:  "contains",
		})
	}

	return allNodes, allEdges, nil
}

func (a *adapter) Health() (adapters.HealthMetrics, error) {
	if a.client == nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": "not connected"}, nil
	}

	res, err := a.client.Cluster.Health()
	if err != nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": err.Error()}, nil
	}
	defer res.Body.Close()

	if res.IsError() {
		return adapters.HealthMetrics{"status": "unhealthy", "error": res.Status()}, nil
	}

	var health clusterHealth
	if err := json.NewDecoder(res.Body).Decode(&health); err != nil {
		return adapters.HealthMetrics{"status": "degraded", "error": err.Error()}, nil
	}

	status := "healthy"
	switch health.Status {
	case "yellow":
		status = "degraded"
	case "red":
		status = "unhealthy"
	}

	return adapters.HealthMetrics{
		"status":              status,
		"cluster_name":        health.ClusterName,
		"cluster_status":      health.Status,
		"number_of_nodes":     health.NumberOfNodes,
		"active_shards":       health.ActiveShards,
		"relocating_shards":   health.RelocatingShards,
		"unassigned_shards":   health.UnassignedShards,
		"active_primary_shards": health.ActivePrimaryShards,
	}, nil
}

func (a *adapter) Close() error {
	// elasticsearch client uses HTTP — no persistent connection to close
	return nil
}

type catIndex struct {
	Index     string `json:"index"`
	Health    string `json:"health"`
	DocsCount string `json:"docs.count"`
	StoreSize string `json:"store.size"`
}

type clusterHealth struct {
	ClusterName         string `json:"cluster_name"`
	Status              string `json:"status"`
	NumberOfNodes       int    `json:"number_of_nodes"`
	ActiveShards        int    `json:"active_shards"`
	RelocatingShards    int    `json:"relocating_shards"`
	UnassignedShards    int    `json:"unassigned_shards"`
	ActivePrimaryShards int    `json:"active_primary_shards"`
}

func sanitizeID(s string) string {
	return strings.NewReplacer(":", "-", "/", "-", ".", "-").Replace(s)
}

func mapIndexHealth(h string) string {
	switch h {
	case "green":
		return "healthy"
	case "yellow":
		return "degraded"
	case "red":
		return "unhealthy"
	default:
		return "unknown"
	}
}
