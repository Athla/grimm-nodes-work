package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"binary/internal/adapters"
	"binary/internal/graph"
	"binary/internal/graph/edges"
	"binary/internal/graph/health"
	"binary/internal/graph/nodes"
)

// mockRegistry implements adapters.Registry for testing.
type mockRegistry struct {
	graph   *graph.Graph
	health  []health.HealthMetrics
	discErr error
}

func (m *mockRegistry) Register(string, string, adapters.Adapter, adapters.ConnectionConfig) error {
	return nil
}
func (m *mockRegistry) Get(string) (adapters.Adapter, bool) { return nil, false }
func (m *mockRegistry) Names() []string                     { return nil }
func (m *mockRegistry) DiscoverAll() (*graph.Graph, error)  { return m.graph, m.discErr }
func (m *mockRegistry) HealthAll() []health.HealthMetrics   { return m.health }
func (m *mockRegistry) InvalidateCache()                    {}
func (m *mockRegistry) SetTopology(string, []nodes.Node, []edges.Edge) {}
func (m *mockRegistry) CloseAll() error                     { return nil }

func testGraph() *graph.Graph {
	return &graph.Graph{
		Nodes: []nodes.Node{
			{Id: "pg-test", Type: "database", Name: "test", Health: "healthy", Metadata: map[string]any{}},
			{Id: "pg-test-users", Type: "table", Name: "users", Parent: "pg-test", Health: "healthy", Metadata: map[string]any{}},
		},
		Edges: []edges.Edge{
			{Id: "fk-1", Source: "pg-test-users", Target: "pg-test", Type: "foreign_key"},
		},
	}
}

func newTestServer(reg adapters.Registry) *Server {
	return &Server{registry: reg}
}

func TestGraphHandler(t *testing.T) {
	s := newTestServer(&mockRegistry{graph: testGraph()})
	srv := httptest.NewServer(http.HandlerFunc(s.graphHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, ok := body["data"]; !ok {
		t.Fatal("response missing 'data' key")
	}

	var g graph.Graph
	if err := json.Unmarshal(body["data"], &g); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(g.Edges))
	}
}

func TestGraphHandler_EmptyAdapters(t *testing.T) {
	s := newTestServer(&mockRegistry{graph: &graph.Graph{
		Nodes: make([]nodes.Node, 0),
		Edges: make([]edges.Edge, 0),
	}})
	srv := httptest.NewServer(http.HandlerFunc(s.graphHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	data := string(raw["data"])
	// Verify arrays are [] not null
	if !strings.Contains(data, `"nodes":[]`) {
		t.Errorf("expected non-null nodes array, got: %s", data)
	}
	if !strings.Contains(data, `"edges":[]`) {
		t.Errorf("expected non-null edges array, got: %s", data)
	}
}

func TestNodeHandler_Found(t *testing.T) {
	s := newTestServer(&mockRegistry{graph: testGraph()})
	r := s.RegisterRoutes()

	req := httptest.NewRequest("GET", "/api/node/pg-test-users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]json.RawMessage
	json.NewDecoder(w.Body).Decode(&body)

	var node nodes.Node
	json.Unmarshal(body["data"], &node)
	if node.Id != "pg-test-users" {
		t.Errorf("expected node id 'pg-test-users', got %q", node.Id)
	}
}

func TestNodeHandler_NotFound(t *testing.T) {
	s := newTestServer(&mockRegistry{graph: testGraph()})
	r := s.RegisterRoutes()

	req := httptest.NewRequest("GET", "/api/node/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestApiHealthHandler_Ok(t *testing.T) {
	s := newTestServer(&mockRegistry{
		health: []health.HealthMetrics{
			{NodeID: "postgres", Status: health.Healthy},
		},
	})
	srv := httptest.NewServer(http.HandlerFunc(s.apiHealthHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", body["status"])
	}
	if body["timestamp"] == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestApiHealthHandler_Degraded(t *testing.T) {
	s := newTestServer(&mockRegistry{
		health: []health.HealthMetrics{
			{NodeID: "postgres", Status: health.Degraded},
		},
	})
	srv := httptest.NewServer(http.HandlerFunc(s.apiHealthHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)

	if body["status"] != "degraded" {
		t.Errorf("expected 'degraded', got %q", body["status"])
	}
}

func TestApiHealthHandler_Error(t *testing.T) {
	s := newTestServer(&mockRegistry{
		health: []health.HealthMetrics{
			{NodeID: "postgres", Status: health.Unhealthy},
		},
	})
	srv := httptest.NewServer(http.HandlerFunc(s.apiHealthHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)

	if body["status"] != "error" {
		t.Errorf("expected 'error', got %q", body["status"])
	}
}

func TestGraphHandler_Error(t *testing.T) {
	s := newTestServer(&mockRegistry{discErr: fmt.Errorf("connection refused")})
	srv := httptest.NewServer(http.HandlerFunc(s.graphHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestHealthHandler(t *testing.T) {
	s := newTestServer(&mockRegistry{})
	srv := httptest.NewServer(http.HandlerFunc(s.healthHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", body["status"])
	}
}
