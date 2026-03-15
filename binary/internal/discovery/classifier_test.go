package discovery

import "testing"

func TestClassifyImage(t *testing.T) {
	tests := []struct {
		image string
		want  ServiceType
	}{
		// Postgres
		{"postgres:17-alpine", TypePostgres},
		{"postgres", TypePostgres},
		{"docker.io/library/postgres:16", TypePostgres},
		{"ghcr.io/org/postgres:latest", TypePostgres},

		// Postgres excludes
		{"postgres-exporter:latest", TypeHTTP},
		{"pgadmin4:latest", TypeHTTP},
		{"pgbouncer:latest", TypeHTTP},
		{"postgres-backup:latest", TypeHTTP},

		// MongoDB
		{"mongo:7", TypeMongoDB},
		{"mongo:latest", TypeMongoDB},
		{"docker.io/library/mongo:6", TypeMongoDB},

		// MongoDB excludes
		{"mongo-express:latest", TypeHTTP},
		{"mongodb-exporter:latest", TypeHTTP},

		// MinIO / S3
		{"minio/minio:latest", TypeS3},
		{"minio:latest", TypeS3},
		{"quay.io/minio/minio:RELEASE.2024", TypeS3},

		// MinIO excludes
		{"minio/mc:latest", TypeHTTP},

		// Redis
		{"redis:7-alpine", TypeRedis},
		{"redis:latest", TypeRedis},
		{"docker.io/library/redis:6", TypeRedis},

		// Redis excludes
		{"redis-exporter:latest", TypeHTTP},
		{"redis-commander:latest", TypeHTTP},
		{"redis-insight:latest", TypeHTTP},

		// HTTP fallback
		{"nginx:latest", TypeHTTP},
		{"my-custom-app:v1.0", TypeHTTP},
		{"node:20-alpine", TypeHTTP},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := ClassifyImage(tt.image)
			if got != tt.want {
				t.Errorf("ClassifyImage(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

func TestDefaultPortForType(t *testing.T) {
	tests := []struct {
		svcType ServiceType
		want    uint16
	}{
		{TypePostgres, 5432},
		{TypeMongoDB, 27017},
		{TypeS3, 9000},
		{TypeRedis, 6379},
		{TypeHTTP, 8080},
		{"unknown", 8080},
	}

	for _, tt := range tests {
		t.Run(string(tt.svcType), func(t *testing.T) {
			got := DefaultPortForType(tt.svcType)
			if got != tt.want {
				t.Errorf("DefaultPortForType(%q) = %d, want %d", tt.svcType, got, tt.want)
			}
		})
	}
}

func TestNormalizeImage(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"postgres:17-alpine", "postgres"},
		{"docker.io/library/postgres:16", "postgres"},
		{"ghcr.io/org/my-app:v1.0", "my-app"},
		{"minio/minio:latest", "minio"},
		{"redis", "redis"},
		{"POSTGRES:Latest", "postgres"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeImage(tt.input)
			if got != tt.want {
				t.Errorf("normalizeImage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
