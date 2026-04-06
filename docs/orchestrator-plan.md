# Orchestrator Support — Implementation Plan

## Overview

Orchestrator support enables graph-go to discover and visualize infrastructure running in
Kubernetes (and future orchestrators) without being locked behind docker-compose. This unlocks
real-time visualization of production-like environments and lays the groundwork for stress
testing and flow observability features.

**Scope:** Kubernetes only for v1. Contracts will be extracted from this implementation to
support future orchestrators (ECS, Nomad, etc.) without rearchitecting.

---

## Design Decisions

### Discoverer Interface (Option A — Interface-driven)

A shared `Discoverer` interface replaces the current tight Docker coupling. Both Docker and
Kubernetes implement it. The caller doesn't know or care which discoverer produced the data.

This also enables **parallel discovery** — each discoverer runs in its own goroutine, results
are synced and concatenated, keeping cold start time flat regardless of discoverer count.

```go
type ServiceInfo struct {
    Name     string
    Type     string             // "postgres", "deployment", "pod", etc.
    Source   string             // "docker", "kubernetes", "yaml"
    Config   ConnectionConfig   // for future adapter bridging (Level 2)
    Nodes    []nodes.Node
    Edges    []edges.Edge
    Health   health.Status
    Metadata map[string]any
}

type Discoverer interface {
    Name() string
    Discover(ctx context.Context) ([]ServiceInfo, error)
    Watch(ctx context.Context, onChange func()) error
    Close() error
}
```

**Why this shape:**
- `ctx` on Discover/Watch for cancellation and graceful shutdown
- `ServiceInfo` carries nodes/edges directly for orchestrators (topology), while `Config`
  stays available for future adapter bridging (Level 2)
- `Watch` takes a callback (not a channel) — matches the existing Docker event watcher pattern
- `Name()` for logging/debugging
- `Close()` for resource cleanup (informer shutdown, Docker client close)

### Kubernetes Resources — Tier 1 Only

Discovered resources:
- Namespaces
- Deployments, StatefulSets, DaemonSets
- Pods
- Services (as `K8sService` to avoid collision with existing `TypeService`)

**Not in scope:** Ingresses, ConfigMaps, Secrets, Jobs, CronJobs, HPA, NetworkPolicies, RBAC.
These would turn graph-go into a kubectl dashboard, which is not the goal. The value is
*seeing running infrastructure and its health at a glance*.

### Topology Only (Level 1)

The K8s discoverer produces nodes and edges for K8s resources themselves. It does **not**
connect adapters to databases running inside pods (that's Level 2, planned separately).

> **Level 2 note:** After Level 1 is solid, add adapter bridging — classify pods by image,
> extract credentials from env/ConfigMaps/Secrets, feed into the adapter registry. Pod
> metadata (image, env, labels) should be retained in node metadata during Level 1 to make
> Level 2 a natural extension. Add caching so same pods aren't refetched on every cycle.

### Node Types and Hierarchy

New node types:
```
TypeNamespace
TypeDeployment
TypeStatefulSet
TypeDaemonSet
TypePod
TypeK8sService
```

Edge relationships:
```
Namespace    → Deployment/StatefulSet/DaemonSet   (contains)
Deployment/StatefulSet/DaemonSet → Pod            (contains)
K8sService   → Pod                                (routes_to — via label selector matching)
```

`routes_to` is a new edge type. Label selector matching happens at discovery time to generate
these edges.

### Namespaces as Group Containers

Namespaces render as **visual group containers** (bounding boxes), not parent nodes. A namespace
can contain an entire application stack — wrapping it visually communicates the boundary better
than a node with many edges fanning out.

Backend marks namespace nodes with `"group": true` in metadata. The frontend uses React Flow's
native grouping to render them as collapsible containers.

### Health Mapping

Kubernetes resources map to the existing `healthy | degraded | unhealthy | unknown` model:

| Resource | Healthy | Degraded | Unhealthy |
|---|---|---|---|
| **Pod** | Running + all containers ready | Running, some containers not ready | Pending / Failed / CrashLoopBackOff |
| **Deployment/StatefulSet/DaemonSet** | availableReplicas == desired | available > 0 but < desired | availableReplicas == 0 |
| **K8sService** | All target pods healthy | Mixed pod health | No healthy target pods |
| **Namespace** | Aggregate of children | Aggregate of children | Aggregate of children |

### Authentication — Both In-Cluster and Out-of-Cluster

Standard fallback pattern:
1. Try in-cluster config (service account token at `/var/run/secrets/kubernetes.io/serviceaccount/`)
2. Fall back to `~/.kube/config`
3. If neither exists, K8s discovery simply doesn't activate

This also supports multiple clusters in parallel — each discovered independently, results merged.

### Watching — Informers with Debounce

Use client-go's `SharedInformerFactory` for real-time resource watching. Informers provide:
- Event callbacks (add/update/delete) mapping to the `Watch(ctx, onChange)` interface
- Local cache queryable during `Discover()` — no API server hit after initial sync

**Debounce topology events** by default (2-3 second window). K8s is much noisier than Docker —
rolling deployments, pod restarts, scaling events fire many events in quick succession. Batch
them into a single cache invalidation + WebSocket push.

> A flag to disable debounce (raw event mode) is planned for a later phase, useful when users
> want to watch every event during stress testing.

### Auto-Discovery First, Config as Override

Same philosophy as Docker. K8s discovery activates automatically when it detects a cluster.
Config is only needed for edge cases:

```yaml
kubernetes:
  enabled: true  # nil = auto-detect
  context: ""    # empty = current context
  namespaces: [] # empty = all namespaces
```

Most users never touch this section.

### Merge Strategy — Concatenation, Not Conflict Resolution

All discoverers' results are **concatenated** into the final graph. No priority-based merging.
Node ID prefixes (e.g., `k8s-default-nginx-abc123`, `pg-mydb-users`) keep things naturally
distinct. Each `ServiceInfo` carries its `Source` field.

YAML overrides are scoped: a YAML entry overrides within its own type/source. This is the
fallback for advanced corrections, not the normal flow.

**Why no conflict resolution:** Running Docker and K8s discovery simultaneously against the same
services is a rare edge case. Premature conflict resolution adds complexity for a scenario that
barely exists in practice.

### Dependency — client-go Direct

`k8s.io/client-go` added directly to `go.mod`. No build tag isolation. One binary does
everything. The dependency weight is a cost we accept — graph-go is typically containerized.

Binary size optimization: `go build -ldflags="-s -w"` strips debug info and symbols (~20-30%
reduction).

### Testing — kind/k3d Integration Tests

Integration tests use **kind or k3d** to spin up a real local K8s cluster. Deploy a small set
of test workloads (a deployment, a statefulset, a service), validate discovery against them.

Gated with `//go:build integration`, consistent with the existing adapter test strategy.
No mocks — real infrastructure.

---

## Implementation Phases

### Phase 1 — Discoverer Interface + Docker Refactor

**Goal:** Extract the Discoverer contract and refactor Docker to implement it. Pure refactor —
no new behavior, all existing tests must pass.

**Tasks:**
1. Define `Discoverer` interface and `ServiceInfo` in `discovery/discovery.go`
2. Create `discovery/docker/` subpackage, move existing Docker files into it:
   - `docker.go` → `discovery/docker/docker.go`
   - `classifier.go` → `discovery/docker/classifier.go`
   - `credentials.go` → `discovery/docker/credentials.go`
   - `events.go` → `discovery/docker/events.go`
   - `labels.go` → `discovery/docker/labels.go`
3. Implement `Discoverer` interface on the Docker discoverer
4. Update `merge.go` to work with `[]ServiceInfo` concatenation
5. Update server setup to use the `Discoverer` interface
6. Fix all import paths across the project
7. Verify all existing tests pass

**Key instruction:** This is a mechanical refactor. Don't change behavior, don't optimize, don't
"improve" anything along the way. The only goal is clean extraction of the interface.

### Phase 2 — Kubernetes Discoverer (Topology)

**Goal:** Implement K8s discovery for Tier 1 resources with informer-based watching.

**Tasks:**
1. Add `k8s.io/client-go` dependency
2. Create `discovery/kubernetes/kubernetes.go`:
   - Implement `Discoverer` interface
   - Auto-detection: in-cluster → kubeconfig fallback
   - `Discover()` lists Tier 1 resources, delegates to mapper
   - `Watch()` sets up informers with debounced `onChange` callback
   - `Close()` stops informer factory
3. Create `discovery/kubernetes/mapper.go`:
   - Map K8s resources to `nodes.Node` and `edges.Edge`
   - Generate node IDs with `k8s-{namespace}-{kind}-{name}` prefix pattern
   - Resolve Service → Pod edges via label selector matching
   - Mark namespace nodes with `"group": true` metadata
   - Retain pod metadata (image, env, labels) for future Level 2
4. Create `discovery/kubernetes/health.go`:
   - Pod health from phase + container statuses
   - Workload health from replica counts
   - Service health aggregated from target pods
   - Namespace health aggregated from children
5. Add new node types to `graph/nodes/nodes.go`:
   - `TypeNamespace`, `TypeDeployment`, `TypeStatefulSet`, `TypeDaemonSet`, `TypePod`,
     `TypeK8sService`
6. Add `routes_to` edge type to `graph/edges/edges.go`
7. Integration tests with kind/k3d:
   - Deploy test workloads (deployment + statefulset + service)
   - Validate node discovery (correct types, IDs, hierarchy)
   - Validate edge discovery (contains + routes_to)
   - Validate health mapping
   - Use contract test pattern where applicable

### Phase 3 — Server Integration

**Goal:** Wire K8s discoverer into the server lifecycle with parallel startup.

**Tasks:**
1. Update server setup to detect and start all available discoverers in parallel (goroutines + sync)
2. Concatenate all `ServiceInfo` results into the registry
3. Wire K8s informer events into cache invalidation (with debounce)
4. Add optional `kubernetes:` config section for overrides
5. Update config validation for K8s fields
6. WebSocket pushes K8s topology changes and health updates
7. Graceful shutdown closes all discoverers

### Phase 4 — Frontend

**Goal:** Render K8s resources with proper visual treatment.

**Tasks:**
1. Add new node types + icons/styles for K8s resources
2. Implement group container rendering for namespaces (React Flow grouping)
3. Style `routes_to` edges distinctly from `contains` and `foreign_key`
4. Add source-based filtering (show Docker / K8s / both)
5. Ensure node inspector works with K8s node metadata

---

## Future Work (Not In Scope)

These features depend on the orchestrator foundation but are **not part of this plan**:

- **Level 2 adapter bridging:** Classify K8s pods by image, extract credentials, connect
  adapters to discover internals (tables, collections, etc.)
- **Flow observability:** Real-time data flow visualization showing bottlenecks and degradation
  paths across services
- **Integrated stress trigger:** One-click k6 execution in non-prod environments with real-time
  impact visualization
- **Raw event mode:** Disable debounce flag for watching every K8s event during stress tests
- **Additional orchestrators:** ECS, Nomad — follow the same Discoverer interface
