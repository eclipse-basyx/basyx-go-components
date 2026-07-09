# syntax=docker/dockerfile:1

# Stage 1: Build the application
FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

# Install git and SSL certificates for package downloads
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/digitaltwinregistryservice ./cmd/digitaltwinregistryservice
COPY internal ./internal
COPY pkg ./pkg

# Build the application
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o digitaltwinregistryservice ./cmd/digitaltwinregistryservice

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o healthprobe ./internal/healthprobe

# Stage 2: Distroless runtime image
FROM gcr.io/distroless/static:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240

WORKDIR /app

# Copy runtime artifacts
COPY --from=builder /app/digitaltwinregistryservice /app/digitaltwinregistryservice
COPY --from=builder /app/cmd/digitaltwinregistryservice/config.yaml /config/config.yaml

COPY --from=builder /app/healthprobe /bin/healthprobe
COPY --from=builder /app/healthprobe /bin/wget

# Default port (can be overridden by environment)
ENV SERVER_PORT=5000

# Expose the service port (this is just documentation, the actual port is determined by the environment)
EXPOSE ${SERVER_PORT}

# Command to run the application
# Use environment variables or mount a custom config file to /config/config.yaml
CMD ["/app/digitaltwinregistryservice", "-config", "/config/config.yaml"]

# Health check runs via healthprobe
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/healthprobe"]
