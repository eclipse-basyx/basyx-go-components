# MQTT eventing (experimental)

Publish the existing AAS, asset, Submodel, and PCN CloudEvents over MQTT 5.
AAS Repository, Submodel Repository, and AAS Environment support this feature.
It is disabled by default and does not require history or WORM evidence.
See the [runnable MQTT example](../../examples/BaSyxMQTTExample/README.md).

## Enable MQTT and the HTTP feed together

```yaml
general:
  externalUrl: https://example.com/api/v3
eventing:
  enabled: true
  format: cloudevents
  sinks: [mqtt]
  outboxEnabled: true
  topicPrefix: basyx
  feed:
    enabled: true
  mqtt:
    broker: tls://broker.example.com:8883
    clientId: basyx-replica-1
    sinkId: mqtt
    qos: 1
    retained: false
    usernameFile: /run/secrets/mqtt-username
    passwordFile: /run/secrets/mqtt-password
    caFile: /run/secrets/broker-ca.pem
    certificateFile: /run/secrets/client.pem
    keyFile: /run/secrets/client-key.pem
```

Set `eventing.feed.enabled: false` for MQTT-only operation. Existing feed-only
configurations continue to work without `eventing.enabled`. MQTT requires
`enabled`, `sinks: [mqtt]`, and `outboxEnabled: true`. Unknown sinks are rejected.

Apply schema **v1.2.1** using the configuration service before starting the new
service version. The additive migration creates `event_outbox`; it does not
rewrite model data, change existing feed cursors, or backfill historical events.

## Configuration

| Setting under `eventing.mqtt` | Environment variable | Default / behavior |
| --- | --- | --- |
| `broker` | `BASYX_EVENTING_MQTT_BROKER` | Required; `mqtt://host:port` or `tls://host:port`. No embedded credentials. |
| `clientId` | `BASYX_EVENTING_MQTT_CLIENT_ID` | Required; unique for each process connecting to the broker. |
| `sinkId` | `BASYX_EVENTING_MQTT_SINK_ID` | `mqtt`; stable queue destination identifier. |
| `qos` | `BASYX_EVENTING_MQTT_QOS` | `1`; supports 0, 1, and 2. |
| `retained` | `BASYX_EVENTING_MQTT_RETAINED` | `false`. |
| `username`, `password` | `BASYX_EVENTING_MQTT_USERNAME`, `BASYX_EVENTING_MQTT_PASSWORD` | Optional credentials, omitted from effective configuration output. |
| `usernameFile`, `passwordFile` | `BASYX_EVENTING_MQTT_USERNAME_FILE`, `BASYX_EVENTING_MQTT_PASSWORD_FILE` | Alternative mounted credential files; trailing line endings are removed. |
| `caFile` | `BASYX_EVENTING_MQTT_CA_FILE` | Optional private CA bundle; otherwise use system trust. |
| `certificateFile`, `keyFile` | `BASYX_EVENTING_MQTT_CERTIFICATE_FILE`, `BASYX_EVENTING_MQTT_KEY_FILE` | Optional client certificate/key pair for mutual TLS. |

Shared switches use `BASYX_EVENTING_ENABLED`, `BASYX_EVENTING_SINKS` (comma-separated),
`BASYX_EVENTING_OUTBOX_ENABLED`, and `BASYX_EVENTING_TOPIC_PREFIX` (default `basyx`).
TLS verifies the server name and certificate chain and requires TLS 1.2 or later.
Do not configure a credential value and its corresponding file simultaneously.

`eventing.sourceBaseUrl` and `eventing.schemaBaseUrl` override the shared producer
and schema URLs (`BASYX_EVENTING_SOURCE_BASE_URL`, `BASYX_EVENTING_SCHEMA_BASE_URL`).
The existing `eventing.feed.sourceBaseUrl` / `schemaBaseUrl` settings remain aliases;
conflicting explicit URLs are rejected. Otherwise the public API URL supplies the
source and the hosted schema URL. Schema routes remain available with MQTT-only
operation; `/events` and feed discovery remain disabled.

All replicas sharing a `sinkId` in one database must have the same broker,
credentials, QoS, retained behavior, and topic configuration. Distinct destinations
sharing a database need distinct sink IDs. Each process supports one MQTT
destination. Drain pending deliveries before changing a destination or topic
configuration: stored topics are immutable, and changing a sink ID leaves its old
queue awaiting a worker with that ID. Disabling MQTT stops enqueueing and delivery;
reenabling the same sink resumes its pending queue.

## CloudEvents and topics

Each MQTT PUBLISH contains one REGULAR CloudEvent as structured JSON, with
`specversion: "1.0"`, `datacontenttype: "application/json"`, and MQTT Content Type
`application/cloudevents+json`. The implementation follows the
[CloudEvents MQTT binding](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/mqtt-protocol-binding.md).
MQTT 3.1.1 and binary content mode are not supported.

The ID, timestamp, type, subject, source, dataschema, and data are identical to the
corresponding REGULAR HTTP feed record. Existing versioned payload schemas apply:
change events contain identifiers/references; they do not contain complete model
snapshots. Delete events preserve the existing identifier/reference representation.
PCN notifications contain their existing value-only record.

| Event family | Topic (default prefix) |
| --- | --- |
| AAS | `basyx/aasrepository/aas/created`, `/updated`, `/deleted` |
| Asset | `basyx/aasrepository/asset/created`, `/updated`, `/deleted` |
| Submodel | `basyx/submodelrepository/submodel/created`, `/updated`, `/deleted` |
| PCN | `basyx/submodelrepository/pcn/notification` |

AAS Environment uses the same logical repository topic names. Nested elements,
values, file attachments, thumbnails, and uploads use the existing mutation
triggers. Reads, rolled-back changes, and acknowledged no-op PUTs produce no
change event. Atomic mutations enqueue all their events in the same transaction.

MQTT delivers the configured service's events to its broker. HTTP caller-specific
ABAC filtering does not apply to MQTT subscribers; configure broker ACLs and
network access for the data those topics contain. The HTTP feed retains its
existing authorization checks.

## Delivery and operations

Model changes and outbox entries commit atomically. An enqueue error rolls back
the change. Broker failures after commit do not alter the API response. Services
start while the broker is unavailable, retaining new events in PostgreSQL.

Four workers per process publish outside model transactions. A worker holds only
a delivery-row transaction during its bounded publish attempt (maximum ten
seconds), then acknowledges or schedules a retry. PostgreSQL releases that lock
on process/connection loss. Earlier pending entries prevent later delivery for
the same mutated AAS/Submodel and sink, including derived asset/PCN events.
Unrelated entities can progress. There is no global ordering guarantee.

At QoS 1 or 2, delivery is at least once: a crash between broker acknowledgment and
database acknowledgment can repeat an event. Deduplicate by CloudEvents ID before
applying ordered events. QoS 2 does not make the database-to-broker operation
exactly once. QoS 0 explicitly weakens the guarantee because it has no broker
acknowledgment. With retained messages enabled, subscribers receive the last
message per topic, not a complete change log.

Failed deliveries retry indefinitely with exponential backoff and jitter,
starting around one second and capped at one minute. A permanently rejected event
blocks later events for that mutation key until the underlying problem is fixed.
Pending entries are never expired or dead-lettered automatically. Successfully
acknowledged entries are deleted immediately. HTTP feed cleanup cannot remove
pending MQTT events. Allow database capacity for the expected outage duration.

Existing OpenTelemetry configuration exports these metrics, labeled by sink:

- `basyx.eventing.delivered`: successful delivery acknowledgments.
- `basyx.eventing.delivery.failures`: failed attempts or worker database errors.
- `basyx.eventing.pending`: queued delivery count.
- `basyx.eventing.retry.pending`: queued deliveries that have failed before.
- `basyx.eventing.pending.oldest.age`: age in seconds of the oldest queued event.
- `basyx.eventing.broker.connected`: broker connection state, 0 or 1.

Queue gauges refresh every 15 seconds. Alert on a disconnected broker, growing
queue, or oldest-event age beyond your delivery objective. Inspect coded logs for
connection and publish failures; fix credentials, broker ACLs, certificate trust,
packet limits, or database availability and allow automatic retries to resume.
Keep the same sink ID and event IDs when recovering a queue. Never delete pending
rows as part of routine cleanup.

The shared builder, transactional fan-out, and per-sink outbox support future
adapters such as Kafka without new persistence triggers. Kafka support itself is
outside this release.
