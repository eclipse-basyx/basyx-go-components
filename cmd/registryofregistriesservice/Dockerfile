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
    -o registryofregistriesservice ./cmd/registryofregistriesservice

# Stage 2: Minimal runtime image
FROM alpine

RUN apk --no-cache add ca-certificates wget
WORKDIR /root/

# Copy binary
COPY --from=builder /app/registryofregistriesservice .

# Create a directory for configuration
RUN mkdir -p /config

# Copy default configuration
COPY --from=builder /app/cmd/registryofregistriesservice/config.yaml /config/config.yaml
COPY --from=builder /app/basyxschema.sql /root/basyxschema.sql

# Default port (can be overridden by environment)
ENV SERVER_PORT=5004

# Create a healthcheck
COPY --from=builder /app/cmd/registryofregistriesservice/healthcheck.sh /root/healthcheck.sh
RUN chmod +x /root/healthcheck.sh

# Expose the service port (this is just documentation, the actual port is determined by the environment)
EXPOSE ${SERVER_PORT}

# Command to run the application
# Use environment variables or mount a custom config file to /config/config.yaml
CMD ["./registryofregistriesservice", "-config", "/config/config.yaml", "-databaseSchema", "basyxschema.sql"]

# Health check (if script exists and your service serves /health)
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD /root/healthcheck.sh
