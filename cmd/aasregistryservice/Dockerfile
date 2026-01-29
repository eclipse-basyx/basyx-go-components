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
    -o aasregistryservice ./cmd/aasregistryservice

# Stage 2: Create a minimal image to run the application
FROM alpine

# Install certificates and tools for HTTPS and health checks
RUN apk --no-cache --no-scripts add ca-certificates wget

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/aasregistryservice .

# Create a directory for configuration
RUN mkdir -p /config

# Copy default configuration
COPY --from=builder /app/cmd/aasregistryservice/config.yaml /config/config.yaml
COPY --from=builder /app/basyxschema.sql /root/basyxschema.sql

# Default port (can be overridden by environment)
ENV SERVER_PORT=5000

# Create a healthcheck script
COPY --from=builder /app/cmd/aasregistryservice/healthcheck.sh /root/healthcheck.sh
RUN chmod +x /root/healthcheck.sh

# Expose the service port (this is just documentation, the actual port is determined by the environment)
EXPOSE ${SERVER_PORT}

# Command to run the application
# Use environment variables or mount a custom config file to /config/config.yaml
CMD ["./aasregistryservice", "-config", "/config/config.yaml", "-databaseSchema", "basyxschema.sql"]

# Health check using the script
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD /root/healthcheck.sh