<!-- markdownlint-disable -->
# PRD: Auto-Discovery & Zero-Config Setup

**Status**: Draft
**Author**: devgrimm
**Target**: Week of Feb 10-16, 2026
**Context**: Side project — scoped for evenings/weekend

---

## Problem Statement

graph-info auto-discovers database schemas (tables, collections, buckets) but requires manual YAML configuration for two critical things:

1. **Service-to-infrastructure relationships** — The `depends_on` config in HTTP adapters is the only way to express "user-service reads from pg-mydb-users." This is the most valuable information in the graph and the most likely to go stale.

2. **Service registration itself** — Every database, object store, and application service must be declared in `config.yaml` with connection strings. This duplicates information already present in `docker-compose.yml` or the running environment.

These two manual steps undermine the "auto-discovery" promise. The tool should discover infrastructure topology from what's actually running, not from what someone remembered to write in a config file.

---

## Goal

Eliminate mandatory configuration. A developer adds graph-info to their Docker environment, and it discovers:
- What infrastructure services are running (Postgres, Mongo, MinIO, Redis)
- What application services exist
- Which services are connected to which databases, and which tables they touch

No `config.yaml` required for the common case.

---

## Non-Goals

- Production monitoring (alerting, metrics retention, dashboards)
- Agent/sidecar deployment model
- Deep query analysis or performance profiling
- Support for non-Docker environments (Kubernetes, bare metal) in this iteration
- Frontend changes beyond supporting new data (no UI redesign)

---

## Design

### 1. Docker API Service Discovery

**Replace `config.yaml` as the primary config source with the Docker API.**

graph-info connects to the Docker daemon via `/var/run/docker.sock` (mounted as a volume) and queries running containers. From each container it extracts:

| Docker API field | What we learn |
|---|---|
| `Config.Image` | Infrastructure type detection (`postgres:*` -> postgres adapter) |
| `Config.Env` | Credentials (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, etc.) |
| `NetworkSettings.Networks[*].IPAddress` | Maps back to `pg_stat_activity.client_addr` |
| `Config.Labels` | User overrides (`graphinfo.type`, `graphinfo.dsn`, `graphinfo.ignore`) |
| `Name` / compose service name | Node display name and ID |
| `State.Health.Status` | Container-level health |

**Image detection rules:**

| Image pattern | Adapter type | Credential env vars |
|---|---|---|
| `postgres:*`, `*postgres*` | postgres | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` |
| `mongo:*`, `*mongo*` | mongodb | `MONGO_INITDB_ROOT_USERNAME`, `MONGO_INITDB_ROOT_PASSWORD` |
| `minio/*`, `*minio*` | s3 | `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD` |
| `redis:*`, `*redis*` | redis | `REDIS_PASSWORD` (optional) |
| Everything else | http | Detected as application service |

**User overrides via Docker labels:**

```yaml
# docker-compose.yml
services:
  my-custom-db:
    image: my-company/pg-custom:latest
    labels:
      graphinfo.type: postgres                           # Force adapter type
      graphinfo.dsn: "postgres://user:pass@:5432/mydb"  # Override auto-detected DSN
      graphinfo.ignore: "true"                           # Exclude from graph
      graphinfo.node-type: "database"                    # Override display type
```

Labels are the escape hatch for anything auto-detection gets wrong.

**Fallback**: `config.yaml` still works as an optional additive config source. Entries in YAML are merged with Docker-discovered services. YAML takes precedence on conflict (explicit > implicit). This maintains backward compatibility and supports non-Docker infrastructure (remote databases, cloud services).

#### Implementation

New package: `binary/internal/discovery/docker.go`

```go
type DockerDiscovery struct {
    client *docker.Client
}

type DiscoveredService struct {
    Name       string
    Type       string               // postgres, mongodb, s3, redis, http
    Config     adapters.ConnectionConfig
    IPAddress  string               // For connection mapping
    ContainerID string
}

// Discover queries the Docker API for running containers,
// classifies them, extracts credentials, and returns
// services ready for adapter registration.
func (d *DockerDiscovery) Discover() ([]DiscoveredService, error)
```

**Dependency**: `github.com/docker/docker/client` — official Docker SDK for Go.

**Integration point**: `server.go` calls `DockerDiscovery.Discover()` before the existing config loop. Discovered services are registered the same way config entries are today — through `AdapterFactory` + `registry.Register()`. The adapter interface doesn't change.

#### Container IP Resolution

The Docker discovery also builds a lookup table of container IPs to service names:

```go
type IPResolver struct {
    ipToService map[string]string  // "172.18.0.5" -> "user-service"
}
```

This is passed to infrastructure adapters that support connection tracking (Phase 2), allowing them to map database client addresses back to application service names.

---

### 2. Connection-Based Dependency Discovery

**Replace manual `depends_on` with live connection tracking from database-side metadata.**

Each infrastructure adapter gains a new optional method:

```go
type ConnectionTracker interface {
    // ActiveConnections returns the services currently connected
    // to this infrastructure, resolved via the IP lookup table.
    ActiveConnections(resolver IPResolver) ([]Connection, error)
}

type Connection struct {
    ServiceName string   // Resolved from client IP -> container name
    ClientAddr  string   // Raw IP
    Database    string   // Which database/bucket
    Tables      []string // Which tables/collections (if available)
}
```

Adapters implement this interface optionally. The registry checks for it during `DiscoverAll()` and generates `depends_on` edges from the results.

#### Postgres: `pg_stat_activity` + `pg_stat_user_tables`

```sql
-- Who is connected right now?
SELECT client_addr, application_name, datname, usename, state
FROM pg_stat_activity
WHERE datname = $1 AND client_addr IS NOT NULL;
```

This returns every active connection with the client's IP address. The `IPResolver` maps `client_addr` -> container name -> service name. Each unique service becomes a `depends_on` edge to the database.

For table-level granularity (which tables a service touches):

```sql
-- Which tables have been accessed? (cumulative stats since last reset)
SELECT schemaname, relname,
       seq_scan + idx_scan as total_reads,
       n_tup_ins + n_tup_upd + n_tup_del as total_writes
FROM pg_stat_user_tables
WHERE seq_scan + idx_scan + n_tup_ins + n_tup_upd + n_tup_del > 0;
```

This gives table-level activity but not per-service breakdown. Combined with `pg_stat_activity` (which shows the current query), we can infer which service is accessing which tables for active connections.

**Graceful degradation**: `pg_stat_activity` requires `pg_monitor` role or superuser. If the adapter connects with limited permissions:
1. Try `pg_stat_activity` query
2. If permission denied, log a warning: "Connection tracking unavailable — grant pg_monitor role for auto-discovery of service dependencies"
3. Fall back to schema-only discovery (tables + FKs, no service edges)
4. Set a metadata flag `connectionTrackingAvailable: false` on the service node so the frontend can surface this to the user

The adapter still works, it just discovers less.

#### MongoDB: `db.currentOp()`

```javascript
db.adminCommand({ currentOp: 1, active: true })
```

Returns active operations with `client` (IP:port) and `ns` (database.collection). Same IP resolution flow as Postgres.

**Graceful degradation**: `currentOp` requires `clusterMonitor` role or equivalent. Same fallback pattern — if denied, log warning and skip connection tracking.

#### S3/MinIO

No built-in connection tracking equivalent. S3 is stateless HTTP — there's no "who is connected" API. For MinIO, audit logging exists but is opt-in and complex to parse.

**Decision**: Skip connection tracking for S3 in this iteration. Service-to-bucket relationships can still come from:
- Manual `depends_on` in YAML (optional override)
- Future: MinIO audit webhook integration

---

### 3. Edge Generation

The registry's `DiscoverAll()` gains a new step after adapter discovery:

```
For each infrastructure adapter that implements ConnectionTracker:
    1. Call ActiveConnections(ipResolver)
    2. For each connection:
       a. Find or create the service node (by resolved service name)
       b. Create "depends_on" edge: service -> database/table
       c. Set edge label from connection context ("reads/writes", "connected")
    3. If a service node was auto-created (not from HTTP adapter or Docker API):
       mark it with metadata: { discovered: "connection" }
```

This means a service can appear in the graph purely because Postgres sees it connected — no config, no Docker label, no HTTP adapter entry needed.

**Edge deduplication**: If the same service->table edge exists from both connection tracking and manual YAML `depends_on`, the manual one takes precedence (richer label, explicit intent).

**Edge labels**:
- If we can see the current query: extract table names and classify as "reads" (SELECT) or "writes" (INSERT/UPDATE/DELETE)
- If we can only see the connection exists: label as "connected"
- Manual YAML override: whatever label the user specified

---

### 4. HTTP Adapter Changes

The HTTP adapter remains but becomes **optional and additive**:

- Services discovered from Docker API don't need HTTP adapter entries
- If an HTTP adapter entry exists for a Docker-discovered service, the HTTP config enriches the node (adds `node_type`, custom labels, health endpoint)
- HTTP adapter is still useful for:
  - Services not in Docker (remote APIs, cloud functions)
  - Overriding `node_type` (distinguishing `gateway` vs `auth` vs `service`)
  - Adding explicit `depends_on` edges to non-database targets (service-to-service)

The `depends_on` field in HTTP adapter config becomes optional. If omitted, dependencies are discovered from connection tracking. If present, they're merged (manual edges supplement auto-discovered ones).

---

### 5. Config Changes

`config.yaml` is no longer required. The new config hierarchy (highest precedence first):

1. **Docker labels** (`graphinfo.*`) — per-container overrides
2. **config.yaml** — explicit entries, additive
3. **Docker API** — auto-detected from running containers
4. **Connection tracking** — auto-detected from database metadata

New optional top-level config for Docker discovery:

```yaml
# conf/config.yaml (entirely optional now)

docker:
  enabled: true                    # default: true if socket available
  socket: /var/run/docker.sock     # default
  network: graph-info_default      # optional: limit to specific Docker network
  ignore_images:                   # optional: patterns to skip
    - "traefik:*"
    - "graph-info*"

# Existing connections array still works for non-Docker or override use cases
connections:
  - name: remote-postgres
    type: postgres
    dsn: "postgres://user:pass@remote-host:5432/prod"
```

**Breaking change**: The `depends_on` field on HTTP adapter entries is no longer the expected way to map service dependencies. It still works, but docs should guide users toward Docker labels or connection-based discovery instead. No code removal needed — purely a documentation/messaging shift.

---

### 6. Discovery Flow (New)

Complete flow when `GET /api/graph` is called (first time, empty cache):

```
1. DockerDiscovery.Discover()
   ├─ Query Docker API for running containers
   ├─ Classify each container by image name + labels
   ├─ Extract credentials from environment variables
   ├─ Build IP -> service name resolver
   └─ Return []DiscoveredService

2. Merge with config.yaml (if exists)
   ├─ YAML entries override Docker-discovered entries with same name
   └─ YAML entries for unknown services are added

3. For each service:
   ├─ AdapterFactory(type) -> create adapter
   ├─ adapter.Connect(config) -> validate connection
   └─ registry.Register(name, type, adapter, config)

4. registry.DiscoverAll() (existing flow, extended)
   ├─ For each adapter: adapter.Discover() -> nodes + edges
   ├─ For non-HTTP: create service wrapper node, re-parent orphans
   ├─ NEW: For each ConnectionTracker adapter:
   │   ├─ adapter.ActiveConnections(ipResolver)
   │   └─ Generate depends_on edges from live connections
   ├─ Merge manual depends_on edges from HTTP adapters
   └─ Cache result (30s TTL)

5. Return graph JSON
```

---

## Implementation Plan

Ordered by dependency. Each step produces a working (if incomplete) system.

### Step 1: Docker API Discovery

**New files:**
- `binary/internal/discovery/docker.go` — Docker client, container classification, credential extraction
- `binary/internal/discovery/resolver.go` — IP-to-service-name resolution

**Modified files:**
- `binary/internal/server/server.go` — Call Docker discovery before config-based registration, merge results
- `go.mod` — Add `github.com/docker/docker` dependency

**Outcome**: graph-info detects running Postgres/Mongo/MinIO containers without any config.yaml. Application services appear as nodes detected from Docker. No dependency edges yet (that's Step 2).

**Verify**: Remove `config.yaml`, run `docker-compose up`, hit `/api/graph` — infrastructure nodes should appear.

### Step 2: Connection Tracking Interface

**New files:**
- `binary/internal/adapters/tracker.go` — `ConnectionTracker` interface definition, `IPResolver` type, `Connection` type

**Modified files:**
- `binary/internal/adapters/postgres/postgres.go` — Implement `ConnectionTracker` with `pg_stat_activity` query, graceful degradation on permission denied
- `binary/internal/adapters/mongodb/mongo.go` — Implement `ConnectionTracker` with `currentOp`, graceful degradation
- `binary/internal/adapters/registry.go` — After adapter discovery, check for `ConnectionTracker` interface, call `ActiveConnections`, generate edges

**Outcome**: Service-to-database edges appear automatically based on live connections. No `depends_on` config needed.

**Verify**: Start a service that connects to Postgres. Without any config referencing that service, the edge should appear in the graph within 30 seconds (cache TTL).

### Step 3: Docker Labels & Config Merge

**Modified files:**
- `binary/internal/discovery/docker.go` — Read `graphinfo.*` labels, apply overrides
- `binary/internal/config/config.go` — Add `Docker` section to config struct, load both sources
- `binary/internal/server/server.go` — Merge Docker-discovered + YAML config entries

**Outcome**: Users can override auto-detection with labels. YAML config is optional but still works.

**Verify**: Add `graphinfo.ignore: "true"` label to a container — it should disappear from the graph. Add `graphinfo.type: postgres` to a custom image — it should be detected as Postgres.

### Step 4: Self-Exclusion & Edge Cleanup

**Modified files:**
- `binary/internal/discovery/docker.go` — Auto-exclude the graph-info container itself from discovery, filter known infrastructure tools (Traefik, etc.)
- `binary/internal/adapters/registry.go` — Deduplicate edges (same source+target), prefer manual label over auto-detected

**Outcome**: Clean graph without graph-info showing up as a node, no duplicate edges.

### Step 5: Table-Level Connection Detail (Stretch)

**Modified files:**
- `binary/internal/adapters/postgres/postgres.go` — Parse `pg_stat_activity.query` for active connections to extract table names, cross-reference with `pg_stat_user_tables` for cumulative stats

**Outcome**: Edges show "user-service -> pg-mydb-users (reads/writes)" instead of just "user-service -> pg-mydb (connected)".

**Note**: This is a stretch goal. The value difference between database-level edges and table-level edges is significant, but the implementation complexity of query parsing is non-trivial. Ship database-level first, refine to table-level if time allows.

---

## Verification

### Phase 1: Docker Discovery (Step 1)

1. Remove `conf/config.yaml` entirely
2. `docker-compose up` with the existing stack
3. Hit `GET /api/graph`
4. Verify: `service-postgres`, `service-mongodb`, `service-minio` nodes appear with correct types
5. Verify: all child nodes (databases, tables, collections, buckets) are discovered
6. Verify: application services (api-gateway, user-service, etc.) appear as nodes
7. Verify: mock services with no database connections appear as standalone nodes

### Phase 2: Connection Discovery (Step 2)

1. With Docker discovery running (no config.yaml)
2. Verify: `user-service -> pg-mydb` edge appears (because user-service has a Postgres connection)
3. Verify: `product-service -> mongo-store` edge appears
4. Verify: services with no DB connection (api-gateway) have no spurious edges
5. Stop a service, wait for cache refresh — verify edge disappears
6. Test graceful degradation: connect Postgres adapter with limited-privilege user, verify schema discovery still works and a warning is logged

### Phase 3: Labels & Config Merge (Step 3)

1. Add `graphinfo.ignore: "true"` to a container, verify it's excluded
2. Add `graphinfo.type: postgres` to a custom image, verify it's detected
3. Add `config.yaml` alongside Docker discovery, verify both sources appear
4. Verify YAML entry overrides Docker-detected entry with the same name
5. Verify `depends_on` in YAML adds edges that supplement auto-discovered ones

### Performance

- Docker API query should complete in < 500ms
- `pg_stat_activity` query should complete in < 100ms
- `currentOp` should complete in < 100ms
- Full discovery (Docker + all adapters + connection tracking) should complete in < 5s
- Cached response should return in < 5ms

---

## Risks

**Docker socket permission scope**: Mounting the socket grants broad Docker API access. Mitigate by documenting clearly, using read-only operations only, and considering a future move to the Docker API over TCP with TLS for production-adjacent environments.

**Image detection false positives**: A container named `postgres-exporter` matches `*postgres*` but isn't a Postgres database. Mitigate with the `graphinfo.ignore` label and by requiring the image to expose the expected port (5432 for Postgres, 27017 for Mongo).

**Transient connections**: `pg_stat_activity` shows connections at a point in time. A service that connects, runs a query, and disconnects between polling intervals won't be captured. Mitigate by checking on every discovery call (30s interval) and potentially caching seen connections for a configurable window (e.g., "show edges for services seen in the last 5 minutes").

**Credential extraction isn't universal**: Not all Postgres containers use `POSTGRES_USER`. Custom images, `DATABASE_URL` format, secrets managers, and `.env` files all complicate this. The label override (`graphinfo.dsn`) is the escape hatch, and should be documented prominently.

---

## Future Work (Out of Scope for This Week)

- **Redis adapter**: `CLIENT LIST` for connections, `INFO keyspace` for databases, `KEYS` or `SCAN` for key patterns
- **Kafka adapter**: Topic listing, consumer group discovery, offset lag
- **Query-level table mapping**: Parse SQL from `pg_stat_statements` or `pg_stat_activity.query` for precise table-level edges
- **Connection history**: Remember previously-seen connections (with TTL) to handle transient services
- **Kubernetes discovery**: Replace Docker API with Kubernetes API for pod/service discovery (same pattern, different client)
- **Published Docker image**: `ghcr.io/devgrimm/graph-info` so users don't need to clone the repo
