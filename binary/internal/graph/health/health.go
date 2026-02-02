package health

import "time"

type Status string

const (
	Healthy   Status = "healthy"
	Degraded  Status = "degraded"
	Unhealthy Status = "unhealthy"
	Unkown    Status = "unkown"
)

type HealthMetrics struct {
	NodeID    string         `json:"node_id"`
	Status    Status         `json:"status"`
	Metrics   map[string]any `json:"metrics"`
	Timestamp time.Time      `json:"timestamp"`
}
