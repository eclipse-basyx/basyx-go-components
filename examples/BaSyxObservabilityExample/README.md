# BaSyx Observability Example

This development example connects BaSyx SNAPSHOT images, request traces, and
structured logs to a local observability stack:

- AAS Environment Service `SNAPSHOT`
- BaSyx Configuration Service `SNAPSHOT`
- BaSyx Web UI `SNAPSHOT-20260724-064425-2b74f32`
- PostgreSQL
- OpenTelemetry Collector `0.157.0`
- Tempo `3.0.2`
- Loki `3.7.4`
- Grafana Alloy `1.18.0`
- Grafana `13.1.1`

BaSyx sends OTLP/HTTP traces to the Collector, which batches and forwards them
to Tempo. BaSyx JSON logs remain on stderr; Alloy reads the explicitly labelled
BaSyx containers, parses their JSON timestamps, attaches trace and request
identifiers as non-indexed structured metadata, and sends the records to Loki.
Grafana provisions both data sources and trace-to-log/log-to-trace links.

## Prerequisites

- Docker with Docker Compose
- `curl`
- Python 3 for the smoke verifier
- Free loopback ports `3000`, `3001`, `3100`, `3200`, and `8083`

## Start

From this directory:

```sh
docker compose up -d
```

Compose pulls the BaSyx Go SNAPSHOT images, the BaSyx Web UI, and the external
infrastructure images. The examples smoke workflow builds the two Go SNAPSHOT
images from the checked-out source and starts Compose with pulling disabled.

The AAS Environment automatically imports
[`IESEDriveMotorDM3000.aasx`](aas/IESEDriveMotorDM3000.aasx). Open the Web UI,
select **IESEDriveMotorDM3000**, and browse its submodels to generate realistic
request traces and correlated access logs.

## Verify

Run the bounded end-to-end verifier:

```sh
./smoke.sh
```

It sends a request with known W3C trace, request, and correlation IDs, then
confirms:

1. The BaSyx Web UI is reachable.
2. Grafana Explore is available to the anonymous development user.
3. The preconfigured `IESEDriveMotorDM3000` AAS is available.
4. BaSyx returns the canonical request and correlation headers.
5. Tempo contains the trace and can calculate TraceQL metrics from it.
6. Loki contains the matching structured `HTTP request completed` record.

The optional outage check also stops the Collector briefly and verifies that
BaSyx continues serving requests:

```sh
./smoke.sh --check-collector-outage
```

## Open the UIs

- BaSyx Web UI: [http://127.0.0.1:3000](http://127.0.0.1:3000)
- Grafana: [http://127.0.0.1:3001](http://127.0.0.1:3001)
- AAS Environment: [http://127.0.0.1:8083](http://127.0.0.1:8083)

Grafana allows anonymous editor access in this example so Explore is available
without a login. In Explore, select Loki to inspect structured logs or Tempo to
inspect traces with TraceQL. The **Drilldown > Traces** page uses the same Tempo
data source for a service-oriented view. Log records with a `trace_id` include a
Tempo link, and trace views offer a Loki query for the
same service, time range, and trace ID. Requests made while browsing the
preconfigured AAS in the BaSyx Web UI appear in both data sources.

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
- Grafana anonymous Editor access is enabled so users can query data through
  Explore. Anonymous users can also modify Grafana resources in this
  development stack.
- The BaSyx Web UI and AAS Environment are unsecured.
- The AAS Environment has ABAC disabled.
- Tempo stores traces in a local Docker volume.
- Loki uses local container filesystem storage.
- Alloy mounts the Docker socket read-only. Docker socket access is still
  highly privileged because it exposes container metadata and log streams.
- The example has no TLS, authentication between observability components,
  retention policy, redundancy, or resource limits.

Use authenticated endpoints, least-privilege collection, durable storage,
retention controls, resource limits, and platform-specific secret management
for deployed environments.
