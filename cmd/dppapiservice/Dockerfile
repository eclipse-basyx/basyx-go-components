# syntax=docker/dockerfile:1

# Stage 1: Build the application
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS builder

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/dppapiservice ./cmd/dppapiservice
COPY internal ./internal
COPY pkg ./pkg

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o dppapiservice ./cmd/dppapiservice

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o healthprobe ./internal/healthprobe

# Stage 2: Distroless runtime image
FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

WORKDIR /app

COPY --from=builder /app/dppapiservice /app/dppapiservice
COPY --from=builder /app/cmd/dppapiservice/config.yaml /config/config.yaml

COPY --from=builder /app/healthprobe /bin/healthprobe
COPY --from=builder /app/healthprobe /bin/wget

ENV SERVER_PORT=8080

EXPOSE ${SERVER_PORT}

CMD ["/app/dppapiservice", "-config", "/config/config.yaml"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/healthprobe"]
