# Contributing to graph-info

---

## Code of Conduct

This project is intended for legitimate infrastructure visualization and monitoring purposes. Contributors must:

- Only develop features for authorized infrastructure access
- Not contribute code designed to circumvent authentication or authorization
- Follow responsible disclosure practices for any security issues discovered
- Respect the intended use case: helping engineers visualize and monitor systems they own or have permission to access

---

## Getting Started

See [README.md](README.md#local-development-setup) for prerequisites, installation, and running locally.

### Fork and Clone

```bash
git clone https://github.com/YOUR_USERNAME/graph-info.git
cd graph-info
git remote add upstream https://github.com/original/graph-info.git
```

---

## Development Workflow

### Branching

- `main` — Production-ready code
- `feature/your-feature-name` — New features
- `fix/bug-description` — Bug fixes

### Code Style

**Go:** Follow standard conventions (`gofmt`, `go vet`). Descriptive names, early returns, functions under 50 lines.

**TypeScript:** Strict mode, no `any`, functional components with hooks, components under 200 lines.

### Before Committing

```bash
cd binary && go fmt ./... && go vet ./...
cd binary && go test ./...                                                    # unit tests
cd binary && go test -tags=integration -timeout=5m ./internal/adapters/...    # integration tests (requires Docker)
cd webui && npx tsc --noEmit
```

---

## Submitting Changes

### PR Checklist

- [ ] `make lint` passes
- [ ] `make test` passes
- [ ] `go test -tags=integration -timeout=5m ./internal/adapters/...` passes (if touching adapters)
- [ ] `npx tsc --noEmit` passes
- [ ] Commit messages use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`)
- [ ] New adapters include discovery logic, health checks, **and integration tests**
- [ ] Documentation updated if applicable

---

## Adding a New Adapter

See [README.md](README.md#adding-a-new-adapter) for the full step-by-step guide.

### Integration Tests (Required)

Every adapter **must** have integration tests. This is non-negotiable — it's how we ensure adapters work against real infrastructure, not just in theory.

1. Create `{name}_integration_test.go` in your adapter package
2. Add `//go:build integration` as the first line
3. Use `TestMain` with [testcontainers-go](https://golang.testcontainers.org/) to start a real instance of the service
4. Seed the container with representative data (tables, indices, keys, buckets, etc.)
5. Call `adaptertest.RunContractTests` — this validates your adapter against the shared contract (node/edge integrity, health metrics, connect/close lifecycle)
6. Add adapter-specific tests for any unique behavior (filtering, ID format, metadata)

Example pattern (see any existing adapter test for reference):

```go
//go:build integration

package myadapter

import (
    "binary/internal/adapters"
    "binary/internal/adapters/adaptertest"
    // testcontainers module + driver imports
)

var (
    testAdapter adapters.Adapter
    testConfig  adapters.ConnectionConfig
)

func TestMain(m *testing.M) {
    // 1. Start container with testcontainers-go
    // 2. Seed data
    // 3. Connect adapter
    // 4. m.Run()
    // 5. Cleanup
}

func TestContract(t *testing.T) {
    adaptertest.RunContractTests(t, testAdapter,
        func() adapters.Adapter { return New() },
        testConfig,
        adaptertest.ContractOpts{
            MinNodes:           ...,
            MinEdges:           ...,
            RootNodeType:       "...",
            ChildNodeTypes:     []string{"..."},
            RequiredHealthKeys: []string{"status", ...},
        },
    )
}
```

Run your tests with:

```bash
cd binary && go test -tags=integration -v ./internal/adapters/{name}/
```

---

## Use Scope

**Intended:** New adapters, UI improvements, performance optimizations, bug fixes, documentation.

**Not Accepted:** Features that bypass auth, unauthorized scanning tools, or code that violates the intended use case.

---

## Questions?

1. Check existing [Issues](https://github.com/yourusername/graph-info/issues)
2. Start a [Discussion](https://github.com/yourusername/graph-info/discussions)
