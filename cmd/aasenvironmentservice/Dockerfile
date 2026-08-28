# syntax=docker/dockerfile:1

# Stage 1: Build the application
FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates
WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/aasenvironmentservice ./cmd/aasenvironmentservice
COPY internal ./internal
COPY pkg ./pkg

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o aasenvironmentservice ./cmd/aasenvironmentservice

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -trimpath \
    -o healthprobe ./internal/healthprobe

FROM gcr.io/distroless/static:nonroot@sha256:1c2c046bc09ed40fad370b599a0b1ae7987f55b01e247cf27a7c27cd97e5bbc7

WORKDIR /app

COPY --from=builder /app/aasenvironmentservice /app/aasenvironmentservice
COPY --from=builder /app/cmd/aasenvironmentservice/config.yaml /config/config.yaml

COPY --from=builder /app/healthprobe /bin/healthprobe
COPY --from=builder /app/healthprobe /bin/wget

ENV SERVER_PORT=5004

EXPOSE ${SERVER_PORT}

CMD ["/app/aasenvironmentservice", "-config", "/config/config.yaml"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/healthprobe"]
