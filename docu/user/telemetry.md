# OpenTelemetry Tracing

The 11 BaSyx commands that expose HTTP APIs support optional OpenTelemetry
tracing. Tracing is configured only with standard OpenTelemetry environment
variables; it does not add a BaSyx YAML section.

`basyxconfigurationservice` and `historyevidenceverifier` do not expose HTTP
servers or traced operations, so they remain logging-only commands.

## Activation

Tracing is disabled when `OTEL_TRACES_EXPORTER` is unset, empty, or `none`.
`OTEL_SDK_DISABLED=true` also disables tracing regardless of the exporter.

Use an OTLP Collector in deployed environments:

```env
OTEL_TRACES_EXPORTER=otlp
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
```

The `console` exporter is available for local diagnostics:

```env
OTEL_TRACES_EXPORTER=console
```

Unsupported explicit values stop startup with an `OTEL-CONFIG-*` error. A
Collector outage after configuration does not stop the service or fail HTTP
requests. Export and shutdown failures are emitted as structured warnings
without logging configured endpoints or OTLP headers.

## Standard Environment Variables

BaSyx supports the standard trace settings provided by the OpenTelemetry Go
SDK and auto-exporter:

- `OTEL_TRACES_EXPORTER`: `otlp`, `console`, or `none`
- `OTEL_SDK_DISABLED`
- `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`
- `OTEL_EXPORTER_OTLP_PROTOCOL` and `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL`
- `OTEL_EXPORTER_OTLP_HEADERS` and `OTEL_EXPORTER_OTLP_TRACES_HEADERS`
- `OTEL_EXPORTER_OTLP_COMPRESSION` and
  `OTEL_EXPORTER_OTLP_TRACES_COMPRESSION`
- `OTEL_EXPORTER_OTLP_TIMEOUT` and `OTEL_EXPORTER_OTLP_TRACES_TIMEOUT`
- `OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES`
- `OTEL_TRACES_SAMPLER` and `OTEL_TRACES_SAMPLER_ARG`
- `OTEL_BSP_SCHEDULE_DELAY`, `OTEL_BSP_EXPORT_TIMEOUT`,
  `OTEL_BSP_MAX_QUEUE_SIZE`, and `OTEL_BSP_MAX_EXPORT_BATCH_SIZE`
- `OTEL_PROPAGATORS`

The default trace resource `service.name` is the lowercase command directory
name, such as `aasenvironmentservice`. `OTEL_SERVICE_NAME` overrides it. BaSyx
does not invent a `service.version`; operators can provide one through
`OTEL_RESOURCE_ATTRIBUTES`.

The default propagators are W3C Trace Context and Baggage. Supported
`OTEL_PROPAGATORS` values are `tracecontext`, `baggage`, and `none`.

See the [OpenTelemetry environment variable
specification](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/)
and [Go auto-exporter
documentation](https://pkg.go.dev/go.opentelemetry.io/contrib/exporters/autoexport)
for the detailed value formats.

## Sampling and Lifecycle

The default sampler is parent-based and always-on. For sustained or high-volume
traffic, configure an appropriate sampler instead of tracing every request. For
example:

```env
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.1
```

Completed spans are exported in batches. When a service stops, BaSyx flushes
and shuts down the tracer provider with a fresh bounded context after HTTP
shutdown has completed.

## HTTP Spans and Propagation

Every HTTP request creates one `SERVER` span when tracing is enabled. A valid
inbound W3C `traceparent` and `tracestate` is extracted; otherwise BaSyx creates
a new trace. Matched spans are named `<METHOD> <route>`, while unmatched routes
use `<METHOD>`.

Server span attributes include:

- `http.request.method`
- `url.path`
- `http.route` when Chi resolves a route
- `http.response.status_code`
- `http.response.body.size`
- `request.id`
- `correlation.id`

5xx responses and panics mark the span as an error. Panic spans contain only a
generic marker, not the panic value.

Delegated submodel-operation calls create `CLIENT` spans and inject the active
W3C trace context. This instrumentation wraps the existing guarded transport;
its host allowlisting, DNS/IP checks, proxy restrictions, timeout, pinned
dialing, redirect checks, and authorization behavior remain unchanged.
Requests without an active span do not receive fabricated trace headers.

This release does not propagate trace context to OIDC endpoints or arbitrary
third-party HTTP clients.

## Privacy

Trace spans never capture query strings, request or response bodies,
authorization values, arbitrary headers, tokens, user agents, or client IP
addresses. Delegated client spans contain only the method, scheme, hostname and
port, path, response status, and a generic error marker.

## Log Correlation

Every `slog` event emitted with a valid active span context receives these
top-level fields:

- `trace_id`
- `span_id`
- `trace_flags`

The IDs use lowercase hexadecimal values. `trace_flags` is `01` for sampled
contexts and `00` for unsampled contexts. Background logs remain unchanged.
The `HTTP request completed` access event is written before the server span
ends, so its identifiers match the span.

Logs still go to stderr. BaSyx does not export logs through OTLP or contain
direct clients for Jaeger, Loki, Tempo, or Grafana.

## Local Example

The [BaSyx observability
example](../../examples/BaSyxObservabilityExample/README.md) builds the BaSyx
images from the current checkout and connects:

```text
BaSyx --OTLP/HTTP--> Collector --OTLP/gRPC--> Jaeger
BaSyx --JSON stderr--> Alloy --Loki API--> Loki
Grafana --> Jaeger and Loki
```

Grafana is provisioned with links from log `trace_id` values to Jaeger and
from Jaeger traces back to Loki.

## Scope

This tracing foundation does not add metrics, OTLP log export, database spans,
PostgreSQL instrumentation, AAS-domain spans, runtime reconfiguration, or
custom exporter plugins.
