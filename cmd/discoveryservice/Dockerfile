# Stage 1: Build the application
FROM golang:1.25-alpine3.22 AS builder

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

# Stage 2: Minimal runtime image
FROM alpine:3.22

RUN apk --no-cache --no-scripts add ca-certificates wget
WORKDIR /root/

# Copy binary
COPY --from=builder /app/discoveryservice /root/discoveryservice

# Config + schema expected by your code:
# Your code reads schema from: getwd() + "/resources/sql/discoveryschema.sql"
# With WORKDIR=/root, that becomes /root/resources/sql/discoveryschema.sql
RUN mkdir -p /config /root/resources/sql

# Copy default config and SQL schema for discovery service
COPY --from=builder /app/cmd/discoveryservice/config.yaml /config/config.yaml
COPY --from=builder /app/cmd/discoveryservice/resources/sql/discoveryschema.sql /root/resources/sql/discoveryschema.sql

# Optional healthcheck script for /health
# If you have one in the discoveryservice folder, copy it; otherwise you can omit.
COPY --from=builder /app/cmd/discoveryservice/healthcheck.sh /root/healthcheck.sh
RUN chmod +x /root/healthcheck.sh || true

# Default port (override via env in compose)
ENV SERVER_PORT=5004

# NOTE: EXPOSE doesn't accept env vars at runtime; this is just documentation.
EXPOSE 5004

# Start service with config
CMD ["/root/discoveryservice", "-config", "/config/config.yaml"]

# Health check (if script exists and your service serves /health)
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD /root/healthcheck.sh || wget -qO- http://127.0.0.1:${SERVER_PORT}/health || exit 1
