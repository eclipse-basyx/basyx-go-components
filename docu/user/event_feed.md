# Event Feed (experimental)

The Event Feed is an opt-in CloudEvents REST API. It is **disabled by default**.
Enabling it exposes identifiers, relationships, semantic IDs, and mutation timing
to callers that can reach `GET /events`. With ABAC on, each event is re-checked
as a read of the referenced AAS, Submodel, or asset lookup.

MQTT and Kafka sinks remain unimplemented. `eventing.sinks` and
`eventing.outboxEnabled` still fail fast.

## Configuration

Enable only the REST feed:

```yaml
eventing:
  enabled: false
  format: cloudevents
  feed:
    enabled: true
    maxAgeDays: 30
    hardDeleteGraceDays: 10
    maxPageSize: 100
    sourceBaseUrl: "http://localhost:5004"
    schemaBaseUrl: "https://admin-shell.io/events/schemas"
    cleanupIntervalHours: 24
```

| Setting | Environment variable | Meaning |
| --- | --- | --- |
| `eventing.feed.enabled` | `BASYX_EVENTING_FEED_ENABLED` | Opt in to `/events` and well-known capabilities. Default `false`. |
| `eventing.enabled` | `BASYX_EVENTING_ENABLED` | Reserved for future MQTT/Kafka. Does **not** enable the REST feed. |
| `eventing.feed.maxAgeDays` | `BASYX_EVENTING_FEED_MAX_AGE_DAYS` | Consumer-visible retention window. |
| `eventing.feed.hardDeleteGraceDays` | `BASYX_EVENTING_FEED_HARD_DELETE_GRACE_DAYS` | Extra delay before physical delete. |
| `eventing.feed.maxPageSize` | `BASYX_EVENTING_FEED_MAX_PAGE_SIZE` | Default and maximum page size when `limit` is omitted. |
| `eventing.feed.sourceBaseUrl` | `BASYX_EVENTING_FEED_SOURCE_BASE_URL` | CloudEvents `source` prefix. |
| `eventing.feed.schemaBaseUrl` | `BASYX_EVENTING_FEED_SCHEMA_BASE_URL` | CloudEvents `dataschema` prefix. |
| `eventing.feed.cleanupIntervalHours` | `BASYX_EVENTING_FEED_CLEANUP_INTERVAL_HOURS` | Retention worker interval (also runs once at startup). |

Requires database schema `v1.2.0` (`feed_events`). Sample `config.yaml` files
ship with `eventing.feed.enabled: false`.

## Endpoints

Relative to the service API base path (for example `/api/v3`):

- `GET /events` — page of CloudEvents (`presentation=REGULAR` or `COMPACT`)
- `GET /.well-known/event-feed.json` — capabilities (API/event versions, schemas,
  filters, presentation modes, `maxPageSize`, inherited auth)

When the feed is disabled, both routes and the matching OpenAPI operations are
absent.

## Security

- Authentication and authorization are **inherited** from the hosting service.
  There are no `publicAccess` / `bearerAuth` feed knobs.
- With ABAC disabled, an anonymous caller that can reach the service can read
  the feed once it is enabled.
- With ABAC enabled, `GET /events` and `GET /.well-known/event-feed.json` map
  to `READ` (and to `$aas` / `$sm` IDENTIFIABLE collections). Include those
  routes in ROUTE-based policies. Each returned event is re-authorized as a
  read of `/shells/{id}`, `/submodels/{id}`, or `/lookup/shells`. Events the
  caller cannot read are omitted. Field-based formulas that need a hydrated
  AAS/Submodel body fail closed (the event is hidden).
- Do not enable the feed on a public endpoint unless that exposure is intended.

## Delivery, ordering, retention

- Feed rows are written in the **same PostgreSQL transaction** as the model
  mutation (`history.AppendVersionTx` / `AppendMutatedVersionTx`). A rolled-back
  write produces no event. A committed write always has the matching feed rows.
- Ordering and cursors use the table `seq` (`BIGSERIAL`), not wall-clock time.
- Equal PUT/update snapshots do not emit `updated` events.
- Coverage includes create/update/delete of AAS and Submodels, including PATCH,
  SubmodelElement, file, thumbnail, AssetInformation, and AAS Environment
  `/upload` paths that go through the same persistence transactions. PCN events
  (`io.admin-shell.pcn.v1`) are emitted when a PCN submodel (semantic id
  `0173-1#01-AHE582#003`) gains a new record, including via SubmodelElement
  writes.
- Retention: events older than `maxAgeDays` leave the consumer window; physical
  delete waits `hardDeleteGraceDays`. Cleanup runs at startup and on the
  configured interval, in bounded batches, with a PostgreSQL advisory lock.

## Schema and compatibility

- CloudEvents spec version `1.0`, feed API version `1.0`.
- Event types are versioned (`*.v1`). `dataschema` points at
  `schemaBaseUrl` JSON Schema documents for `REGULAR` and `COMPACT`.
- `presentation=FULL` is accepted as a deprecated alias of `REGULAR`.
- Future MQTT/Kafka transports will use `eventing.enabled` / sinks, not
  `eventing.feed.enabled`.
