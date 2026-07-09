# syntax=docker/dockerfile:1

# Stage 1: Build the application
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS builder

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

COPY cmd/submodelrepositoryservice ./cmd/submodelrepositoryservice
COPY internal ./internal
COPY pkg ./pkg

# Build the application
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o submodelrepositoryservice ./cmd/submodelrepositoryservice

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o healthprobe ./internal/healthprobe

# Stage 2: Distroless runtime image
FROM gcr.io/distroless/static:nonroot@sha256:d29e660cc75a5b6b1334e03c5c81ccf9bc0884a002c6000dbf0fb96034814478

WORKDIR /app

# Copy runtime artifacts
COPY --from=builder /app/submodelrepositoryservice /app/submodelrepositoryservice
COPY --from=builder /app/cmd/submodelrepositoryservice/config.yaml /config/config.yaml

COPY --from=builder /app/healthprobe /bin/healthprobe
COPY --from=builder /app/healthprobe /bin/wget

# Default port (can be overridden by environment)
ENV SERVER_PORT=5000

# Expose the service port (this is just documentation, the actual port is determined by the environment)
EXPOSE ${SERVER_PORT}

# Command to run the application
# Use environment variables or mount a custom config file to /config/config.yaml
CMD ["/app/submodelrepositoryservice", "-config", "/config/config.yaml"]

# Health check runs via healthprobe
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/healthprobe"]
