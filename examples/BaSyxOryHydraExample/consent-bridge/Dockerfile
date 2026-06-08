FROM golang:1.26.4-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /consent-bridge .

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /consent-bridge /consent-bridge

ENTRYPOINT ["/consent-bridge"]
