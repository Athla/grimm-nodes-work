package discovery

import (
	"testing"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

func TestMergeWithYAML(t *testing.T) {
	tests := []struct {
		name       string
		discovered []ServiceInfo
		yaml       []YAMLEntry
		wantLen    int
		check      func(t *testing.T, result []ServiceInfo)
	}{
		{
			name: "yaml overrides type and config",
			discovered: []ServiceInfo{
				{Name: "mydb", Type: "postgres", Source: "docker", Config: adapters.ConnectionConfig{"host": "old"}},
			},
			yaml: []YAMLEntry{
				{Name: "mydb", Type: "mysql", Config: adapters.ConnectionConfig{"host": "new"}},
			},
			wantLen: 1,
			check: func(t *testing.T, result []ServiceInfo) {
				if result[0].Type != "mysql" {
					t.Errorf("expected type 'mysql', got %q", result[0].Type)
				}
				if result[0].Config["host"] != "new" {
					t.Errorf("expected config host 'new', got %v", result[0].Config["host"])
				}
				if result[0].Source != "docker" {
					t.Errorf("expected source preserved as 'docker', got %q", result[0].Source)
				}
			},
		},
		{
			name: "preserves discovered metadata and topology",
			discovered: []ServiceInfo{
				{
					Name:     "k8s-svc",
					Type:     "k8s_service",
					Source:   "kubernetes",
					Nodes:    []nodes.Node{{Id: "n1"}},
					Health:   "healthy",
					Metadata: map[string]any{"ns": "default"},
				},
			},
			yaml: []YAMLEntry{
				{Name: "k8s-svc", Type: "service", Config: adapters.ConnectionConfig{"port": "8080"}},
			},
			wantLen: 1,
			check: func(t *testing.T, result []ServiceInfo) {
				if len(result[0].Nodes) != 1 {
					t.Errorf("expected nodes preserved, got %d", len(result[0].Nodes))
				}
				if result[0].Health != "healthy" {
					t.Errorf("expected health preserved, got %q", result[0].Health)
				}
				if result[0].Metadata["ns"] != "default" {
					t.Error("expected metadata preserved")
				}
			},
		},
		{
			name:       "unmatched yaml appended with source=yaml",
			discovered: []ServiceInfo{},
			yaml: []YAMLEntry{
				{Name: "extra", Type: "redis", Config: adapters.ConnectionConfig{"host": "redis"}},
			},
			wantLen: 1,
			check: func(t *testing.T, result []ServiceInfo) {
				if result[0].Source != SourceYAML {
					t.Errorf("expected source %q, got %q", SourceYAML, result[0].Source)
				}
				if result[0].Name != "extra" {
					t.Errorf("expected name 'extra', got %q", result[0].Name)
				}
			},
		},
		{
			name: "unmatched discovered kept as-is",
			discovered: []ServiceInfo{
				{Name: "mydb", Type: "postgres", Source: "docker"},
			},
			yaml:    []YAMLEntry{},
			wantLen: 1,
			check: func(t *testing.T, result []ServiceInfo) {
				if result[0].Name != "mydb" || result[0].Type != "postgres" {
					t.Errorf("expected unchanged discovered service")
				}
			},
		},
		{
			name:       "empty both returns empty",
			discovered: []ServiceInfo{},
			yaml:       []YAMLEntry{},
			wantLen:    0,
			check:      func(t *testing.T, result []ServiceInfo) {},
		},
		{
			name: "mixed match and append",
			discovered: []ServiceInfo{
				{Name: "db1", Type: "postgres", Source: "docker"},
				{Name: "db2", Type: "mysql", Source: "docker"},
			},
			yaml: []YAMLEntry{
				{Name: "db1", Type: "pg-override", Config: adapters.ConnectionConfig{"dsn": "x"}},
				{Name: "newcache", Type: "redis", Config: adapters.ConnectionConfig{"host": "r"}},
			},
			wantLen: 3,
			check: func(t *testing.T, result []ServiceInfo) {
				// db1 overridden
				if result[0].Type != "pg-override" {
					t.Errorf("expected db1 overridden, got %q", result[0].Type)
				}
				// db2 untouched
				if result[1].Name != "db2" || result[1].Type != "mysql" {
					t.Errorf("expected db2 unchanged")
				}
				// newcache appended
				if result[2].Name != "newcache" || result[2].Source != SourceYAML {
					t.Errorf("expected newcache appended with source yaml")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeWithYAML(tt.discovered, tt.yaml)
			if len(result) != tt.wantLen {
				t.Fatalf("expected %d results, got %d", tt.wantLen, len(result))
			}
			tt.check(t, result)
		})
	}
}
