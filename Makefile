.PHONY: help install install-backend install-frontend \
	dev run-backend run-frontend watch-backend \
	build build-backend build-frontend \
	test test-backend test-frontend clean \
	docker-build docker-up docker-down docker-logs docker-clean

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

## build: Build both backend and frontend for production
build: build-backend build-frontend

## build-backend: Build the backend binary
build-backend:
	@echo "Building backend..."
	@go build -o bin/graph-go ./cmd/app/main.go
	@echo "Backend binary created at: bin/graph-go"

## build-frontend: Build the frontend for production
build-frontend:
	@echo "Building frontend..."
	@cd webui && npm run build
	@echo "Frontend build created at: webui/dist"

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

## clean: Remove build artifacts and dependencies
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -rf tmp/
	@rm -rf webui/dist/
	@rm -rf webui/node_modules/
	@echo "✓ Clean complete"

## docker-build: Build Docker images
docker-build:
	docker compose build

## docker-up: Start all services with Docker Compose
docker-up:
	@if [ ! -f conf/config.yaml ]; then \
		echo "conf/config.yaml not found, copying from config.docker.yaml..."; \
		cp conf/config.docker.yaml conf/config.yaml; \
	fi
	docker compose up -d
	@echo ""
	@echo "Services started:"
	@echo "  Frontend:      http://localhost:3000"
	@echo "  Backend API:   http://localhost:8080"
	@echo "  MinIO Console: http://localhost:9001"

## docker-down: Stop all services
docker-down:
	docker compose down

## docker-logs: Follow logs from all services
docker-logs:
	docker compose logs -f

## docker-clean: Stop services and remove volumes and local images
docker-clean:
	docker compose down -v --rmi local
