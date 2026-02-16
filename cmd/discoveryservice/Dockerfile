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

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -o healthprobe ./internal/healthprobe

# Stage 2: Distroless runtime image
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

# Copy runtime artifacts
COPY --from=builder /app/discoveryservice /app/discoveryservice
COPY --from=builder /app/cmd/discoveryservice/config.yaml /config/config.yaml
COPY --from=builder /app/basyxschema.sql /app/basyxschema.sql
COPY --from=builder /app/healthprobe /bin/healthprobe
COPY --from=builder /app/healthprobe /bin/wget
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Default port (override via env in compose)
ENV SERVER_PORT=5004
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

EXPOSE ${SERVER_PORT}

# Start service with config
CMD ["/app/discoveryservice", "-config", "/config/config.yaml", "-databaseSchema", "/app/basyxschema.sql"]

# Health check runs via healthprobe
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/healthprobe"]
