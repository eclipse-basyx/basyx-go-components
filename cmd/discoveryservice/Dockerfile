# Stage 1: Build the application
FROM golang:1.25-alpine AS builder

# Version argument for build-time injection
ARG VERSION=dev

RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates
WORKDIR /app

# Dependencies first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy full source
COPY . .

# Build discoveryservice binary with version information
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X main.Version=${VERSION} -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o discoveryservice ./cmd/discoveryservice

# Stage 2: Distroless runtime image
FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

# Copy runtime artifacts
COPY --from=builder /app/discoveryservice /app/discoveryservice
COPY --from=builder /app/cmd/discoveryservice/config.yaml /config/config.yaml
COPY --from=builder /app/basyxschema.sql /app/basyxschema.sql
COPY --from=builder /bin/busybox /bin/busybox
COPY --from=builder --chmod=0755 /app/cmd/wget-compat.sh /bin/wget

# Default port (override via env in compose)
ENV SERVER_PORT=5004

EXPOSE ${SERVER_PORT}

# Start service with config
CMD ["/app/discoveryservice", "-config", "/config/config.yaml", "-databaseSchema", "/app/basyxschema.sql"]

# Health check using BusyBox for distroless compatibility
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/busybox", "sh", "-c", "PORT=${SERVER_PORT:-5004}; CONTEXT_PATH=${SERVER_CONTEXTPATH:-}; if [ -z \"$CONTEXT_PATH\" ]; then HEALTH_URL=\"http://127.0.0.1:${PORT}/health\"; else HEALTH_URL=\"http://127.0.0.1:${PORT}${CONTEXT_PATH}/health\"; fi; /bin/busybox wget --spider -q \"$HEALTH_URL\""]
