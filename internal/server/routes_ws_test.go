package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/guilherme-grimm/graph-go/internal/graph"
	"github.com/guilherme-grimm/graph-go/internal/graph/health"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

// dialTestWebSocket creates an httptest server from s.RegisterRoutes(),
// dials the /websocket endpoint, and returns the connection plus a cleanup
// function that closes everything.
func dialTestWebSocket(t *testing.T, s *Server) (*websocket.Conn, func()) {
	t.Helper()
	srv := httptest.NewServer(s.RegisterRoutes())
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/websocket"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		cancel()
		srv.Close()
		t.Fatalf("dial: %v", err)
	}
	return conn, func() { conn.CloseNow(); cancel(); srv.Close() }
}

// healthMsg is the structure sent over the WebSocket.
type healthMsg struct {
	Type    string `json:"type"`
	Payload struct {
		NodeID string `json:"nodeId"`
		Health string `json:"health"`
	} `json:"payload"`
}

// readAllMessages reads WebSocket messages until the context deadline is
// reached. It collects and returns all successfully read messages.
func readAllMessages(t *testing.T, conn *websocket.Conn, timeout time.Duration) []healthMsg {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var msgs []healthMsg
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		var m healthMsg
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal ws message: %v", err)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func TestWebSocket_HealthUpdates(t *testing.T) {
	reg := &mockRegistry{
		graph: &graph.Graph{
			Nodes: []nodes.Node{
				{Id: "service-pg", Type: "postgres", Name: "pg", Metadata: map[string]any{"adapter": "pg"}},
			},
		},
		health: []health.HealthMetrics{
			{NodeID: "pg", Status: health.Healthy},
		},
	}
	s := newTestServer(reg)
	conn, cleanup := dialTestWebSocket(t, s)
	defer cleanup()

	msgs := readAllMessages(t, conn, 2*time.Second)

	if len(msgs) == 0 {
		t.Fatal("expected at least one health_update message")
	}

	found := false
	for _, m := range msgs {
		if m.Type != "health_update" {
			t.Errorf("unexpected message type %q", m.Type)
		}
		if m.Payload.NodeID == "service-pg" && m.Payload.Health == "healthy" {
			found = true
		}
	}
	if !found {
		t.Errorf("did not find health_update for service-pg with health=healthy; got %+v", msgs)
	}
}

func TestWebSocket_MultipleNodes(t *testing.T) {
	reg := &mockRegistry{
		graph: &graph.Graph{
			Nodes: []nodes.Node{
				{Id: "service-pg", Type: "postgres", Name: "pg", Metadata: map[string]any{"adapter": "pg"}},
				{Id: "service-mongo", Type: "mongodb", Name: "mongo", Metadata: map[string]any{"adapter": "mongo"}},
			},
		},
		health: []health.HealthMetrics{
			{NodeID: "pg", Status: health.Healthy},
			{NodeID: "mongo", Status: health.Degraded},
		},
	}
	s := newTestServer(reg)
	conn, cleanup := dialTestWebSocket(t, s)
	defer cleanup()

	msgs := readAllMessages(t, conn, 2*time.Second)

	seen := make(map[string]string) // nodeId -> health
	for _, m := range msgs {
		seen[m.Payload.NodeID] = m.Payload.Health
	}

	if seen["service-pg"] != "healthy" {
		t.Errorf("expected service-pg=healthy, got %q", seen["service-pg"])
	}
	if seen["service-mongo"] != "degraded" {
		t.Errorf("expected service-mongo=degraded, got %q", seen["service-mongo"])
	}
}

func TestWebSocket_NoAdapterMetadata(t *testing.T) {
	reg := &mockRegistry{
		graph: &graph.Graph{
			Nodes: []nodes.Node{
				{Id: "orphan-node", Type: "unknown", Name: "orphan", Metadata: map[string]any{}},
			},
		},
		health: []health.HealthMetrics{
			{NodeID: "pg", Status: health.Healthy},
		},
	}
	s := newTestServer(reg)
	conn, cleanup := dialTestWebSocket(t, s)
	defer cleanup()

	msgs := readAllMessages(t, conn, 2*time.Second)

	for _, m := range msgs {
		if m.Payload.NodeID == "orphan-node" {
			t.Errorf("should not have received message for node without adapter metadata")
		}
	}
}

func TestWebSocket_AdapterNotInHealth(t *testing.T) {
	reg := &mockRegistry{
		graph: &graph.Graph{
			Nodes: []nodes.Node{
				{Id: "service-redis", Type: "redis", Name: "redis", Metadata: map[string]any{"adapter": "redis"}},
			},
		},
		health: []health.HealthMetrics{}, // no health for "redis"
	}
	s := newTestServer(reg)
	conn, cleanup := dialTestWebSocket(t, s)
	defer cleanup()

	msgs := readAllMessages(t, conn, 2*time.Second)

	for _, m := range msgs {
		if m.Payload.NodeID == "service-redis" {
			t.Errorf("should not have received message for adapter with no health metric")
		}
	}
}

func TestWebSocket_DiscoveryError(t *testing.T) {
	reg := &mockRegistry{
		graph:   nil,
		discErr: fmt.Errorf("docker daemon unreachable"),
		health:  []health.HealthMetrics{},
	}
	s := newTestServer(reg)
	conn, cleanup := dialTestWebSocket(t, s)
	defer cleanup()

	// Should not crash; just no messages sent.
	msgs := readAllMessages(t, conn, 2*time.Second)

	for _, m := range msgs {
		t.Errorf("unexpected message when DiscoverAll errors: %+v", m)
	}
}

func TestWebSocket_NilGraph(t *testing.T) {
	reg := &mockRegistry{
		graph:  nil,
		health: []health.HealthMetrics{{NodeID: "pg", Status: health.Healthy}},
	}
	s := newTestServer(reg)
	conn, cleanup := dialTestWebSocket(t, s)
	defer cleanup()

	msgs := readAllMessages(t, conn, 2*time.Second)

	for _, m := range msgs {
		t.Errorf("unexpected message when graph is nil: %+v", m)
	}
}

func TestWebSocket_ClientClose(t *testing.T) {
	reg := &mockRegistry{
		graph: &graph.Graph{
			Nodes: []nodes.Node{
				{Id: "service-pg", Type: "postgres", Name: "pg", Metadata: map[string]any{"adapter": "pg"}},
			},
		},
		health: []health.HealthMetrics{
			{NodeID: "pg", Status: health.Healthy},
		},
	}
	s := newTestServer(reg)
	conn, cleanup := dialTestWebSocket(t, s)
	defer cleanup()

	// Close from client side immediately.
	conn.Close(websocket.StatusNormalClosure, "bye")

	// If the handler leaks a goroutine this test will eventually be
	// caught by the race detector or -count flag. Here we just verify
	// the close doesn't panic.
}
