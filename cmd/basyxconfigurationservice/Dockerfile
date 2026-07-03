# syntax=docker/dockerfile:1

# Stage 1: Build the application
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine AS builder

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

COPY cmd/basyxconfigurationservice ./cmd/basyxconfigurationservice
COPY internal ./internal
COPY pkg ./pkg
COPY database ./database

# Build the application
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o basyxconfigurationservice ./cmd/basyxconfigurationservice

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o healthprobe ./internal/healthprobe

# Stage 2: Distroless runtime image
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

# Copy runtime artifacts
COPY --from=builder /app/basyxconfigurationservice /app/basyxconfigurationservice
COPY --from=builder /app/database/ /app/
COPY --from=builder /app/healthprobe /bin/healthprobe
COPY --from=builder /app/healthprobe /bin/wget

# Command to run the application
CMD ["/app/basyxconfigurationservice"]

# Health check runs via healthprobe
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/healthprobe"]
