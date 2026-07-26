# BaSyx Observability Example

This development example builds BaSyx from the current checkout and connects
request traces and structured logs to a local observability stack:

- AAS Environment Service and BaSyx Configuration Service
- PostgreSQL
- OpenTelemetry Collector `0.157.0`
- Jaeger `2.20.0` with in-memory trace storage
- Loki `3.7.4`
- Grafana Alloy `1.18.0`
- Grafana `13.1.1`

BaSyx sends OTLP/HTTP traces to the Collector, which batches and forwards them
to Jaeger. BaSyx JSON logs remain on stderr; Alloy reads the explicitly labelled
BaSyx containers, parses their JSON timestamps, attaches trace and request
identifiers as non-indexed structured metadata, and sends the records to Loki.
Grafana provisions both data sources and trace-to-log/log-to-trace links.

## Prerequisites

- Docker with Docker Compose
- `curl`
- Python 3 for the smoke verifier
- Free loopback ports `3001`, `3100`, `8083`, and `16686`

## Start

From this directory:

```sh
docker compose up -d --build
```

The two BaSyx images are built locally from `../..`. Compose pulls only the
external infrastructure and base images.

## Verify

Run the bounded end-to-end verifier:

```sh
./smoke.sh
```

It sends a request with known W3C trace, request, and correlation IDs, then
confirms:

1. BaSyx returns the canonical request and correlation headers.
2. Jaeger contains the trace.
3. Loki contains the matching structured `HTTP request completed` record.

The optional outage check also stops the Collector briefly and verifies that
BaSyx continues serving requests:

```sh
./smoke.sh --check-collector-outage
```

## Open the UIs

- Grafana: [http://127.0.0.1:3001](http://127.0.0.1:3001)
- Jaeger: [http://127.0.0.1:16686](http://127.0.0.1:16686)
- AAS Environment: [http://127.0.0.1:8083](http://127.0.0.1:8083)

Grafana allows anonymous viewer access in this example. In Explore, select
Loki to inspect structured logs or Jaeger to inspect traces. Log records with a
`trace_id` include a Jaeger link, and trace views offer a Loki query for the
same service, time range, and trace ID.

## Stop

```sh
docker compose down
```

Remove the local volumes as well:

```sh
docker compose down -v
```

## Development-only Security and Storage

This stack is not a production deployment reference:

- All published ports bind to loopback.
- Grafana anonymous access is enabled.
- The AAS Environment has ABAC disabled.
- Jaeger stores traces only in memory.
- Loki uses local container filesystem storage.
- Alloy mounts the Docker socket read-only. Docker socket access is still
  highly privileged because it exposes container metadata and log streams.
- The example has no TLS, authentication between observability components,
  retention policy, redundancy, or resource limits.

Use authenticated endpoints, least-privilege collection, durable storage,
retention controls, resource limits, and platform-specific secret management
for deployed environments.
