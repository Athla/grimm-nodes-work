package docker

import (
	"testing"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
)

func TestShouldIgnore(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"ignore true", map[string]string{"graphinfo.ignore": "true"}, true},
		{"ignore True", map[string]string{"graphinfo.ignore": "True"}, true},
		{"ignore TRUE", map[string]string{"graphinfo.ignore": "TRUE"}, true},
		{"ignore false", map[string]string{"graphinfo.ignore": "false"}, false},
		{"no label", map[string]string{}, false},
		{"other labels", map[string]string{"com.docker.compose.service": "foo"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldIgnore(tt.labels)
			if got != tt.want {
				t.Errorf("ShouldIgnore(%v) = %v, want %v", tt.labels, got, tt.want)
			}
		})
	}
}

func TestApplyLabelOverrides_Type(t *testing.T) {
	labels := map[string]string{
		"graphinfo.type": "postgres",
	}
	cfg := adapters.ConnectionConfig{"endpoint": "http://foo:8080"}

	resultType, _ := ApplyLabelOverrides(labels, TypeHTTP, cfg)
	if resultType != TypePostgres {
		t.Errorf("expected type override to postgres, got %s", resultType)
	}
}

func TestApplyLabelOverrides_DSN_Postgres(t *testing.T) {
	labels := map[string]string{
		"graphinfo.dsn": "postgres://custom:pass@host:5432/db",
	}
	cfg := adapters.ConnectionConfig{"dsn": "original"}

	_, resultCfg := ApplyLabelOverrides(labels, TypePostgres, cfg)
	if resultCfg["dsn"] != "postgres://custom:pass@host:5432/db" {
		t.Errorf("expected DSN override, got %v", resultCfg["dsn"])
	}
}

func TestApplyLabelOverrides_DSN_MongoDB(t *testing.T) {
	labels := map[string]string{
		"graphinfo.dsn": "mongodb://custom:pass@host:27017",
	}
	cfg := adapters.ConnectionConfig{"uri": "original"}

	_, resultCfg := ApplyLabelOverrides(labels, TypeMongoDB, cfg)
	if resultCfg["uri"] != "mongodb://custom:pass@host:27017" {
		t.Errorf("expected URI override, got %v", resultCfg["uri"])
	}
}

func TestApplyLabelOverrides_NodeType(t *testing.T) {
	labels := map[string]string{
		"graphinfo.node-type": "gateway",
	}
	cfg := adapters.ConnectionConfig{}

	_, resultCfg := ApplyLabelOverrides(labels, TypeHTTP, cfg)
	if resultCfg["node_type"] != "gateway" {
		t.Errorf("expected node_type override, got %v", resultCfg["node_type"])
	}
}

func TestApplyLabelOverrides_Name(t *testing.T) {
	labels := map[string]string{
		"graphinfo.name": "custom-name",
	}
	cfg := adapters.ConnectionConfig{}

	_, resultCfg := ApplyLabelOverrides(labels, TypeHTTP, cfg)
	if resultCfg["name"] != "custom-name" {
		t.Errorf("expected name override, got %v", resultCfg["name"])
	}
}

func TestApplyLabelOverrides_NoLabels(t *testing.T) {
	cfg := adapters.ConnectionConfig{"dsn": "original"}

	resultType, resultCfg := ApplyLabelOverrides(map[string]string{}, TypePostgres, cfg)
	if resultType != TypePostgres {
		t.Errorf("expected type unchanged, got %s", resultType)
	}
	if resultCfg["dsn"] != "original" {
		t.Errorf("expected config unchanged, got %v", resultCfg["dsn"])
	}
}
