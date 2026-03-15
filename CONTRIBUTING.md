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
cd webui && npx tsc --noEmit
```

---

## Submitting Changes

### PR Checklist

- [ ] `make lint` passes
- [ ] `make test` passes
- [ ] `npx tsc --noEmit` passes
- [ ] Commit messages use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`)
- [ ] New adapters include discovery logic and health checks
- [ ] Documentation updated if applicable

---

## Adding a New Adapter

See [README.md](README.md#adding-a-new-adapter) for the step-by-step guide.

---

## Use Scope

**Intended:** New adapters, UI improvements, performance optimizations, bug fixes, documentation.

**Not Accepted:** Features that bypass auth, unauthorized scanning tools, or code that violates the intended use case.

---

## Questions?

1. Check existing [Issues](https://github.com/yourusername/graph-info/issues)
2. Start a [Discussion](https://github.com/yourusername/graph-info/discussions)
