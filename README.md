<div align="center">

# graph-info

**See your infrastructure. Instantly.**

Point graph-info at your stack and get a live, interactive map of every database, table, service, and storage bucket — with real-time health monitoring.

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
![graph-info demo](./docs/GraphGo.png)

</div>

---

graph-info **auto-discovers** your infrastructure by connecting to the Docker daemon, inspecting running containers, and probing databases and storage services. No manual inventory needed — it builds the graph for you.

| Capability | Details |
|---|---|
| **Auto-discovery** | Detects PostgreSQL, MongoDB, MinIO/S3, Redis, and HTTP services from Docker containers |
| **PostgreSQL** | Tables, foreign key relationships, schema topology |
| **MongoDB** | Databases and collections |
| **S3 / MinIO** | Buckets and top-level prefixes |
| **HTTP services** | Health endpoints, dependency mapping between services |
| **Real-time health** | WebSocket-powered live status updates every 5 seconds |
| **Interactive graph** | Pan, zoom, filter by type/health, search nodes, pin layouts |

---

## Quick Start (Docker)

The fastest way to try graph-info is with Docker Compose, which includes PostgreSQL, MongoDB, and MinIO with sample data:

```bash
# Clone and start all services
git clone https://github.com/Athla/grimm-nodes-work.git
cd graph-info
make docker-up

# Services will be available at:
# - Frontend:      http://localhost:3000
# - Backend API:   http://localhost:8080
# - MinIO Console: http://localhost:9001
```

The Docker environment includes seed data so you can see the graph immediately.

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
- Node.js 18+ (or Bun)
- PostgreSQL, MongoDB, or S3 (optional, for testing adapters)

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
make lint         # Run golangci-lint
```

---

## Configuration

graph-info uses a YAML configuration file at `conf/config.yaml`. See `conf/config.sample.yaml` for examples.

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

**Important:** This tool is intended for authorized infrastructure visualization and monitoring of systems you own or have permission to access. Do not use it to scan or access systems without authorization.

---

## Architecture Overview

### Backend (Go)

```
Docker Daemon ──→ Auto-Discovery ──→ Classify containers
                                         ↓
Config (YAML) ──→ Adapter Registry ──→ Merge discovered + configured
                      ├─ PostgreSQL Adapter → Tables + foreign keys
                      ├─ MongoDB Adapter    → Collections
                      ├─ S3 Adapter         → Buckets + prefixes
                      └─ HTTP Adapter       → Service health + dependencies
                      ↓
                  Graph Model (Nodes + Edges)
                      ↓
                  REST API + WebSocket (Real-time health)
```

**Key Components:**
- **Auto-Discovery**: Inspects Docker containers, classifies images, extracts credentials from env vars, watches Docker events for live topology changes
- **Adapters**: Implement the `Adapter` interface to discover infrastructure
- **Registry**: Manages adapters, creates service-level parent nodes, aggregates graph data
- **Cache**: 30-second TTL with singleflight pattern to prevent thundering herd
- **WebSocket**: Streams health updates every 5 seconds

### Frontend (React + TypeScript)

- **React Flow**: Interactive graph visualization with pan/zoom
- **Hierarchical Layout**: Automatically positions nodes by parent-child relationships
- **Node Inspector**: Side panel showing detailed metadata and connections
- **WebSocket Hook**: Real-time health updates without polling

### Node Hierarchy

```
Service Node (postgres/mongodb/s3)
    └─ Database/Bucket Node
        └─ Table/Collection/Prefix Node
```

Edges represent relationships (foreign keys, contains, etc.).

---

## Tech Stack

**Backend:**
- Go 1.25.6
- gorilla/mux (HTTP routing)
- pgxpool (PostgreSQL)
- mongo-driver v2 (MongoDB)
- AWS SDK v2 (S3)
- coder/websocket (WebSocket)

**Frontend:**
- TypeScript
- React 18
- React Flow (graph visualization)
- Vite (build tool)

**Infrastructure:**
- Docker + Docker Compose
- PostgreSQL 17
- MongoDB 7
- MinIO (S3-compatible)

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
3. **Register in the adapter registry** (`binary/internal/adapters/registry.go`)
4. **Add node type** in `binary/internal/graph/nodes/nodes.go`
5. **Update frontend types** in `webui/src/types/graph.ts`
6. **Add icon** in `webui/src/components/graph/CustomNode.tsx`

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

- **CI** runs on every push/PR to `main` — backend tests, `golangci-lint`, and frontend build
- **Releases** are triggered by version tags (`v*`) and produce:
  - Cross-platform binaries (Linux, macOS, Windows) via [GoReleaser](https://goreleaser.com)
  - Docker images pushed to `ghcr.io/athla/grimm-nodes-work-backend` and `-frontend`

To create a release:
```bash
git tag v0.1.0
git push --tags
```

---

## Roadmap

- [x] Docker auto-discovery
- [x] HTTP service health monitoring
- [ ] Redis adapter
- [ ] Kafka adapter
- [ ] Elasticsearch adapter
- [ ] Custom edge types (replication, sharding)
- [ ] Graph persistence (save/load views)
- [ ] Multi-region visualization
- [ ] Alert configuration per node

---

## Support

- **Issues**: [github.com/Athla/grimm-nodes-work/issues](https://github.com/athla/grimm-nodes-work/issues)
- **Discussions**: [github.com/Athla/grimm-nodes-work/discussions](https://github.com/athla/grimm-nodes-work/discussions)

---

**Built with ❤️ for DevOps and infrastructure engineers**
