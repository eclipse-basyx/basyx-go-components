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
{"time":"2026-07-25T10:00:00Z","level":"INFO","msg":"configuration loaded","service.name":"aasregistryservice","configuration.source":"/app/config.yaml","logging.format":"json","logging.level":"info","server":{"host":"0.0.0.0","port":8080,"context_path":"","cache_enabled":false,"verification_mode":"permissive"},"features":{"abac_enabled":false,"swagger_enabled":true,"history_mode":"off","eventing_enabled":false}}
```

The same event in text mode is one readable record:

```text
time=2026-07-25T10:00:00.000Z level=INFO msg="configuration loaded" service.name=aasregistryservice configuration.source=/app/config.yaml logging.format=text logging.level=info
```

Configuration records contain only a curated subset. Passwords, DSNs, access
tokens, object-store credentials, private-key material, and request bodies are
not logged.

## Following Logs

The supported runtime sink is standard error. Container runtimes and service
managers collect it without application-specific configuration:

```sh
docker logs -f <container>
kubectl logs -f <pod> [-c <container>]
journalctl -f -u <unit>
```

BaSyx does not provide a `/logs` endpoint, application log files, file
rotation, retention management, or direct Loki integration. Feed the process
stream into the platform collector of your choice. For example, Grafana Alloy,
Fluent Bit, or an OpenTelemetry Collector can forward container or journal logs
to Loki without coupling the BaSyx process to that backend.

OpenTelemetry trace correlation and OTLP export are intentionally outside this
logging foundation.
