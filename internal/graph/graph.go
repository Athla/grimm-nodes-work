package graph

import (
	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

type Graph struct {
	Nodes []nodes.Node `json:"nodes"`
	Edges []edges.Edge `json:"edges"`
}
