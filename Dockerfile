# ---- Frontend bundle ----
FROM oven/bun:1 AS frontend-build
WORKDIR /app
COPY webui/package.json webui/bun.lock ./
RUN bun install
COPY webui/ ./
RUN bun run build

# ---- Backend build (embeds the frontend bundle) ----
FROM golang:1.25-bookworm AS backend-build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# Stage the frontend bundle into the embed tree before `go build`.
RUN rm -rf internal/webui/dist && mkdir -p internal/webui/dist
COPY --from=frontend-build /app/dist/ /app/internal/webui/dist/
RUN CGO_ENABLED=1 go build -o /graph-go ./cmd/app/main.go

# ---- Runtime ----
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
RUN useradd --create-home --shell /bin/bash appuser
WORKDIR /app
COPY --from=backend-build /graph-go ./graph-go
RUN mkdir -p conf data && chown -R appuser:appuser /app
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1
CMD ["./graph-go"]
