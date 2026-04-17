package docker

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
)

// ExtractCredentials builds an adapters.ConnectionConfig from container
// environment variables, based on the detected service type.
func ExtractCredentials(svcType ServiceType, envVars []string, containerHost string, containerName string) adapters.ConnectionConfig {
	env := parseEnvVars(envVars)

	switch svcType {
	case TypePostgres:
		return extractPostgresCredentials(env, containerHost)
	case TypeMongoDB:
		return extractMongoCredentials(env, containerHost)
	case TypeS3:
		return extractS3Credentials(env, containerHost)
	case TypeRedis:
		return extractRedisCredentials(env, containerHost)
	case TypeMySQL:
		return extractMySQLCredentials(env, containerHost)
	case TypeElasticsearch:
		return extractElasticsearchCredentials(env, containerHost)
	case TypeHTTP:
		return extractHTTPCredentials(env, containerHost, containerName)
	default:
		return adapters.ConnectionConfig{}
	}
}

func parseEnvVars(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if ok {
			m[k] = v
		}
	}
	return m
}

func extractPostgresCredentials(env map[string]string, host string) adapters.ConnectionConfig {
	user := envOrDefault(env, "POSTGRES_USER", "postgres")
	pass := envOrDefault(env, "POSTGRES_PASSWORD", "")
	db := envOrDefault(env, "POSTGRES_DB", user)

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(user), url.QueryEscape(pass), host, DefaultPortForType(TypePostgres), db)

	return adapters.ConnectionConfig{"dsn": dsn}
}

func extractMongoCredentials(env map[string]string, host string) adapters.ConnectionConfig {
	user := envOrDefault(env, "MONGO_INITDB_ROOT_USERNAME", "")
	pass := envOrDefault(env, "MONGO_INITDB_ROOT_PASSWORD", "")

	var uri string
	if user != "" && pass != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s:%d",
			url.QueryEscape(user), url.QueryEscape(pass), host, DefaultPortForType(TypeMongoDB))
	} else {
		uri = fmt.Sprintf("mongodb://%s:%d",
			host, DefaultPortForType(TypeMongoDB))
	}

	return adapters.ConnectionConfig{"uri": uri}
}

func extractS3Credentials(env map[string]string, host string) adapters.ConnectionConfig {
	accessKey := envOrDefault(env, "MINIO_ROOT_USER", "minioadmin")
	secretKey := envOrDefault(env, "MINIO_ROOT_PASSWORD", "minioadmin")
	endpoint := fmt.Sprintf("http://%s:%d", host, DefaultPortForType(TypeS3))

	return adapters.ConnectionConfig{
		"endpoint":          endpoint,
		"access_key_id":     accessKey,
		"secret_access_key": secretKey,
		"region":            "us-east-1",
	}
}

func extractRedisCredentials(env map[string]string, host string) adapters.ConnectionConfig {
	pass := envOrDefault(env, "REDIS_PASSWORD", "")
	port := DefaultPortForType(TypeRedis)

	cfg := adapters.ConnectionConfig{
		"host": host,
		"port": port,
	}
	if pass != "" {
		cfg["password"] = pass
	}
	return cfg
}

func extractHTTPCredentials(env map[string]string, host string, containerName string) adapters.ConnectionConfig {
	port := envOrDefault(env, "PORT", "8080")
	endpoint := fmt.Sprintf("http://%s:%s", host, port)

	return adapters.ConnectionConfig{
		"endpoint":    endpoint,
		"health_path": "/health",
		"name":        containerName,
	}
}

func extractMySQLCredentials(env map[string]string, host string) adapters.ConnectionConfig {
	user := envOrDefault(env, "MYSQL_USER", "root")
	pass := envOrDefault(env, "MYSQL_ROOT_PASSWORD", "")
	if pass == "" {
		pass = envOrDefault(env, "MYSQL_PASSWORD", "")
	}
	db := envOrDefault(env, "MYSQL_DATABASE", "")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		user, url.QueryEscape(pass), host, DefaultPortForType(TypeMySQL), db)

	return adapters.ConnectionConfig{"dsn": dsn}
}

func extractElasticsearchCredentials(env map[string]string, host string) adapters.ConnectionConfig {
	endpoint := fmt.Sprintf("http://%s:%d", host, DefaultPortForType(TypeElasticsearch))

	cfg := adapters.ConnectionConfig{
		"endpoint": endpoint,
	}

	user := envOrDefault(env, "ELASTIC_USERNAME", "elastic")
	pass := envOrDefault(env, "ELASTIC_PASSWORD", "")
	if pass != "" {
		cfg["username"] = user
		cfg["password"] = pass
	}

	return cfg
}

func envOrDefault(env map[string]string, key, fallback string) string {
	if v, ok := env[key]; ok && v != "" {
		return v
	}
	return fallback
}
