package docker

import (
	"strings"

	"binary/internal/adapters"
)

const (
	LabelIgnore   = "graphinfo.ignore"
	LabelType     = "graphinfo.type"
	LabelDSN      = "graphinfo.dsn"
	LabelNodeType = "graphinfo.node-type"
	LabelName     = "graphinfo.name"
)

// ShouldIgnore returns true if the container has graphinfo.ignore=true.
func ShouldIgnore(labels map[string]string) bool {
	v, ok := labels[LabelIgnore]
	return ok && strings.EqualFold(v, "true")
}

// ApplyLabelOverrides applies graphinfo.* label overrides to the detected
// service type and connection config. Returns the (possibly modified) type
// and config.
func ApplyLabelOverrides(labels map[string]string, detectedType ServiceType, config adapters.ConnectionConfig) (ServiceType, adapters.ConnectionConfig) {
	resultType := detectedType

	if v, ok := labels[LabelType]; ok && v != "" {
		resultType = ServiceType(v)
	}

	if v, ok := labels[LabelDSN]; ok && v != "" {
		switch resultType {
		case TypePostgres:
			config["dsn"] = v
		case TypeMongoDB:
			config["uri"] = v
		default:
			config["dsn"] = v
		}
	}

	if v, ok := labels[LabelNodeType]; ok && v != "" {
		config["node_type"] = v
	}

	if v, ok := labels[LabelName]; ok && v != "" {
		config["name"] = v
	}

	return resultType, config
}
