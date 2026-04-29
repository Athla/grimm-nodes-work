package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/guilherme-grimm/graph-go/internal/graph"
	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/health"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

func TestWriteGraphJSON_HappyPath(t *testing.T) {
	reg := &mockRegistry{graph: testGraph()}
	var buf bytes.Buffer

	if err := WriteGraphJSON(&buf, reg, false, false); err != nil {
		t.Fatalf("WriteGraphJSON: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	raw, ok := env["data"]
	if !ok {
		t.Fatal("response missing 'data' key")
	}

	var g graph.Graph
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(g.Edges))
	}

	// Non-pretty output must be a single compact line.
	if bytes.Contains(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n")) {
		t.Errorf("non-pretty output should not contain embedded newlines, got: %s", buf.String())
	}
}

func TestWriteGraphJSON_WithHealthMerges(t *testing.T) {
	g := &graph.Graph{
		Nodes: []nodes.Node{
			{
				Id:       "pg-test",
				Type:     "database",
				Name:     "test",
				Metadata: map[string]any{"adapter": "pg-test"},
			},
		},
		Edges: make([]edges.Edge, 0),
	}
	reg := &mockRegistry{
		graph: g,
		health: []health.HealthMetrics{
			{NodeID: "pg-test", Status: health.Healthy},
		},
	}

	var buf bytes.Buffer
	if err := WriteGraphJSON(&buf, reg, true, false); err != nil {
		t.Fatalf("WriteGraphJSON: %v", err)
	}

	var env struct {
		Data graph.Graph `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(env.Data.Nodes))
	}
	if got := env.Data.Nodes[0].Health; got != string(health.Healthy) {
		t.Errorf("expected health %q, got %q", health.Healthy, got)
	}
}

func TestWriteGraphJSON_PrettyIndents(t *testing.T) {
	reg := &mockRegistry{graph: testGraph()}
	var buf bytes.Buffer

	if err := WriteGraphJSON(&buf, reg, false, true); err != nil {
		t.Fatalf("WriteGraphJSON: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "\n") {
		t.Errorf("pretty output should contain newlines, got: %s", out)
	}
	if !strings.Contains(out, "\n  ") {
		t.Errorf("pretty output should contain two-space indent, got: %s", out)
	}

	// Output must still be valid JSON.
	var env map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("pretty output is not valid JSON: %v", err)
	}
}

func TestWriteGraphJSON_DiscoverError(t *testing.T) {
	reg := &mockRegistry{discErr: errors.New("boom")}
	var buf bytes.Buffer

	err := WriteGraphJSON(&buf, reg, false, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "discover:") {
		t.Errorf("error should be wrapped with 'discover:', got: %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should contain underlying message 'boom', got: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty buffer on error, got %d bytes: %s", buf.Len(), buf.String())
	}
}
