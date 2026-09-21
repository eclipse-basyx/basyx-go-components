# MQTT Eventing (experimental)

Publish AAS, asset, Submodel, and PCN changes as CloudEvents over MQTT 5.
AAS Repository, Submodel Repository, and AAS Environment support this opt-in
feature. Try the [MQTT example](../../examples/BaSyxMQTTExample/README.md).

## Enable MQTT

Run the configuration service to apply database schema v1.2.1, then configure:

```yaml
general:
  externalUrl: https://example.com/api/v3
eventing:
  enabled: true
  format: cloudevents
  sinks: [mqtt]
  outboxEnabled: true
  mqtt:
    broker: mqtt://localhost:1883
    clientId: basyx-instance-1
```

All three activation settings are required. Set `general.externalUrl` to the
public API base URL so event sources and schema links resolve for consumers.

## Connection settings

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
By default, the public API URL supplies the source and hosted schema URLs.

Replicas sharing a database use the same `sinkId` and destination settings, with
unique MQTT client IDs. Drain the queue before changing the broker, sink ID, or
topic configuration. Reenabling the same sink resumes its pending queue.

## Messages and topics

Each message contains one REGULAR CloudEvent as structured JSON. The MQTT 5
Content Type is `application/cloudevents+json`; the envelope declares
`specversion: "1.0"` and `datacontenttype: "application/json"`, following the
[CloudEvents MQTT binding](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/mqtt-protocol-binding.md).
Change payloads contain model identifiers and references; PCN payloads contain
the added record's value-only representation. `dataschema` links to the payload
schema served by the API.

| Event family | Topic (default prefix) |
| --- | --- |
| AAS | `basyx/aasrepository/aas/{created,updated,deleted}` |
| Asset | `basyx/aasrepository/asset/{created,updated,deleted}` |
| Submodel | `basyx/submodelrepository/submodel/{created,updated,deleted}` |
| PCN | `basyx/submodelrepository/pcn/notification` |

AAS Environment uses the same logical repository topic names. Nested elements,
values, file attachments, thumbnails, and uploads use the existing mutation
triggers. Reads, rolled-back changes, and acknowledged no-op PUTs produce no
change event. Atomic mutations enqueue all their events in the same transaction.

## Delivery and operations

Events commit atomically with model changes. During broker outages, services
continue accepting mutations and retain pending events in PostgreSQL. Failed
publishes retry indefinitely with exponential backoff and jitter, starting at
one second and capped at one minute. Publish attempts time out after ten seconds.

Delivery is ordered per mutated AAS or Submodel, including derived asset and PCN
events. A failed event blocks later events for that entity; other entities can
progress. Successful deliveries are removed from the queue. Pending entries do
not expire, so allow database capacity for the expected outage duration.

QoS 1 and 2 provide at-least-once delivery. Deduplicate by CloudEvents ID: a crash
after broker acknowledgment can cause a retry with the same ID. QoS 0 has no
broker acknowledgment and weakens this guarantee. Retained messages keep only
the latest message per topic.

Configure broker authentication and subscriber ACLs for the data in each topic.
Use `tls://` for TLS and mount credential/certificate files through the settings
above. HTTP access rules do not filter MQTT subscribers.

Existing OpenTelemetry configuration exports these metrics, labeled by sink:

- `basyx.eventing.delivered`: acknowledged deliveries.
- `basyx.eventing.delivery.failures`: failed attempts or worker database errors.
- `basyx.eventing.pending`: pending deliveries.
- `basyx.eventing.retry.pending`: pending deliveries with previous failures.
- `basyx.eventing.pending.oldest.age`: oldest pending event age in seconds.
- `basyx.eventing.broker.connected`: connection state, 0 or 1.

Queue gauges refresh every 15 seconds. Monitor queue growth and delivery age;
coded logs identify connection and publication failures.
