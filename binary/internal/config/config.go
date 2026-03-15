package config

import (
	"fmt"
	"log"
	"os"

	"binary/internal/adapters"

	"gopkg.in/yaml.v3"
)

type DockerConfig struct {
	Enabled      *bool    `yaml:"enabled,omitempty"`      // nil = auto-detect
	Socket       string   `yaml:"socket,omitempty"`       // default: /var/run/docker.sock
	Network      string   `yaml:"network,omitempty"`      // limit to specific network
	IgnoreImages []string `yaml:"ignore_images,omitempty"`
}

type Config struct {
	Docker      DockerConfig      `yaml:"docker,omitempty"`
	Connections []ConnectionEntry `yaml:"connections"`
}

type DependsOnEntry struct {
	Target string `yaml:"target"`
	Label  string `yaml:"label,omitempty"`
}

type ConnectionEntry struct {
	Name            string           `yaml:"name"`
	Type            string           `yaml:"type"`
	DSN             string           `yaml:"dsn,omitempty"`
	URI             string           `yaml:"uri,omitempty"`
	Region          string           `yaml:"region,omitempty"`
	Endpoint        string           `yaml:"endpoint,omitempty"`
	AccessKeyID     string           `yaml:"access_key_id,omitempty"`
	SecretAccessKey string           `yaml:"secret_access_key,omitempty"`
	HealthPath      string           `yaml:"health_path,omitempty"`
	NodeType        string           `yaml:"node_type,omitempty"`
	DependsOn       []DependsOnEntry `yaml:"depends_on,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: failed to read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: failed to parse %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *Config) Validate() error {
	for i, conn := range cfg.Connections {
		if conn.Name == "" {
			return fmt.Errorf("config: connections[%d]: 'name' is required", i)
		}
		if conn.Type == "" {
			return fmt.Errorf("config: connections[%d] (%s): 'type' is required", i, conn.Name)
		}
		switch conn.Type {
		case "postgres":
			if conn.DSN == "" {
				return fmt.Errorf("config: connections[%d] (%s): 'dsn' is required for postgres", i, conn.Name)
			}
		case "mongodb":
			if conn.URI == "" {
				return fmt.Errorf("config: connections[%d] (%s): 'uri' is required for mongodb", i, conn.Name)
			}
		case "s3", "http":
			if conn.Endpoint == "" {
				return fmt.Errorf("config: connections[%d] (%s): 'endpoint' is required for %s", i, conn.Name, conn.Type)
			}
		case "redis":
			// Valid type, adapter not yet implemented
		default:
			return fmt.Errorf("config: connections[%d] (%s): unknown type %q", i, conn.Name, conn.Type)
		}
	}
	return nil
}

func (e *ConnectionEntry) ToConnectionConfig() adapters.ConnectionConfig {
	cfg := adapters.ConnectionConfig{}

	switch e.Type {
	case "postgres":
		if e.DSN != "" {
			cfg["dsn"] = e.DSN
		}
	case "mongodb":
		if e.URI != "" {
			cfg["uri"] = e.URI
		}
	case "s3":
		if e.Region != "" {
			cfg["region"] = e.Region
		}
		if e.Endpoint != "" {
			cfg["endpoint"] = e.Endpoint
		}
		if e.AccessKeyID != "" {
			cfg["access_key_id"] = e.AccessKeyID
		}
		if e.SecretAccessKey != "" {
			cfg["secret_access_key"] = e.SecretAccessKey
		}
	case "http":
		if e.Endpoint != "" {
			cfg["endpoint"] = e.Endpoint
		}
		if e.HealthPath != "" {
			cfg["health_path"] = e.HealthPath
		}
		if e.NodeType != "" {
			cfg["node_type"] = e.NodeType
		}
		cfg["name"] = e.Name
		if len(e.DependsOn) > 0 {
			deps := make([]map[string]string, len(e.DependsOn))
			for i, d := range e.DependsOn {
				deps[i] = map[string]string{"target": d.Target, "label": d.Label}
			}
			cfg["depends_on"] = deps
		}
	default:
		log.Printf("WARNING: ToConnectionConfig: unrecognized type %q for %q", e.Type, e.Name)
	}

	return cfg
}
