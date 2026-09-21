# Event Feed (experimental)

Use the Event Feed to discover AAS and Submodel changes through a CloudEvents
REST API. It is opt-in and **disabled by default** in all standard service
configurations. For a guided local trial, start with the
[Event Feed example](../../examples/BaSyxEventFeedExample/README.md).

## Enable the feed

Set `BASYX_EVENTING_FEED_ENABLED=true`, or enable it in your service configuration:

```yaml
general:
  externalUrl: "https://example.com/api/v3"
eventing:
  feed:
    enabled: true
```

Set `general.externalUrl` to your public API base URL, including any context
path, so event sources and schema links work for your clients behind a proxy.
The deployment requires database schema `v1.2.0` or later; run the configuration
service to apply migrations before starting the hosting service.

## Read events

These endpoints are relative to your service's API base path:

| Request | Purpose |
| --- | --- |
| `GET /events` | Read a page of events. |
| `GET /.well-known/event-feed.json` | Discover supported event types, filters, schemas, retention, and page limits. |
| `GET /.well-known/event-feed/schemas/{schema}` | Retrieve the versioned JSON Schema linked by an event's `dataschema`. |

The feed and discovery routes are absent when the feed is disabled.

The feed spans all entities you are authorized to read. Each event's `subject`
identifies the affected entity, `type` identifies the change, and `time` gives
the mutation timestamp. The response contains retained history, so startup
imports and earlier changes can appear alongside the operation you just performed.
Reading events does not consume them.

AAS and Submodel creation, update, and deletion produce events, including changes
through SubmodelElement, value, file, and AAS Environment upload endpoints.
Writing an identical PUT snapshot does not produce an update event. Adding a
new PCN record to a Submodel with semantic ID `0173-1#01-AHE582#003` produces both
a PCN notification and a Submodel update.

### Filter and choose a presentation

For example, read compact events for one Submodel (replace the URL and identifier):

```sh
curl --get 'https://example.com/api/v3/events' \
  --data-urlencode 'filter=rsql:event.subject==urn:example:submodel:1' \
  --data-urlencode 'presentation=COMPACT' \
  --data-urlencode 'limit=10'
```

`REGULAR` is the default presentation and includes the full event payload.
`COMPACT` contains its identification subset; for PCN events it omits `record`.
`FULL` is a deprecated alias of `REGULAR`.

RSQL filters support `==`, `!=`, `=in=`, and `=out=` on `event.type`,
`event.subject`, `event.source`, and `event.dataschema`. AND (`;`), OR (`,`), and
parenthesized groups are supported; AND has higher precedence. Quote reserved
characters inside identifiers. Malformed syntax and unsupported fields are
rejected. Expressions are limited to 16 KiB and 32 levels of nesting.

### Page through history and poll for changes

Without a filter or checkpoint, requests begin at the earliest retained events
in publication order. `limit=1` starts with the first published event available
to the caller. Each page is sorted chronologically; `updated` is the newest
mutation timestamp in that page. A transaction that commits late can introduce
an older timestamp on a later page, so timestamps alone are not checkpoints.

Follow the response's opaque `cursor` until it is absent. The cursor preserves
`since`, `filter`, and `presentation`; omit them on subsequent requests or supply
the same values. Conflicting values are rejected. Authorization is evaluated on
every request. Even an empty page can contain a cursor when more records remain
to be scanned.

Use `lastEventId` to resume polling after a processed event, and follow any
returned cursors to read the subsequent pages. Consumers must deduplicate by
event ID: resuming can replay records because page presentation and publication
order can differ. A transaction that commits after the checkpoint is issued
remains discoverable. An unpublished event ID cannot be used as a checkpoint.

Use `since` as an inclusive initial time filter. It cannot be combined with
`lastEventId` and does not guarantee complete ongoing polling. Events eventually
expire from the feed; applications that need to recover after a retention gap
should read current state from the AAS APIs.

## Access control

Authentication and authorization are inherited from the hosting service. There
are no separate feed authentication settings. With ABAC disabled, the enabled
feed has the same anonymous access as the hosting service.

With ABAC enabled, feed and schema routes require `READ` and belong to the
`$aas`/`$sm` IDENTIFIABLE collections. ROUTE-based policies must cover the routes
they intend to expose. Every returned event is checked against the caller's
current access rules, including the configured context path:

- AAS events require unrestricted read access to the referenced AAS.
- Submodel and PCN events require unrestricted read access to the Submodel.
  If the payload includes asset IDs, access to every contributing AAS is also
  required. Sharing a Submodel does not grant access to a private AAS's asset IDs.
- Asset events require read access to their owning AAS. Access to
  `/lookup/shells` alone does not authorize their contents.

The feed hides an entire event if permission for all its contents cannot be
established, including when row or field restrictions cannot be evaluated
safely. Removing an AAS/Submodel relationship later does not bypass these
checks. Older experimental events without sufficient ownership information
are also hidden when ABAC is enabled.

## Configuration reference

Defaults work for an initial deployment. Adjust retention to the time consumers
may be offline, and page size to the volume they can process per request.

| Setting | Environment variable | Default and purpose |
| --- | --- | --- |
| `eventing.feed.enabled` | `BASYX_EVENTING_FEED_ENABLED` | `false`; enable the REST feed. |
| `eventing.feed.maxAgeDays` | `BASYX_EVENTING_FEED_MAX_AGE_DAYS` | `30`; visible retention window, measured from mutation time. |
| `eventing.feed.hardDeleteGraceDays` | `BASYX_EVENTING_FEED_HARD_DELETE_GRACE_DAYS` | `10`; additional days before physical deletion. `0` disables this delay. |
| `eventing.feed.maxPageSize` | `BASYX_EVENTING_FEED_MAX_PAGE_SIZE` | `100`; default and maximum page size. |
| `eventing.feed.sourceBaseUrl` | `BASYX_EVENTING_FEED_SOURCE_BASE_URL` | Public API base URL. Falls back to `general.externalUrl`, then the local server URL and context path. |
| `eventing.feed.schemaBaseUrl` | `BASYX_EVENTING_FEED_SCHEMA_BASE_URL` | Optional schema mirror. Defaults to the source base URL followed by `/.well-known/event-feed/schemas`. |
| `eventing.feed.cleanupIntervalHours` | `BASYX_EVENTING_FEED_CLEANUP_INTERVAL_HOURS` | `24`; physical cleanup interval. Cleanup also runs at startup. |
| `eventing.feed.publishIntervalMillis` | `BASYX_EVENTING_FEED_PUBLISH_INTERVAL_MILLIS` | `250`; interval before checking for committed events to publish. Publication also runs at startup. |

Expired events are no longer readable during the hard-delete grace period.
Delivery latency depends on the publish interval, database load, and backlog.
Event source and schema URLs come from configuration, not request host headers.

## Schema compatibility

CloudEvents use spec version `1.0`; the feed API version is `1.0`. Follow each
event's `dataschema` to validate its payload. The hosted documents are BaSyx's
versioned implementation schemas, not unmodified copies of the external eventing
draft. Their identifiers use the `urn:eclipse-basyx:event-feed:schemas:` namespace.
Custom mirrors must serve the same versioned documents.

Submodel events support an absent `semanticId`, matching the AAS metamodel.
AAS and asset references are `ModelReference` objects; optional semantic
references are `ExternalReference` objects. PCN events use
`pcnNotificationEvent.v1.schema.json` for both presentations.

For mutation capture, database ordering, and extension requirements, see the
[developer runtime guide](../developer/aas_v3_2_runtime.md#event-feed-runtime).
