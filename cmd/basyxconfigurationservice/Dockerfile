# Stage 1: Build the application
FROM golang:1.25.9-alpine AS builder

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
    -o basyxconfigurationservice ./cmd/basyxconfigurationservice

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -o healthprobe ./internal/healthprobe

# Stage 2: Distroless runtime image
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

# Copy runtime artifacts
COPY --from=builder /app/basyxconfigurationservice /app/basyxconfigurationservice
COPY --from=builder /app/database/3_1_full.sql /app/3_1_full.sql
COPY --from=builder /app/healthprobe /bin/healthprobe
COPY --from=builder /app/healthprobe /bin/wget

# Command to run the application
CMD ["/app/basyxconfigurationservice"]

# Health check runs via healthprobe
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/healthprobe"]
