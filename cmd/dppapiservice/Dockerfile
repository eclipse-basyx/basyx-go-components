# Stage 1: Build the application
FROM golang:1.26.4-alpine AS builder

ARG VERSION=dev

RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X main.Version=${VERSION} -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o dppapiservice ./cmd/dppapiservice

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -o healthprobe ./internal/healthprobe

# Stage 2: Distroless runtime image
FROM gcr.io/distroless/static:nonroot

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
