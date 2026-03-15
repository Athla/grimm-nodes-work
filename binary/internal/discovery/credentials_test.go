package discovery

import (
	"strings"
	"testing"
)

func TestExtractCredentials_Postgres(t *testing.T) {
	env := []string{
		"POSTGRES_USER=myuser",
		"POSTGRES_PASSWORD=mypass",
		"POSTGRES_DB=mydb",
	}

	cfg := ExtractCredentials(TypePostgres, env, "postgres-host", "postgres")

	dsn, ok := cfg["dsn"].(string)
	if !ok {
		t.Fatal("expected dsn string in config")
	}
	if !strings.Contains(dsn, "myuser:mypass@postgres-host:5432/mydb") {
		t.Errorf("unexpected DSN: %s", dsn)
	}
}

func TestExtractCredentials_PostgresDefaults(t *testing.T) {
	cfg := ExtractCredentials(TypePostgres, nil, "pg", "postgres")

	dsn := cfg["dsn"].(string)
	if !strings.Contains(dsn, "postgres:@pg:5432/postgres") {
		t.Errorf("expected default user/db in DSN, got: %s", dsn)
	}
}

func TestExtractCredentials_MongoDB(t *testing.T) {
	env := []string{
		"MONGO_INITDB_ROOT_USERNAME=admin",
		"MONGO_INITDB_ROOT_PASSWORD=secret",
	}

	cfg := ExtractCredentials(TypeMongoDB, env, "mongo-host", "mongo")

	uri, ok := cfg["uri"].(string)
	if !ok {
		t.Fatal("expected uri string in config")
	}
	if !strings.Contains(uri, "admin:secret@mongo-host:27017") {
		t.Errorf("unexpected URI: %s", uri)
	}
}

func TestExtractCredentials_MongoDBNoAuth(t *testing.T) {
	cfg := ExtractCredentials(TypeMongoDB, nil, "mongo-host", "mongo")

	uri := cfg["uri"].(string)
	if !strings.Contains(uri, "mongodb://mongo-host:27017") {
		t.Errorf("expected no-auth URI, got: %s", uri)
	}
	if strings.Contains(uri, "@") {
		t.Errorf("expected no @ in no-auth URI, got: %s", uri)
	}
}

func TestExtractCredentials_S3(t *testing.T) {
	env := []string{
		"MINIO_ROOT_USER=myadmin",
		"MINIO_ROOT_PASSWORD=myadminpass",
	}

	cfg := ExtractCredentials(TypeS3, env, "minio-host", "minio")

	if cfg["access_key_id"] != "myadmin" {
		t.Errorf("unexpected access_key_id: %v", cfg["access_key_id"])
	}
	if cfg["secret_access_key"] != "myadminpass" {
		t.Errorf("unexpected secret_access_key: %v", cfg["secret_access_key"])
	}
	endpoint := cfg["endpoint"].(string)
	if !strings.Contains(endpoint, "minio-host:9000") {
		t.Errorf("unexpected endpoint: %s", endpoint)
	}
}

func TestExtractCredentials_Redis(t *testing.T) {
	env := []string{"REDIS_PASSWORD=redispass"}
	cfg := ExtractCredentials(TypeRedis, env, "redis-host", "redis")

	if cfg["host"] != "redis-host" {
		t.Errorf("unexpected host: %v", cfg["host"])
	}
	if cfg["password"] != "redispass" {
		t.Errorf("unexpected password: %v", cfg["password"])
	}
}

func TestExtractCredentials_HTTP(t *testing.T) {
	env := []string{"PORT=3000"}
	cfg := ExtractCredentials(TypeHTTP, env, "myapp", "my-service")

	endpoint := cfg["endpoint"].(string)
	if !strings.Contains(endpoint, "myapp:3000") {
		t.Errorf("unexpected endpoint: %s", endpoint)
	}
	if cfg["name"] != "my-service" {
		t.Errorf("unexpected name: %v", cfg["name"])
	}
}

func TestExtractCredentials_HTTPDefaultPort(t *testing.T) {
	cfg := ExtractCredentials(TypeHTTP, nil, "myapp", "my-service")

	endpoint := cfg["endpoint"].(string)
	if !strings.Contains(endpoint, "myapp:8080") {
		t.Errorf("expected default port 8080, got: %s", endpoint)
	}
}

func TestParseEnvVars(t *testing.T) {
	env := []string{
		"KEY1=value1",
		"KEY2=value2=with=equals",
		"EMPTY=",
		"NOEQUALS",
	}

	m := parseEnvVars(env)

	if m["KEY1"] != "value1" {
		t.Errorf("KEY1 = %q, want %q", m["KEY1"], "value1")
	}
	if m["KEY2"] != "value2=with=equals" {
		t.Errorf("KEY2 = %q, want %q", m["KEY2"], "value2=with=equals")
	}
	if m["EMPTY"] != "" {
		t.Errorf("EMPTY = %q, want empty", m["EMPTY"])
	}
	if _, ok := m["NOEQUALS"]; ok {
		t.Error("NOEQUALS should not be in map")
	}
}
