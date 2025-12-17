# ---------- Build stage ----------
FROM golang:1.25-alpine3.22 AS builder

WORKDIR /app

# Install git (needed for go modules in some environments)
RUN apk add --no-cache git

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source
COPY . .

# Build the AAS repository service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o aasrepositoryservice ./cmd/aasrepositoryservice


# ---------- Runtime stage ----------
FROM alpine:3.19

WORKDIR /app

# Add CA certificates (important for HTTPS / DB drivers)
RUN apk add --no-cache ca-certificates

# Copy binary
COPY --from=builder /app/aasrepositoryservice .

# Copy resources if needed
COPY cmd/aasrepositoryservice/resources ./resources

# Expose port (informational; not required to map)
EXPOSE 5105

# Run the service
ENTRYPOINT ["./aasrepositoryservice"]
