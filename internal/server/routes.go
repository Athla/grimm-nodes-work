package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gorilla/mux"

	"github.com/guilherme-grimm/graph-go/internal/graph/health"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := mux.NewRouter()

	r.Use(s.corsMiddleware)

	r.HandleFunc("/health", s.healthHandler)

	// API routes
	r.HandleFunc("/api/graph", s.graphHandler).Methods("GET")
	r.HandleFunc("/api/node/{id}", s.nodeHandler).Methods("GET")
	r.HandleFunc("/api/health", s.apiHealthHandler).Methods("GET")

	r.HandleFunc("/websocket", s.websocketHandler)

	return r
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "false")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) graphHandler(w http.ResponseWriter, r *http.Request) {
	g, err := s.registry.DiscoverAll()
	if err != nil {
		log.Printf("graphHandler: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"data": g}); err != nil {
		log.Printf("graphHandler: encode error: %v", err)
	}
}

func (s *Server) nodeHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	g, err := s.registry.DiscoverAll()
	if err != nil {
		log.Printf("nodeHandler: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	for _, node := range g.Nodes {
		if node.Id == id {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{"data": node}); err != nil {
				log.Printf("nodeHandler: encode error: %v", err)
			}
			return
		}
	}

	http.Error(w, "node not found", http.StatusNotFound)
}

func (s *Server) apiHealthHandler(w http.ResponseWriter, r *http.Request) {
	status := "ok"

	metrics := s.registry.HealthAll()
	for _, m := range metrics {
		if m.Status == health.Unhealthy {
			status = "error"
			break
		}
		if m.Status == health.Degraded {
			status = "degraded"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":    status,
		"timestamp": time.Now().Format(time.RFC3339),
	}); err != nil {
		log.Printf("apiHealthHandler: encode error: %v", err)
	}
}

func (s *Server) websocketHandler(w http.ResponseWriter, r *http.Request) {
	origins := os.Getenv("WS_ALLOWED_ORIGINS")
	if origins == "" {
		origins = "*"
	}
	patterns := strings.Split(origins, ",")
	for i := range patterns {
		patterns[i] = strings.TrimSpace(patterns[i])
	}

	socket, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: patterns,
	})
	if err != nil {
		log.Printf("could not open websocket: %v", err)
		return
	}

	defer socket.Close(websocket.StatusGoingAway, "server closing websocket")

	ctx := r.Context()
	socketCtx := socket.CloseRead(ctx)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var prevNodeKey string

	for {
		// Get the current graph to map adapter health to actual node IDs
		g, err := s.registry.DiscoverAll()
		if err != nil {
			log.Printf("websocket: discover error: %v", err)
		}

		// Detect graph topology changes (nodes added/removed)
		if g != nil {
			ids := make([]string, 0, len(g.Nodes))
			for _, node := range g.Nodes {
				ids = append(ids, node.Id)
			}
			sort.Strings(ids)
			currentKey := strings.Join(ids, ",")

			if prevNodeKey != "" && currentKey != prevNodeKey {
				msg := map[string]any{
					"type":    "graph_update",
					"payload": map[string]any{},
				}
				data, jsonErr := json.Marshal(msg)
				if jsonErr == nil {
					if err := socket.Write(socketCtx, websocket.MessageText, data); err != nil {
						return
					}
				}
			}
			prevNodeKey = currentKey
		}

		metrics := s.registry.HealthAll()

		// Build a lookup from adapter name to its health status.
		adapterHealth := make(map[string]string, len(metrics))
		for _, m := range metrics {
			adapterHealth[m.NodeID] = string(m.Status)
		}

		// Send a health update for each node. Adapter-owned nodes get their
		// health via the adapter lookup; topology nodes (e.g. Kubernetes
		// resources) carry health directly on Node.Health.
		if g != nil {
			for _, node := range g.Nodes {
				adapterName, _ := node.Metadata["adapter"].(string)
				healthStr, ok := adapterHealth[adapterName]
				if !ok {
					if node.Health == "" {
						continue
					}
					healthStr = node.Health
				}
				msg := map[string]any{
					"type": "health_update",
					"payload": map[string]any{
						"nodeId": node.Id,
						"health": healthStr,
					},
				}
				data, jsonErr := json.Marshal(msg)
				if jsonErr != nil {
					continue
				}
				if err := socket.Write(socketCtx, websocket.MessageText, data); err != nil {
					return
				}
			}
		}

		select {
		case <-socketCtx.Done():
			return
		case <-ticker.C:
		}
	}
}
