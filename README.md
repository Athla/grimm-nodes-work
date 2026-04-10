<div align="center">

# graph-go

**See your infrastructure. Zero Config.**

Point graph-info at your stack and get a live, interactive map of every database, table, service, and storage bucket — with real-time health monitoring.

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
![graph-info demo](./docs/GraphGo.png)

</div>

---

graph-info **auto-discovers** your infrastructure by connecting to the Docker daemon, inspecting running containers, and probing databases and storage services. No manual inventory needed — it builds the graph for you.

| Capability | Details |
|---|---|
| **Auto-discovery** | Detects infrastructure from Docker containers and Kubernetes clusters — no manual inventory needed |
| **Kubernetes** | Namespaces, Deployments, StatefulSets, DaemonSets, Pods, Services — with informer-based real-time watching |
| **Docker** | Classifies running containers, extracts credentials, watches Docker events for live topology changes |
| **PostgreSQL** | Tables, foreign key relationships, schema topology |
| **MongoDB** | Databases and collections |
| **MySQL** | Tables, foreign key relationships |
| **Redis** | Keyspaces and key distribution |
| **Elasticsearch** | Indices, cluster health, shard status |
| **S3 / MinIO** | Buckets and top-level prefixes |
| **HTTP services** | Health endpoints, dependency mapping between services |
| **Real-time health** | WebSocket-powered live status updates every 5 seconds |
| **Interactive graph** | Swimlane layout, namespace group containers, pan/zoom, filter by type/health, search nodes |

---

## Installation

### Docker (recommended)

Point graph-info at your existing infrastructure — no config file needed. Auto-discovery handles the rest:

```bash
# Backend — mount the Docker socket for auto-discovery
docker run -d -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/guilherme-grimm/graph-go-backend:latest

# Frontend
docker run -d -p 3000:80 \
  ghcr.io/guilherme-grimm/graph-go-frontend:latest
```

Open `http://localhost:3000` and your infrastructure graph will appear automatically.

To add connections that aren't in Docker (e.g., remote databases, external S3), mount a config file:

```bash
docker run -d -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v ./conf/config.yaml:/app/conf/config.yaml \
  ghcr.io/guilherme-grimm/graph-go-backend:latest
```

### Pre-built binaries

Download the latest release for your platform from [GitHub Releases](https://github.com/guilherme-grimm/graph-go/releases):

```bash
# Example: Linux amd64
curl -sL https://github.com/guilherme-grimm/graph-go/releases/latest/download/graph-info_linux_amd64.tar.gz | tar xz
./graph-info
```

The backend starts on `http://localhost:8080`. You'll need to serve the frontend separately (see [Local Development Setup](#local-development-setup)).

---

## Quick Start (Docker Compose demo)

To try graph-info with sample data (PostgreSQL, MongoDB, MinIO, and mock services):

```bash
git clone https://github.com/guilherme-grimm/graph-go.git
cd graph-go
make docker-up

# Services will be available at:
# - Frontend:      http://localhost:3000
# - Backend API:   http://localhost:8080
# - MinIO Console: http://localhost:9001
```

**To stop:**
```bash
make docker-down
```

**To rebuild after code changes:**
```bash
make docker-build && make docker-up
```

---

## Local Development Setup

### Prerequisites
- Go 1.25.6+
- Node.js 24+ (or Bun)
- Docker (required for integration tests)

### Backend Setup

```bash
# Install Go dependencies
cd binary
go mod download

# Copy sample config and edit connection strings
cp ../conf/config.sample.yaml ../conf/config.yaml
# Edit conf/config.yaml with your connection details

# Run the backend
go run ./cmd/app/main.go
```

The backend API will start on `http://localhost:8080`.

### Frontend Setup

```bash
# Install dependencies
cd webui
npm install

# Start dev server
npm run dev
```

The frontend dev server will start on `http://localhost:5173`.

### Using Make

```bash
make install       # Install all dependencies
make dev          # Run backend + frontend concurrently
make build        # Build production binaries
make test         # Run all tests
```

---

## Configuration

**Auto-discovery is the preferred way to use graph-info.** Mount the Docker socket and/or run inside a Kubernetes cluster, and graph-info will automatically detect your infrastructure — no configuration needed. Both discoverers activate via auto-detection and run in parallel.

The YAML config file (`conf/config.yaml`) is only needed for services that aren't reachable via Docker or Kubernetes, such as remote databases, managed cloud services, or external endpoints. When both are used, graph-info merges discovered services with the config file.

See `conf/config.sample.yaml` for examples.

### Kubernetes Discovery

Kubernetes discovery activates automatically when it detects an in-cluster service account or a `~/.kube/config`. Override with:

```yaml
kubernetes:
  enabled: true          # nil = auto-detect
  kubeconfig: ""         # path override; empty = default lookup
  context: ""            # empty = current context
  namespaces: []         # empty = all namespaces
```

### Docker Discovery

```yaml
docker:
  enabled: true          # nil = auto-detect
  socket: "/var/run/docker.sock"
  network: ""            # limit to specific Docker network
  ignore_images: []      # images to skip during classification
```

### PostgreSQL Adapter

```yaml
connections:
  - name: postgres
    type: postgres
    dsn: "postgres://user:password@localhost:5432/mydb?sslmode=disable"
```

### MongoDB Adapter

```yaml
connections:
  - name: mongodb
    type: mongodb
    uri: "mongodb://user:password@localhost:27017"
```

### S3 / AWS Adapter

```yaml
connections:
  - name: s3
    type: s3
    region: us-east-1
    access_key_id: "YOUR_ACCESS_KEY"
    secret_access_key: "YOUR_SECRET_KEY"
```

### MinIO / S3-Compatible Adapter

```yaml
connections:
  - name: minio
    type: s3
    region: us-east-1
    endpoint: "http://localhost:9000"
    access_key_id: minioadmin
    secret_access_key: minioadmin
```

### MySQL Adapter

```yaml
connections:
  - name: mysql
    type: mysql
    dsn: "root:password@tcp(localhost:3306)/mydb"
```

### Redis Adapter

```yaml
connections:
  - name: redis
    type: redis
    uri: "redis://localhost:6379"
```

Or with host/port (useful for Docker discovery):

```yaml
connections:
  - name: redis
    type: redis
    host: localhost
    port: 6379
    password: ""
```

### Elasticsearch Adapter

```yaml
connections:
  - name: elasticsearch
    type: elasticsearch
    endpoint: "http://localhost:9200"
    username: elastic
    password: changeme
```

**Important:** This tool is intended for authorized infrastructure visualization and monitoring of systems you own or have permission to access. Do not use it to scan or access systems without authorization.

---

## Architecture Overview

### Backend (Go)

```
                          ┌─────────────────────────────────────┐
                          │         Discoverer Interface         │
                          │  Discover() · Watch() · Close()     │
                          └──────────┬──────────┬───────────────┘
                                     │          │
                          ┌──────────▼──┐  ┌────▼──────────────┐
                          │   Docker    │  │   Kubernetes       │
                          │  Discoverer │  │   Discoverer       │
                          │ (containers,│  │ (informers, pods,  │
                          │  classify,  │  │  deployments,      │
                          │  events)    │  │  services, health) │
                          └──────┬──────┘  └────┬──────────────┘
                                 │               │
                          ┌──────▼───────────────▼──────┐
                          │  Parallel Discovery + Merge  │
                          │  (concatenate ServiceInfo)   │
                          └──────────────┬──────────────┘
                                         │
Config (YAML) ──→ YAML Merge ───────────▶│
                                         ▼
                          ┌─────────────────────────────┐
                          │     Adapter Registry         │
                          │  ├─ PostgreSQL  → Tables + FK│
                          │  ├─ MongoDB    → Collections │
                          │  ├─ MySQL      → Tables + FK │
                          │  ├─ Redis      → Keyspaces   │
                          │  ├─ Elasticsearch → Indices   │
                          │  ├─ S3         → Buckets      │
                          │  └─ HTTP       → Health + deps│
                          │                               │
                          │  + Topology (K8s nodes/edges) │
                          └──────────────┬───────────────┘
                                         ▼
                          Graph Model (Nodes + Edges)
                                         ▼
                          REST API + WebSocket (Real-time)
```

**Key Components:**
- **Discoverer Interface**: Uniform contract (`Discover`, `Watch`, `Close`) for all discovery backends — Docker and Kubernetes run in parallel, results are concatenated
- **Docker Discovery**: Inspects containers, classifies images, extracts credentials from env vars, watches Docker events for live topology changes
- **Kubernetes Discovery**: Uses client-go informers with debounced event handling; discovers Namespaces, Deployments, StatefulSets, DaemonSets, Pods, and Services with health mapping
- **Adapters**: Implement the `Adapter` interface to probe databases and storage services
- **Registry**: Manages adapters and topology sets, creates service-level parent nodes, aggregates graph data
- **Cache**: 30-second TTL with singleflight pattern to prevent thundering herd
- **WebSocket**: Streams health updates every 5 seconds

### Frontend (React + TypeScript)

- **Swimlane Layout**: Namespace-aware layout with zone classification (system, infra, application namespaces)
- **Group Containers**: K8s namespaces render as collapsible bounding boxes via React Flow grouping
- **Node Inspector**: Side panel showing detailed metadata and connections
- **WebSocket Hook**: Real-time health updates without polling

### Node Hierarchy

```
Adapter-discovered:
  Service Node (postgres/mongodb/s3)
      └─ Database/Bucket Node
          └─ Table/Collection/Prefix Node

Kubernetes-discovered:
  Namespace (group container)
      └─ Deployment / StatefulSet / DaemonSet
          └─ Pod
      └─ K8sService ──routes_to──→ Pod
```

Edges represent relationships (`contains`, `foreign_key`, `routes_to`, etc.).

---

## Tech Stack

**Backend:**
- Go 1.25.6
- gorilla/mux (HTTP routing)
- k8s.io/client-go (Kubernetes discovery + informers)
- pgxpool (PostgreSQL)
- mongo-driver v2 (MongoDB)
- go-sql-driver/mysql (MySQL)
- go-redis/v9 (Redis)
- go-elasticsearch/v8 (Elasticsearch)
- AWS SDK v2 (S3)
- coder/websocket (WebSocket)
- testcontainers-go (integration tests)

**Frontend:**
- TypeScript
- React 18
- React Flow (graph visualization)
- Vite (build tool)

**Infrastructure:**
- Docker + Docker Compose
- PostgreSQL 17
- MongoDB 7
- MySQL 8
- Redis 7
- Elasticsearch 8
- MinIO (S3-compatible)

---

## Testing

### Unit Tests

```bash
cd binary && go test ./...
```

Runs without Docker. Includes pure function tests and HTTP handler tests.

### Integration Tests

```bash
cd binary && go test -tags=integration -v -timeout=5m ./internal/adapters/...
```

Requires Docker. Uses [testcontainers-go](https://golang.testcontainers.org/) to spin up real database instances (PostgreSQL, MongoDB, MySQL, Redis, Elasticsearch, MinIO) — no mocks.

Every adapter runs through the **contract test suite** (`adaptertest.RunContractTests`) which validates:
- Connect/disconnect lifecycle
- Node/edge discovery (unique IDs, valid parent refs, correct types)
- Health metrics (status key, required keys)

Run a single adapter's tests:

```bash
cd binary && go test -tags=integration -v ./internal/adapters/redis/
```

### All Tests

```bash
make test  # unit + type-check
cd binary && go test -tags=integration -timeout=5m ./internal/adapters/...  # integration
```

---

## API Reference

### GET `/api/graph`
Returns the full infrastructure graph (nodes + edges).

**Response:**
```json
{
  "data": {
    "nodes": [
      {
        "id": "service-postgres",
        "type": "postgres",
        "name": "postgres",
        "metadata": { "adapter": "postgres" },
        "health": "healthy"
      }
    ],
    "edges": [
      {
        "id": "edge-1",
        "source": "service-postgres",
        "target": "pg-mydb",
        "type": "contains",
        "label": "contains"
      }
    ]
  }
}
```

### GET `/api/node/{id}`
Returns details for a specific node.

### GET `/api/health`
Returns adapter health status (ok/degraded/error).

### WS `/websocket`
Streams real-time health updates.

**Message format:**
```json
{
  "type": "health_update",
  "nodeId": "postgres",
  "status": "healthy",
  "timestamp": "2026-02-09T10:30:00Z"
}
```

---

## Adding a New Adapter

1. **Create adapter package** in `binary/internal/adapters/{name}/`
2. **Implement the `Adapter` interface:**
   ```go
   type Adapter interface {
       Connect(config ConnectionConfig) error
       Discover() ([]nodes.Node, []edges.Edge, error)
       Health() (HealthMetrics, error)
       Close() error
   }
   ```
3. **Self-register** via `init()` with `adapters.RegisterFactory("name", ...)`
4. **Add integration tests** (required) — create `{name}_integration_test.go` with:
   - Build tag `//go:build integration`
   - `TestMain` using testcontainers-go to start a real instance
   - Seed representative data
   - Call `adaptertest.RunContractTests` to validate the interface contract
   - Add adapter-specific tests (filtering, ID format, metadata, etc.)
5. **Import adapter** in `binary/internal/server/server.go` (blank import for `init()`)
6. **Add node type** in `binary/internal/graph/nodes/nodes.go`
7. **Update frontend types** in `webui/src/types/graph.ts`
8. **Add icon** in `webui/src/components/graph/CustomNode.tsx`

## Adding a New Discoverer

Discoverers live in `binary/internal/discovery/{name}/` and implement the `Discoverer` interface:

```go
type Discoverer interface {
    Name() string
    Discover(ctx context.Context) ([]ServiceInfo, error)
    Watch(ctx context.Context, onChange func()) error
    Close() error
}
```

1. **Create discoverer package** in `binary/internal/discovery/{name}/`
2. **Implement the `Discoverer` interface** — return `[]ServiceInfo` from `Discover()`. Topology-producing discoverers (like K8s) populate `Nodes`/`Edges` directly; adapter-oriented ones (like Docker) populate `Config` for adapter bridging.
3. **Wire into server** in `binary/internal/server/server.go` — add a `build{Name}Discovery()` function and call it alongside the existing discoverers.
4. **Add integration tests** with `//go:build integration` — use real infrastructure (kind/k3d for K8s, testcontainers for others). No mocks.

See `CONTRIBUTING.md` for detailed guidance.

---

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on:
- Development setup
- Code style conventions
- How to add new adapters
- Submitting pull requests

---

## Use Scope & Ethics

**Intended Use:**
- Visualizing and monitoring infrastructure you own or have authorization to access
- DevOps dashboards and topology mapping
- Infrastructure documentation and onboarding
- Exploring database schemas and relationships

**Not Intended For:**
- Unauthorized system scanning or reconnaissance
- Security testing without explicit permission
- Accessing systems you don't own or control

Users are responsible for ensuring they have proper authorization before connecting graph-info to any infrastructure.

---

## License

This project is licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0)**.

See the [LICENSE](LICENSE) file for details. AGPL requires that modified versions used over a network must also be open-sourced.

---

## CI/CD & Releases

The project uses GitHub Actions for continuous integration and automated releases.

- **CI** runs on every push/PR to `main` — backend unit tests, integration tests (testcontainers), and frontend build
- **Releases** are triggered by version tags (`v*`) and produce:
  - Cross-platform binaries (Linux, macOS, Windows) via [GoReleaser](https://goreleaser.com)
  - Docker images pushed to `ghcr.io/guilherme-grimm/graph-go-backend` and `-frontend`

To create a release:
```bash
git tag v0.1.0
git push --tags
```

---

## Roadmap

- [x] Docker auto-discovery
- [x] HTTP service health monitoring
- [x] MySQL adapter
- [x] Redis adapter
- [x] Elasticsearch adapter
- [x] Integration tests with testcontainers-go (all adapters)
- [x] Contract test suite for adapter interface compliance
- [x] Discoverer interface (pluggable discovery backends)
- [x] Kubernetes orchestrator (Namespaces, Deployments, StatefulSets, DaemonSets, Pods, Services)
- [x] Informer-based real-time K8s watching with debounce
- [x] Swimlane layout with namespace group containers
- [ ] K8s adapter bridging (classify pods by image, connect adapters to databases in pods)
- [ ] Flow observability (real-time data flow visualization)
- [ ] Integrated stress trigger (k6 with real-time impact visualization)
- [ ] Kafka adapter
- [ ] Additional orchestrators (ECS, Nomad)
- [ ] Graph persistence (save/load views)
- [ ] Multi-region visualization
- [ ] Alert configuration per node

---

## Support

- **Issues**: [github.com/guilherme-grimm/graph-go/issues](https://github.com/guilherme-grimm/graph-go/issues)
- **Discussions**: [github.com/guilherme-grimm/graph-go/discussions](https://github.com/guilherme-grimm/graph-go/discussions)

---

**Built with ❤️ for DevOps and infrastructure engineers**
