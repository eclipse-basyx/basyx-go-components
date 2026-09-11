# Event Feed (experimental)

The Event Feed is an opt-in CloudEvents REST API. It is **disabled by default**.
All standard service configurations leave it disabled. Enabling the feed adds
its routes, schema documents, mutation capture, and background workers.

The [Event Feed example](../../examples/BaSyxEventFeedExample/README.md) provides
a local playground, sample data, and an automated smoke test.

MQTT and Kafka sinks remain unimplemented. `eventing.sinks` and
`eventing.outboxEnabled` still fail fast. `eventing.enabled` is reserved for
future transports and does not enable the REST feed.

## Configuration

```yaml
general:
  externalUrl: "https://example.com/api/v3"
eventing:
  enabled: false
  format: cloudevents
  feed:
    enabled: true
    maxAgeDays: 30
    hardDeleteGraceDays: 10
    maxPageSize: 100
    sourceBaseUrl: ""
    schemaBaseUrl: ""
    cleanupIntervalHours: 24
    publishIntervalMillis: 250
```

| Setting | Environment variable | Meaning |
| --- | --- | --- |
| `eventing.feed.enabled` | `BASYX_EVENTING_FEED_ENABLED` | Enable the REST feed. Default `false`. |
| `eventing.enabled` | `BASYX_EVENTING_ENABLED` | Reserved for future MQTT/Kafka; independent of the REST feed. |
| `eventing.feed.maxAgeDays` | `BASYX_EVENTING_FEED_MAX_AGE_DAYS` | Consumer-visible retention window, measured from mutation time. |
| `eventing.feed.hardDeleteGraceDays` | `BASYX_EVENTING_FEED_HARD_DELETE_GRACE_DAYS` | Additional delay before physical deletion; `0` disables this delay. |
| `eventing.feed.maxPageSize` | `BASYX_EVENTING_FEED_MAX_PAGE_SIZE` | Default and maximum page size. |
| `eventing.feed.sourceBaseUrl` | `BASYX_EVENTING_FEED_SOURCE_BASE_URL` | Public API base URL, including any context path. Defaults to `general.externalUrl`, then the local server URL and context path. |
| `eventing.feed.schemaBaseUrl` | `BASYX_EVENTING_FEED_SCHEMA_BASE_URL` | Optional mirror of the packaged schema documents. Defaults to `sourceBaseUrl` followed by `/.well-known/event-feed/schemas`. |
| `eventing.feed.cleanupIntervalHours` | `BASYX_EVENTING_FEED_CLEANUP_INTERVAL_HOURS` | Cleanup interval; cleanup also runs at startup. |
| `eventing.feed.publishIntervalMillis` | `BASYX_EVENTING_FEED_PUBLISH_INTERVAL_MILLIS` | Publish-assignment interval; assignment also runs at startup. Default `250`. |

Requires database schema `v1.2.0`. The configuration service applies the single
`v1.2.0` patch for the feed table, including capture-time authorization ownership.
Configure the public API base URL when operating behind a proxy; event sources
and schema URLs are produced from configuration, not caller-supplied host headers.

## Endpoints

Relative to the service API base path (for example `/api/v3`):

- `GET /events` returns a page of CloudEvents.
- `GET /.well-known/event-feed.json` describes event types, schemas, filters,
  presentation modes, retention, and page limits.
- `GET /.well-known/event-feed/schemas/{schema}` serves the exact versioned
  JSON Schema documents used by the feed.

All three routes and their OpenAPI operations are absent when the feed is disabled.

## Security

Authentication and authorization are inherited from the hosting service. There
are no separate `publicAccess` or `bearerAuth` feed settings. With ABAC disabled,
the enabled feed has the same anonymous access as the hosting service.

With ABAC enabled, feed and schema routes require `READ` and belong to the
`$aas`/`$sm` IDENTIFIABLE collections. ROUTE-based policies must cover the routes
they intend to expose. Every returned event is checked against the caller's
current access rules, including the configured context path:

- AAS events require unrestricted read access to the referenced AAS.
- Submodel and PCN events require unrestricted read access to the Submodel.
  If their payload includes asset IDs, access to every contributing AAS is also
  required. A shared Submodel does not grant access to a private AAS's asset IDs.
- Asset events require read access to their owning AAS. Access to
  `/lookup/shells` alone does not authorize AAS contents in an asset event.
- Remaining row or field restrictions that cannot be evaluated safely cause
  the event to be hidden. Related ownership is captured inside the mutation
  transaction, so removing a relationship later does not bypass authorization.
- Legacy experimental asset, Submodel, and PCN rows without trustworthy
  ownership metadata are hidden when ABAC is enabled. Ownership is not guessed
  from current state.

The feed omits an entire event when it cannot establish permission for all its
contents. It does not expose a partially authorized payload.

## Delivery, pagination, and retention

Feed rows are written in the same PostgreSQL transaction as the model mutation.
Rollback or cancellation before commit produces no committed feed row. Equal
PUT snapshots retain their history acknowledgement without emitting an update.

The internal `seq` is assigned before commit and is not a consumer checkpoint.
A background worker assigns `publish_seq` only to committed, visible rows.
Assignment is serialized and batched using a transaction advisory lock on the
same connection as the worker queries. Retention uses its own transaction lock
and commits each bounded deletion batch separately. Both support a
one-connection database pool.

Pagination selects records by `publish_seq`. Each response sorts its selected
records chronologically by mutation time; `updated` is the newest timestamp in
that response. A late-committing transaction can therefore introduce an older
mutation timestamp on a later page. Publication order is a durable scan order,
not a claim to reconstruct PostgreSQL transaction commit timestamps.

Follow the response's opaque `cursor` until it is absent. A cursor preserves
`since`, `filter`, and `presentation`, so these may be omitted on subsequent
requests; conflicting values are rejected. Authorization is evaluated again
for every request. An empty authorized page can still contain a cursor when
more rows remain to be scanned.

Use `lastEventId` to resume polling after a processed event. Because chronological
presentation can differ from scan order, this can replay already processed
records; consumers must deduplicate by event ID. It does not skip a transaction
that commits after a checkpoint was issued. An unpublished event ID cannot be
used as a checkpoint. `since` is an inclusive initial time filter, not a
completeness guarantee for polling.

RSQL filters support `==`, `!=`, `=in=`, and `=out=` on `event.type`,
`event.subject`, `event.source`, and `event.dataschema`. AND (`;`), OR (`,`), and
parenthesized groups are supported; AND has higher precedence. Quote reserved
characters inside identifiers. Malformed syntax and unsupported fields are
rejected. Expressions are limited to 16 KiB and 32 levels of nesting.

Events older than `maxAgeDays` leave the consumer window. Physical deletion
waits an additional `hardDeleteGraceDays`. The publish interval controls worker
scheduling; actual delivery latency also depends on database load and backlog.

## Schema and mutation coverage

CloudEvents use spec version `1.0`; the feed API version is `1.0`. Event types
and schema names are versioned. `presentation=REGULAR` returns the full event
payload; `COMPACT` returns its identification subset. `FULL` remains a deprecated
alias of `REGULAR`.

The API serves the BaSyx schema documents embedded from
`internal/common/eventfeed/schemas`. Their identifiers use the
`urn:eclipse-basyx:event-feed:schemas:` namespace. These are implementation
schemas, not unmodified copies of the external eventing draft. In particular,
`semanticId` is optional in both Submodel presentations, matching the AAS
metamodel; Submodels without one still generate valid events. Custom schema
mirrors must serve these same versioned documents. Tests validate generated
payloads against the exact documents returned by the HTTP schema endpoint.

AAS and asset references are `ModelReference` objects; optional semantic
references are `ExternalReference` objects. PCN events use the single
`pcnNotificationEvent.v1.schema.json` document for both presentations, with
`record` omitted in COMPACT mode.

Mutation capture covers AAS and Submodel creation, update, and deletion through
the shared persistence path, including SubmodelElement/value/file writes and
AAS Environment uploads. PCN notifications are generated for new records in
Submodels with semantic ID `0173-1#01-AHE582#003`. Snapshot capture is serialized
with mutations even when history evidence is disabled, preventing concurrent
record additions from being reported twice.
