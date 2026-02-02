package graph

import (
	"binary/internal/graph/edges"
	"binary/internal/graph/nodes"
)

type Graph struct {
	Nodes []nodes.Node `json:"nodes"`
	Edges []edges.Edge `json:"edges"`
}
