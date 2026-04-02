# graph-go: Planned Features

## Done

### New Adapters
- MySQL, Redis, Elasticsearch adapters (branch: `feature/new-adapters`)

### Integration & Contract Tests
- Integration tests using `testcontainers-go` for all adapters (PostgreSQL, MongoDB, MySQL, Redis, Elasticsearch, S3/MinIO)
- Shared contract test suite (`adaptertest.RunContractTests`) — validates Adapter interface compliance
- Build tag `//go:build integration` keeps tests out of `go test ./...`; CI runs them separately
- Contract tests are a documented requirement for all new adapters

---

## Confirmed Roadmap

### CLI (`graphgo` command)
- Export subcommand: `graphgo export --output mermaid|dot|json`
  - `stdout` for data output (pipeable), `stderr` for all logs and status
- Validate subcommand: `graphgo validate` — parse and schema-check config, report missing fields
  - `graphgo validate --connect` — additionally test connectivity per service, no server started

### Startup Report
- On server start, print a structured connection summary to `stderr`:
  ```
  graph-go: discovery complete
    ✓ postgres (mydb) — 12 tables, 8 edges
    ✓ redis (cache) — 3 keyspaces
    ✗ elasticsearch (search) — connection refused: dial tcp 127.0.0.1:9200
  ```

### Structured Logging
- Use `log/slog` (stdlib, no extra dependency)
- `--log-level` flag (`info` default, `debug` for verbose)
- All log output to `stderr`; `stdout` reserved for machine-readable data
- Debug level surfaces: adapter connect attempts, cache hits/misses, Docker events, discovery duration per adapter

### Orchestrator Support
- Docker stays the core discovery source
- Future layer: Kubernetes + other orchestrators (ECS, Nomad, etc.)
- Scope: read-only discovery against the current user's context

### Message Broker Adapters
- **Phase 1:** Structural topology only — topics, partitions, consumer groups (Kafka); exchanges, queues, bindings (RabbitMQ); subjects/streams (NATS)
- **Phase 2 (major milestone):** Infer producer/consumer relationships and visualize full application flow in a dedicated detailed view. Most broker APIs expose enough for reasonable inference without app-side instrumentation.

### Stress Tests
- Stress test mode:
  - Target: the discovered services themselves (Postgres, Redis, etc.)
  - Triggered from the UI per node
  - Health metrics pushed via WebSocket at sub-second/1-second intervals during stress (bypasses normal 10s cache)
  - Real-time visualization of health degradation and recovery in the graph UI
  - Safety: opt-in per service via config flag or Docker label; confirmation step in UI

---

## Deferred (Polish Phase)

### Hot Reload
- Watch `config.yaml` for changes via `fsnotify`
- Re-merge YAML config + Docker discovery and re-register adapters without restart
- Mirrors existing Docker event watcher pattern in `events.go`

### Bearer Token Auth
- Optional static token via `GRAPHGO_TOKEN` env var
- If set: all `/api/*` and `/websocket` routes require `Authorization: Bearer <token>`
- If unset: open access (current behavior, safe for local dev)
- No sessions or OAuth — one env var for shared environments

---

## Implementation Priority (Recommended)

1. `log/slog` + `--log-level` — foundation everything else builds on
2. Startup report — immediately improves DX, low effort
3. `graphgo validate` — unblocks safer config iteration
4. CLI export (`graphgo export`) — unlocks CI/docs use cases
5. ~~Integration tests — testcontainers-go per adapter~~ **Done**
6. Orchestrator support (K8s first)
7. Message broker adapters (Phase 1)
8. Stress test mode
9. Message broker flow inference (Phase 2)
10. Polish: hot reload, bearer token
