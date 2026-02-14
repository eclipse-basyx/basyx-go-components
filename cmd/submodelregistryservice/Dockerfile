# Stage 1: Build the application
FROM golang:1.25-alpine AS builder

# Version argument for build-time injection
ARG VERSION=dev

# Install git and SSL certificates for package downloads
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the entire project
COPY . .

# Build the application with version information
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X main.Version=${VERSION} -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o submodelregistryservice ./cmd/submodelregistryservice

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -o healthprobe ./internal/healthprobe

# Stage 2: Distroless runtime image
FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

# Copy runtime artifacts
COPY --from=builder /app/submodelregistryservice /app/submodelregistryservice
COPY --from=builder /app/cmd/submodelregistryservice/config.yaml /config/config.yaml
COPY --from=builder /app/basyxschema.sql /app/basyxschema.sql
COPY --from=builder /app/healthprobe /bin/healthprobe
COPY --from=builder /app/healthprobe /bin/wget

# Default port (can be overridden by environment)
ENV SERVER_PORT=5004

# Expose the service port (this is just documentation, the actual port is determined by the environment)
EXPOSE ${SERVER_PORT}

# Command to run the application
# Use environment variables or mount a custom config file to /config/config.yaml
CMD ["/app/submodelregistryservice", "-config", "/config/config.yaml", "-databaseSchema", "/app/basyxschema.sql"]

# Health check using BusyBox for distroless compatibility
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/healthprobe"]
