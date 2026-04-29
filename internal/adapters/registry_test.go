package adapters

import (
	"fmt"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

// stubAdapter is a minimal Adapter for testing registry operations.
type stubAdapter struct {
	connectErr  error
	connected   bool
	discoverN   []nodes.Node
	discoverE   []edges.Edge
	discoverErr error
}

func (s *stubAdapter) Connect(config ConnectionConfig) error {
	if s.connectErr != nil {
		return s.connectErr
	}
	s.connected = true
	return nil
}

func (s *stubAdapter) Discover() ([]nodes.Node, []edges.Edge, error) {
	return s.discoverN, s.discoverE, s.discoverErr
}

func (s *stubAdapter) Health() (HealthMetrics, error) {
	return HealthMetrics{"status": "healthy"}, nil
}

func (s *stubAdapter) Close() error { return nil }

func TestRegister_Success(t *testing.T) {
	reg := NewRegistry(zap.NewNop().Sugar())
	a := &stubAdapter{}

	err := reg.Register("test-db", "postgres", a, ConnectionConfig{"host": "localhost"})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if !a.connected {
		t.Error("expected adapter.Connect to be called")
	}

	got, ok := reg.Get("test-db")
	if !ok {
		t.Fatal("expected adapter to be retrievable via Get")
	}
	if got != a {
		t.Error("Get returned different adapter instance")
	}

	names := reg.Names()
	if len(names) != 1 || names[0] != "test-db" {
		t.Errorf("expected Names=['test-db'], got %v", names)
	}
}

func TestRegister_ConnectFailure(t *testing.T) {
	reg := NewRegistry(zap.NewNop().Sugar())
	a := &stubAdapter{connectErr: fmt.Errorf("connection refused")}

	err := reg.Register("bad-db", "postgres", a, ConnectionConfig{})
	if err == nil {
		t.Fatal("expected Register to return error on Connect failure")
	}

	_, ok := reg.Get("bad-db")
	if ok {
		t.Error("adapter should NOT be stored when Connect fails")
	}

	if len(reg.Names()) != 0 {
		t.Error("Names should be empty after failed register")
	}
}

func TestRegister_InvalidatesCache(t *testing.T) {
	reg := NewRegistry(zap.NewNop().Sugar())
	a1 := &stubAdapter{
		discoverN: []nodes.Node{{Id: "n1", Name: "first", Type: "postgres", Health: "healthy"}},
	}

	if err := reg.Register("db1", "postgres", a1, ConnectionConfig{}); err != nil {
		t.Fatal(err)
	}

	// First DiscoverAll populates cache.
	g1, err := reg.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	initialCount := len(g1.Nodes)

	// Register a second adapter — should invalidate cache.
	a2 := &stubAdapter{
		discoverN: []nodes.Node{{Id: "n2", Name: "second", Type: "redis", Health: "healthy"}},
	}
	if err := reg.Register("cache1", "redis", a2, ConnectionConfig{}); err != nil {
		t.Fatal(err)
	}

	g2, err := reg.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}

	// New DiscoverAll should include nodes from both adapters.
	if len(g2.Nodes) <= initialCount {
		t.Errorf("expected more nodes after second register, got %d (was %d)", len(g2.Nodes), initialCount)
	}
}

func TestRegister_ConcurrentWithDiscoverAll(t *testing.T) {
	reg := NewRegistry(zap.NewNop().Sugar())

	// Pre-register one adapter.
	a0 := &stubAdapter{
		discoverN: []nodes.Node{{Id: "n0", Name: "seed", Type: "postgres", Health: "healthy"}},
	}
	if err := reg.Register("seed", "postgres", a0, ConnectionConfig{}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	const n = 20

	// Concurrent DiscoverAll calls.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = reg.DiscoverAll()
		}()
	}

	// Concurrent Register calls.
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := &stubAdapter{
				discoverN: []nodes.Node{{Id: fmt.Sprintf("n-%d", i), Name: fmt.Sprintf("db-%d", i), Type: "postgres", Health: "healthy"}},
			}
			_ = reg.Register(fmt.Sprintf("db-%d", i), "postgres", a, ConnectionConfig{})
		}()
	}

	wg.Wait()
	// If we get here without deadlock or panic, the test passes.
	// The race detector (go test -race) verifies no data races.
}
