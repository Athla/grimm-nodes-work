package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
	"github.com/guilherme-grimm/graph-go/internal/config"
	"github.com/guilherme-grimm/graph-go/internal/discovery"
	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"

	// Self-registering adapters
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/elasticsearch"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/http"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/mongodb"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/mysql"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/postgres"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/redis"
	_ "github.com/guilherme-grimm/graph-go/internal/adapters/s3"
)

type Server struct {
	port           int
	allowedOrigins []string
	registry       adapters.Registry
	logger         *zap.SugaredLogger
}

// NewServer returns the HTTP server and a cleanup function that should
// be called during graceful shutdown to close adapter connections.
func NewServer(cfg *config.Config, logger *zap.SugaredLogger) (*http.Server, func()) {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	allowedOrigins := cfg.Server.AllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:3000", "http://localhost:5173"}
	}

	reg, discoverers, regCleanup := BuildRegistry(cfg, logger)

	// Start a Watch() goroutine for each discoverer. Topology-oriented
	// discoverers refresh their topology on each callback; adapter-oriented
	// ones just invalidate the cache so next DiscoverAll re-queries adapters.
	watchCtx, watchCancel := context.WithCancel(context.Background())
	for _, d := range discoverers {
		d := d
		onChange := buildOnChange(watchCtx, d, reg, logger)
		go func() {
			if err := d.Watch(watchCtx, onChange); err != nil {
				logger.Warnw("discoverer watch stopped", "discoverer", d.Name(), "err", err)
			}
		}()
		logger.Infow("discoverer watch started", "discoverer", d.Name())
	}

	s := &Server{
		port:           port,
		allowedOrigins: allowedOrigins,
		registry:       reg,
		logger:         logger,
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      s.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	cleanup := func() {
		watchCancel()
		regCleanup()
	}

	return server, cleanup
}

// buildOnChange returns the callback invoked on discoverer events. For
// discoverers that produce topology ServiceInfo, it re-reads their snapshot
// and replaces the registry topology before invalidating caches. For pure
// adapter discoverers, it just invalidates the cache.
func buildOnChange(ctx context.Context, d discovery.Discoverer, reg adapters.Registry, logger *zap.SugaredLogger) func() {
	return func() {
		fresh, err := d.Discover(ctx)
		if err != nil {
			logger.Warnw("discoverer refresh failed", "discoverer", d.Name(), "err", err)
			reg.InvalidateCache()
			return
		}
		var hasTopology bool
		var topoNodes []nodes.Node
		var topoEdges []edges.Edge
		for _, svc := range fresh {
			if len(svc.Nodes) > 0 || len(svc.Edges) > 0 {
				hasTopology = true
				topoNodes = append(topoNodes, svc.Nodes...)
				topoEdges = append(topoEdges, svc.Edges...)
			}
		}
		if hasTopology {
			reg.SetTopology(d.Name(), topoNodes, topoEdges)
			return
		}
		applyServices(reg, fresh, logger)
	}
}
