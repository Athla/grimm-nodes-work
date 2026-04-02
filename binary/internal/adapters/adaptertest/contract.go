package adaptertest

import (
	"testing"

	"binary/internal/adapters"
)

// ContractOpts configures adapter-specific expectations for the contract suite.
type ContractOpts struct {
	// Minimum node count after seeding.
	MinNodes int
	// Minimum edge count after seeding.
	MinEdges int
	// Expected type of the root node (e.g., "elasticsearch", "database").
	RootNodeType string
	// Expected types for child nodes (e.g., "index", "table").
	ChildNodeTypes []string
	// Whether to expect foreign_key edges (MySQL only).
	ExpectFKEdges bool
	// Health metric keys that must be present.
	RequiredHealthKeys []string
}

// RunContractTests validates that an adapter satisfies the Adapter interface contract.
// The adapter must already be connected. newAdapter returns a fresh, unconnected instance
// for tests that need a disposable adapter (Connect/missing, Close).
func RunContractTests(t *testing.T, a adapters.Adapter, newAdapter func() adapters.Adapter, config adapters.ConnectionConfig, opts ContractOpts) {
	t.Helper()

	t.Run("Connect", func(t *testing.T) {
		t.Run("valid_config", func(t *testing.T) {
			t.Parallel()
			fresh := newAdapter()
			if err := fresh.Connect(config); err != nil {
				t.Fatalf("Connect with valid config returned error: %v", err)
			}
			fresh.Close()
		})

		t.Run("missing_required_field", func(t *testing.T) {
			t.Parallel()
			fresh := newAdapter()
			err := fresh.Connect(adapters.ConnectionConfig{})
			if err == nil {
				t.Fatal("Connect with empty config should return an error")
			}
		})
	})

	// Run Discover once and share results across subtests.
	allNodes, allEdges, discoverErr := a.Discover()

	t.Run("Discover", func(t *testing.T) {
		if discoverErr != nil {
			t.Fatalf("Discover returned error: %v", discoverErr)
		}

		t.Run("returns_root_node", func(t *testing.T) {
			t.Parallel()
			var found bool
			for _, n := range allNodes {
				if n.Parent == "" {
					if n.Type != opts.RootNodeType {
						t.Errorf("root node type = %q, want %q", n.Type, opts.RootNodeType)
					}
					found = true
					break
				}
			}
			if !found {
				t.Fatal("no root node (Parent == \"\") found")
			}
		})

		t.Run("node_ids_unique", func(t *testing.T) {
			t.Parallel()
			seen := make(map[string]bool, len(allNodes))
			for _, n := range allNodes {
				if seen[n.Id] {
					t.Errorf("duplicate node ID: %q", n.Id)
				}
				seen[n.Id] = true
			}
		})

		t.Run("node_names_non_empty", func(t *testing.T) {
			t.Parallel()
			for _, n := range allNodes {
				if n.Name == "" {
					t.Errorf("node %q has empty Name", n.Id)
				}
			}
		})

		t.Run("child_nodes_have_valid_parent", func(t *testing.T) {
			t.Parallel()
			nodeIDs := make(map[string]bool, len(allNodes))
			for _, n := range allNodes {
				nodeIDs[n.Id] = true
			}
			for _, n := range allNodes {
				if n.Parent != "" && !nodeIDs[n.Parent] {
					t.Errorf("node %q references non-existent parent %q", n.Id, n.Parent)
				}
			}
		})

		t.Run("edge_refs_valid", func(t *testing.T) {
			t.Parallel()
			nodeIDs := make(map[string]bool, len(allNodes))
			for _, n := range allNodes {
				nodeIDs[n.Id] = true
			}
			for _, e := range allEdges {
				if !nodeIDs[e.Source] {
					t.Errorf("edge %q references non-existent source %q", e.Id, e.Source)
				}
				if !nodeIDs[e.Target] {
					t.Errorf("edge %q references non-existent target %q", e.Id, e.Target)
				}
			}
		})

		t.Run("edge_ids_unique", func(t *testing.T) {
			t.Parallel()
			seen := make(map[string]bool, len(allEdges))
			for _, e := range allEdges {
				if seen[e.Id] {
					t.Errorf("duplicate edge ID: %q", e.Id)
				}
				seen[e.Id] = true
			}
		})

		t.Run("edge_type_non_empty", func(t *testing.T) {
			t.Parallel()
			for _, e := range allEdges {
				if e.Type == "" {
					t.Errorf("edge %q has empty Type", e.Id)
				}
			}
		})

		t.Run("min_counts", func(t *testing.T) {
			t.Parallel()
			if len(allNodes) < opts.MinNodes {
				t.Errorf("got %d nodes, want at least %d", len(allNodes), opts.MinNodes)
			}
			if len(allEdges) < opts.MinEdges {
				t.Errorf("got %d edges, want at least %d", len(allEdges), opts.MinEdges)
			}
		})

		t.Run("child_node_types", func(t *testing.T) {
			t.Parallel()
			typeSet := make(map[string]bool)
			for _, n := range allNodes {
				if n.Parent != "" {
					typeSet[n.Type] = true
				}
			}
			for _, expected := range opts.ChildNodeTypes {
				if !typeSet[expected] {
					t.Errorf("no child node with type %q found", expected)
				}
			}
		})

		if opts.ExpectFKEdges {
			t.Run("fk_edges", func(t *testing.T) {
				t.Parallel()
				var found bool
				for _, e := range allEdges {
					if e.Type == "foreign_key" {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("expected at least one foreign_key edge")
				}
			})
		}

		t.Run("metadata_has_adapter", func(t *testing.T) {
			t.Parallel()
			for _, n := range allNodes {
				if n.Metadata == nil {
					t.Errorf("node %q has nil Metadata", n.Id)
					continue
				}
				if _, ok := n.Metadata["adapter"]; !ok {
					t.Errorf("node %q Metadata missing \"adapter\" key", n.Id)
				}
			}
		})
	})

	t.Run("Health", func(t *testing.T) {
		metrics, err := a.Health()

		t.Run("no_error", func(t *testing.T) {
			t.Parallel()
			if err != nil {
				t.Fatalf("Health returned error: %v", err)
			}
		})

		t.Run("returns_status", func(t *testing.T) {
			t.Parallel()
			status, ok := metrics["status"].(string)
			if !ok {
				t.Fatal("Health metrics missing \"status\" key or not a string")
			}
			valid := map[string]bool{"healthy": true, "degraded": true, "unhealthy": true}
			if !valid[status] {
				t.Errorf("status = %q, want one of healthy/degraded/unhealthy", status)
			}
		})

		t.Run("required_keys", func(t *testing.T) {
			t.Parallel()
			for _, key := range opts.RequiredHealthKeys {
				if _, ok := metrics[key]; !ok {
					t.Errorf("Health metrics missing required key %q", key)
				}
			}
		})
	})

	t.Run("Close", func(t *testing.T) {
		t.Run("no_error", func(t *testing.T) {
			fresh := newAdapter()
			if err := fresh.Connect(config); err != nil {
				t.Fatalf("Connect for Close test failed: %v", err)
			}
			if err := fresh.Close(); err != nil {
				t.Errorf("Close returned error: %v", err)
			}
		})
	})
}
