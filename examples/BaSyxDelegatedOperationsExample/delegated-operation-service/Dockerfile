FROM golang:1.25.9-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN go build -o delegated-operation-service ./main.go

FROM alpine:3.22

WORKDIR /app
COPY --from=builder /src/delegated-operation-service ./delegated-operation-service

EXPOSE 8080
ENTRYPOINT ["/app/delegated-operation-service"]
