package nodes

import "github.com/google/uuid"

type NodeType string

const (
	TypePostgres NodeType = "postgres"
	TypeMongodb  NodeType = "mongodb"
	TypeRedis    NodeType = "redis"
	TypeS3       NodeType = "s3"
)

type Node struct {
	Id       uuid.UUID
	Type     string
	Name     string
	Parent   string
	Metadata map[string]any
}
