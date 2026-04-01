package discovery

import "strings"

// ServiceType represents a recognized infrastructure service type.
type ServiceType string

const (
	TypePostgres      ServiceType = "postgres"
	TypeMongoDB       ServiceType = "mongodb"
	TypeS3            ServiceType = "s3"
	TypeRedis         ServiceType = "redis"
	TypeMySQL         ServiceType = "mysql"
	TypeElasticsearch ServiceType = "elasticsearch"
	TypeHTTP          ServiceType = "http"
)

type classificationRule struct {
	pattern  string
	excludes []string
	svcType  ServiceType
	port     uint16
}

var classificationRules = []classificationRule{
	{
		pattern:  "postgres",
		excludes: []string{"exporter", "backup", "pgadmin", "pgbouncer"},
		svcType:  TypePostgres,
		port:     5432,
	},
	{
		pattern:  "mongo",
		excludes: []string{"express", "exporter"},
		svcType:  TypeMongoDB,
		port:     27017,
	},
	{
		pattern:  "minio",
		excludes: []string{"mc", "client"},
		svcType:  TypeS3,
		port:     9000,
	},
	{
		pattern:  "redis",
		excludes: []string{"exporter", "commander", "insight"},
		svcType:  TypeRedis,
		port:     6379,
	},
	{
		pattern:  "mysql",
		excludes: []string{"exporter", "proxy", "router", "phpmyadmin"},
		svcType:  TypeMySQL,
		port:     3306,
	},
	{
		pattern:  "mariadb",
		excludes: []string{"exporter"},
		svcType:  TypeMySQL,
		port:     3306,
	},
	{
		pattern:  "elasticsearch",
		excludes: []string{"exporter", "kibana", "curator"},
		svcType:  TypeElasticsearch,
		port:     9200,
	},
}

// ClassifyImage determines the ServiceType for a Docker image string.
// It strips the registry prefix and tag, then matches against ordered rules.
func ClassifyImage(image string) ServiceType {
	normalized := normalizeImage(image)

	for _, rule := range classificationRules {
		if !strings.Contains(normalized, rule.pattern) {
			continue
		}
		excluded := false
		for _, exc := range rule.excludes {
			if strings.Contains(normalized, exc) {
				excluded = true
				break
			}
		}
		if !excluded {
			return rule.svcType
		}
	}

	return TypeHTTP
}

// DefaultPortForType returns the standard port for a given service type.
func DefaultPortForType(svcType ServiceType) uint16 {
	for _, rule := range classificationRules {
		if rule.svcType == svcType {
			return rule.port
		}
	}
	return 8080
}

// normalizeImage strips the registry prefix and tag from an image string
// so classification only operates on the image name.
// e.g. "docker.io/library/postgres:17-alpine" -> "postgres"
func normalizeImage(image string) string {
	// Remove tag or digest
	if idx := strings.LastIndex(image, ":"); idx != -1 {
		// Make sure we don't strip port from registry (check for /)
		afterColon := image[idx+1:]
		if !strings.Contains(afterColon, "/") {
			image = image[:idx]
		}
	}
	if idx := strings.LastIndex(image, "@"); idx != -1 {
		image = image[:idx]
	}

	// Take last path component (strips registry/org prefixes)
	if idx := strings.LastIndex(image, "/"); idx != -1 {
		image = image[idx+1:]
	}

	return strings.ToLower(image)
}
