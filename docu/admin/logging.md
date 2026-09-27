# Logging

Every BaSyx command writes diagnostic logs to standard error. HTTP responses and
CLI result data, such as the history evidence verifier JSON report, remain on
standard output.

## Configuration

The default output is human-readable text at `info` level:

```yaml
logging:
  format: text
  level: info
```

The equivalent environment variables are:

```env
LOGGING_FORMAT=text
LOGGING_LEVEL=info
```

`LOGGING_FORMAT` accepts `text` or `json`. `LOGGING_LEVEL` accepts `debug`,
`info`, `warn`, or `error`. Environment variables override YAML values. Values
are case-insensitive, but empty or unsupported values stop the command during
configuration loading.

JSON records use the standard Go `log/slog` envelope and add the command
directory name as `service.name`:

```json
{"time":"2026-07-25T10:00:00Z","level":"INFO","msg":"configuration loaded","service.name":"aasregistryservice","configuration.source":"/app/config.yaml","logging.format":"json","logging.level":"info","server":{"host":"0.0.0.0","port":8080,"context_path":"","cache_enabled":false,"verification_mode":"permissive"},"features":{"abac_enabled":false,"swagger_enabled":true,"history_mode":"off","eventing_enabled":false,"eventing_feed_enabled":false}}
```

The same event in text mode is one readable record:

```text
time=2026-07-25T10:00:00.000Z level=INFO msg="configuration loaded" service.name=aasregistryservice configuration.source=/app/config.yaml logging.format=text logging.level=info
```

Configuration records contain only a curated subset. Passwords, DSNs, access
tokens, object-store credentials, private-key material, and request bodies are
not logged.

## HTTP Request Logging

Every HTTP service emits one `HTTP request completed` event after a request is
handled. Request logs include:

- `request.id`
- `correlation.id`
- `http.request.method`
- `url.path`
- `http.route` when the router resolved a route pattern
- `http.response.status_code`
- `http.response.body.size`
- `duration_ms`

`url.path` never contains the query string. Request and response bodies,
headers, tokens, user agents, and client IP addresses are not included.

Clients may supply `X-Request-ID` and `X-Correlation-ID`. The legacy
`Request-ID` and `Correlation-ID` names are accepted as input aliases. An
accepted value contains 1 to 128 ASCII letters, digits, or the characters
`._:/-`. BaSyx replaces missing or invalid request IDs with a value such as
`req-0123456789abcdef0123456789abcdef`. A missing or invalid correlation ID
defaults to the request ID. The canonical `X-Request-ID` and
`X-Correlation-ID` headers are added to the request context and response.
Cross-origin clients may send and read both canonical headers without adding
them to the CORS configuration.

These identifiers correlate records within one incoming request. They are not
authenticated identity data, and BaSyx does not propagate them as W3C trace
context. OpenTelemetry trace context is handled separately.

An access event in JSON format looks like:

```json
{"time":"2026-07-25T10:00:01Z","level":"INFO","msg":"HTTP request completed","service.name":"aasregistryservice","request.id":"request-42","correlation.id":"workflow-7","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","trace_flags":"01","http.request.method":"GET","url.path":"/shell-descriptors/example","http.route":"/shell-descriptors/{aasIdentifier}","http.response.status_code":200,"http.response.body.size":421,"duration_ms":3.725}
```

The same event in text format is:

```text
time=2026-07-25T10:00:01.000Z level=INFO msg="HTTP request completed" service.name=aasregistryservice request.id=request-42 correlation.id=workflow-7 trace_id=4bf92f3577b34da6a3ce929d0e0e4736 span_id=00f067aa0ba902b7 trace_flags=01 http.request.method=GET url.path=/shell-descriptors/example http.route=/shell-descriptors/{aasIdentifier} http.response.status_code=200 http.response.body.size=421 duration_ms=3.725
```

Normal requests are logged at `info`. `GET` health-probe requests are logged at
`debug`, so the default `info` level suppresses routine probe traffic. The
existing `logging.level` setting controls both request and application logs.

## Trace Correlation

When OpenTelemetry tracing is enabled, every contextual log emitted with a
valid span contains top-level `trace_id`, `span_id`, and `trace_flags` fields.
Valid unsampled contexts are included with `trace_flags=00`; background logs
without a span remain unchanged. The access event is emitted before its server
span ends, so the identifiers match.

See the [OpenTelemetry tracing guide](telemetry.md) for activation, propagation,
sampling, lifecycle, and privacy details.

## Following Logs

The supported runtime sink is standard error. Container runtimes and service
managers collect it without application-specific configuration:

```sh
docker logs -f <container>
kubectl logs -f <pod> [-c <container>]
journalctl -f -u <unit>
```

BaSyx does not provide a `/logs` endpoint, application log files, file
rotation, retention management, direct Loki integration, or direct OTLP log
export. Feed the process stream into the platform collector of your choice. For
example, Grafana Alloy, Fluent Bit, or an OpenTelemetry Collector can forward
container or journal logs to Loki without coupling the BaSyx process to that
backend. The [observability
example](../../examples/BaSyxObservabilityExample/README.md) demonstrates Alloy
and Loki collection while traces use OTLP.
