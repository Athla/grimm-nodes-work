.PHONY: help install install-backend install-frontend \
	dev run-backend run-frontend watch-backend \
	build build-backend build-backend-only build-frontend \
	test test-backend test-frontend lint clean \
	docker-build docker-up docker-down docker-logs docker-clean

COMPOSE_FILE := docker-compose.demo.yml
COMPOSE := docker compose -f $(COMPOSE_FILE)

.DEFAULT_GOAL := help

## help: Display this help message
help:
	@echo "Available commands:"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':' | sort

## install: Install all dependencies (backend + frontend)
install: install-backend install-frontend

## install-backend: Install Go dependencies
install-backend:
	@echo "Installing Go dependencies..."
	@go mod download && go mod tidy

## install-frontend: Install npm dependencies
install-frontend:
	@echo "Installing npm dependencies..."
	@cd webui && npm install

## dev: Run backend and frontend concurrently (requires 'concurrently' or run in separate terminals)
dev:
	@echo "Starting backend and frontend..."
	@echo "Backend: http://localhost:8080"
	@echo "Frontend: http://localhost:5173"
	@(trap 'kill 0' INT; \
		make run-backend & \
		make run-frontend & \
		wait)

## run-backend: Run the backend server
run-backend:
	@echo "Starting backend server on port 8080..."
	@go run ./cmd/app/main.go

## watch-backend: Run backend with air hot-reload (installs air if missing)
watch-backend:
	@if command -v air > /dev/null; then \
		air; \
	else \
		read -p "'air' is not installed. Install it now? [Y/n] " choice; \
		if [ "$$choice" != "n" ] && [ "$$choice" != "N" ]; then \
			go install github.com/air-verse/air@latest && air; \
		else \
			echo "Skipping."; exit 1; \
		fi; \
	fi

## run-frontend: Run the frontend dev server
run-frontend:
	@echo "Starting frontend dev server on port 5173..."
	@cd webui && npm run dev

## build: Build the single self-contained binary (frontend bundle embedded)
build: build-backend

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)

## build-backend: Build the backend binary with the SPA bundle embedded
build-backend: build-frontend
	@echo "Building backend..."
	@go build -ldflags "$(LDFLAGS)" -o bin/graph-go ./cmd/app
	@echo "Backend binary created at: bin/graph-go"

## build-backend-only: Build backend without rebuilding the frontend (fast iteration)
build-backend-only:
	@echo "Building backend (skipping frontend bundle)..."
	@go build -ldflags "$(LDFLAGS)" -o bin/graph-go ./cmd/app
	@echo "Backend binary created at: bin/graph-go"

## build-frontend: Build the frontend bundle and stage it for embed
build-frontend:
	@echo "Building frontend..."
	@cd webui && npm run build
	@echo "Staging bundle into internal/webui/dist/ for embed..."
	@rm -rf internal/webui/dist
	@mkdir -p internal/webui/dist
	@cp -R webui/dist/. internal/webui/dist/
	@touch internal/webui/dist/.gitkeep
	@echo "Frontend bundle staged at: internal/webui/dist"

## test: Run all tests (backend + frontend)
test: test-backend test-frontend

## test-backend: Run Go tests
test-backend:
	@echo "Running Go tests..."
	@go test ./... -v -count=1

## test-frontend: Run TypeScript type checking
test-frontend:
	@echo "Running TypeScript type checking..."
	@cd webui && npx tsc --noEmit

## lint: Run golangci-lint on the backend
lint:
	@echo "Running golangci-lint..."
	@golangci-lint run ./...

## clean: Remove build artifacts and dependencies
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -rf tmp/
	@rm -rf webui/dist/
	@rm -rf internal/webui/dist/*
	@touch internal/webui/dist/.gitkeep
	@rm -rf webui/node_modules/
	@echo "✓ Clean complete"

## docker-build: Build the demo stack image
docker-build:
	$(COMPOSE) build

## docker-up: Start the seeded demo stack
docker-up:
	@if [ ! -f conf/config.yaml ]; then \
		echo "conf/config.yaml not found, copying from config.docker.yaml..."; \
		cp conf/config.docker.yaml conf/config.yaml; \
	fi
	$(COMPOSE) up -d
	@echo ""
	@echo "Demo stack started:"
	@echo "  graph-go:      http://localhost:8080"
	@echo "  MinIO Console: http://localhost:9001"

## docker-down: Stop the demo stack
docker-down:
	$(COMPOSE) down

## docker-logs: Follow logs from the demo stack
docker-logs:
	$(COMPOSE) logs -f

## docker-clean: Stop the demo stack and remove volumes and local images
docker-clean:
	$(COMPOSE) down -v --rmi local
