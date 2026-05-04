# Stage 1: Build the application
FROM golang:1.25.9-alpine AS builder

ARG VERSION=dev

RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X main.Version=${VERSION} -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o aasenvironmentservice ./cmd/aasenvironmentservice

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -o healthprobe ./internal/healthprobe

FROM gcr.io/distroless/static:nonroot

WORKDIR /app

COPY --from=builder /app/aasenvironmentservice /app/aasenvironmentservice
COPY --from=builder /app/cmd/aasenvironmentservice/config.yaml /config/config.yaml
COPY --from=builder /app/basyxschema.sql /app/basyxschema.sql
COPY --from=builder /app/healthprobe /bin/healthprobe
COPY --from=builder /app/healthprobe /bin/wget

ENV SERVER_PORT=5004

EXPOSE ${SERVER_PORT}

CMD ["/app/aasenvironmentservice", "-config", "/config/config.yaml", "-databaseSchema", "/app/basyxschema.sql"]

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/bin/healthprobe"]
