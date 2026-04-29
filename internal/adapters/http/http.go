package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

var _ adapters.Adapter = (*adapter)(nil)

func init() {
	adapters.RegisterFactory("http", func(l *zap.SugaredLogger) adapters.Adapter { return New(l) })
}

type DependsOnEntry struct {
	Target string `json:"target"`
	Label  string `json:"label"`
}

type adapter struct {
	client     *http.Client
	endpoint   string
	healthPath string
	nodeType   string
	name       string
	dependsOn  []DependsOnEntry
	logger     *zap.SugaredLogger
}

func New(logger *zap.SugaredLogger) *adapter {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	return &adapter{
		client: &http.Client{Timeout: 5 * time.Second},
		logger: logger,
	}
}

func (a *adapter) Connect(config adapters.ConnectionConfig) error {
	a.endpoint, _ = config["endpoint"].(string)
	if a.endpoint == "" {
		return fmt.Errorf("http: endpoint is required")
	}

	a.healthPath, _ = config["health_path"].(string)
	if a.healthPath == "" {
		a.healthPath = "/health"
	}

	a.nodeType, _ = config["node_type"].(string)
	if a.nodeType == "" {
		a.nodeType = "service"
	}

	a.name, _ = config["name"].(string)
	if a.name == "" {
		return fmt.Errorf("http: name is required")
	}

	switch deps := config["depends_on"].(type) {
	case []map[string]string:
		for _, d := range deps {
			a.dependsOn = append(a.dependsOn, DependsOnEntry{
				Target: d["target"],
				Label:  d["label"],
			})
		}
	case []any:
		for _, raw := range deps {
			if d, ok := raw.(map[string]any); ok {
				target, _ := d["target"].(string)
				label, _ := d["label"].(string)
				a.dependsOn = append(a.dependsOn, DependsOnEntry{
					Target: target,
					Label:  label,
				})
			}
		}
	}

	// Verify reachability
	resp, err := a.client.Get(a.endpoint + a.healthPath)
	if err != nil {
		return fmt.Errorf("http: health check failed for %s: %w", a.name, err)
	}
	defer resp.Body.Close()

	return nil
}

func (a *adapter) Discover() ([]nodes.Node, []edges.Edge, error) {
	var allNodes []nodes.Node
	var allEdges []edges.Edge

	nodeID := a.name

	allNodes = append(allNodes, nodes.Node{
		Id:   nodeID,
		Type: a.nodeType,
		Name: a.name,
		Metadata: map[string]any{
			"adapter":  a.name,
			"endpoint": a.endpoint,
		},
		Health: "healthy",
	})

	for _, dep := range a.dependsOn {
		label := dep.Label
		if label == "" {
			label = "connects_to"
		}
		allEdges = append(allEdges, edges.Edge{
			Id:     fmt.Sprintf("%s-to-%s", nodeID, dep.Target),
			Source: nodeID,
			Target: dep.Target,
			Type:   "depends_on",
			Label:  label,
		})
	}

	return allNodes, allEdges, nil
}

func (a *adapter) Health() (adapters.HealthMetrics, error) {
	resp, err := a.client.Get(a.endpoint + a.healthPath)
	if err != nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return adapters.HealthMetrics{"status": "unhealthy", "http_status": resp.StatusCode}, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body == nil {
			body = make(map[string]any)
		}
		if _, exists := body["status"]; !exists {
			body["status"] = "healthy"
		}
		return body, nil
	}

	return adapters.HealthMetrics{"status": "degraded", "http_status": resp.StatusCode}, nil
}

func (a *adapter) Close() error {
	return nil
}
