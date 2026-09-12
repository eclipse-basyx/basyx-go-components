# Kafka Eventing (experimental)

Publish AAS, asset, Submodel, and PCN changes as CloudEvents to Kafka from
AAS Repository, Submodel Repository, or AAS Environment. To try it locally,
use the [Kafka example](../../examples/BaSyxKafkaExample/README.md).

## Enable Kafka

Apply database schema v1.2.1 or later with the configuration service and create
the Kafka topic before starting the application. Configure the public API URL and broker:

```yaml
general:
  externalUrl: https://example.com/api/v3
eventing:
  enabled: true
  format: cloudevents
  sinks: [kafka]
  outboxEnabled: true
  kafka:
    brokers: [localhost:9092]
    topic: basyx.events
```

All three activation settings are required: `enabled`, `sinks`, and `outboxEnabled`.
`general.externalUrl` supplies the event source and hosted schema URLs. Override
these with `eventing.sourceBaseUrl` and `eventing.schemaBaseUrl` if needed.

## Connection settings

Settings below are under `eventing.kafka`. Environment variables use the prefix
`BASYX_EVENTING_KAFKA_` followed by the suffix shown.

| Setting | Environment suffix | Default / behavior |
| --- | --- | --- |
| `brokers` | `BROKERS` | Required bootstrap `host:port` list; comma-separated in the environment. No schemes or embedded credentials. |
| `topic` | `TOPIC` | `basyx.events`; one topic for all event families. |
| `sinkId` | `SINK_ID` | `kafka`; stable delivery queue identifier. Replicas sharing a database use the same ID and destination. |
| `clientId` | `CLIENT_ID` | `basyx`; client name reported to Kafka. |
| `producerBatchMaxBytes` | `PRODUCER_BATCH_MAX_BYTES` | `0` uses the client default of 1,000,012 bytes before compression. Set 512–1,073,741,824 for larger notifications; align broker/topic message limits and `socket.request.max.bytes` with the chosen size. |
| `tlsEnabled` | `TLS_ENABLED` | `false`; enables certificate verification with TLS 1.2 or later. |
| `caFile` | `CA_FILE` | Optional PEM CA bundle; otherwise system trust. |
| `certificateFile`, `keyFile` | `CERTIFICATE_FILE`, `KEY_FILE` | PEM client certificate/key pair for mutual TLS. Requires TLS. |
| `saslMechanism` | `SASL_MECHANISM` | Empty disables SASL; supports `PLAIN`, `SCRAM-SHA-256`, `SCRAM-SHA-512`. |
| `username`, `password` | `USERNAME`, `PASSWORD` | Required with SASL; omitted from effective configuration output. |
| `usernameFile`, `passwordFile` | `USERNAME_FILE`, `PASSWORD_FILE` | Mounted credential alternatives; trailing line endings are removed. |

Supply either a value or a file for each credential. Supply the TLS client
certificate and key together. Use TLS when transmitting SASL credentials.
Configure broker ACLs for producers and consumers; HTTP access rules do not
restrict Kafka subscribers.

Activation settings also accept `BASYX_EVENTING_ENABLED`, `BASYX_EVENTING_SINKS`,
and `BASYX_EVENTING_OUTBOX_ENABLED`.

## Consume events

Each record uses the structured JSON format defined by the
[CloudEvents Kafka binding v1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/kafka-protocol-binding.md):

- Value: a CloudEvent envelope with `specversion: "1.0"` and
  `datacontenttype: "application/json"`.
- Header: `content-type: application/cloudevents+json`.
- Key: `aas_history:<AAS ID>` or `submodel_history:<Submodel ID>`.
  Asset events share their AAS key; PCN events share their Submodel key.

Filter by `type` or `subject` to select events from the topic. Change payloads
identify affected models; PCN payloads contain the new record's value-only
representation. `dataschema` links to the payload schema served by the API.

Delivery is at least once: deduplicate by CloudEvents ID. Events for an entity
share a partition and are delivered in order. Keep the partition count stable
when relying on this ordering. There is no ordering across partitions.

## Operate the service

During broker outages, model changes remain available and pending events stay in
PostgreSQL until delivery is acknowledged. Failed deliveries retry indefinitely;
pending events do not expire. Allow database capacity for the expected outage.
A failed delivery delays later events for the same entity.

Kafka must acknowledge writes from all in-sync replicas. Configure replication
and `min.insync.replicas` for your durability requirements. Set topic retention
for the replay period consumers need; log compaction discards earlier events
with the same key.

Drain pending deliveries before changing the broker, sink ID, topic, or partition
layout. Reenabling the same sink resumes its queue. Broker rejections, including
oversized records and missing permissions, require operator correction.

Use the existing OpenTelemetry configuration to monitor these metrics by sink ID:

- `basyx.eventing.pending`: queued deliveries.
- `basyx.eventing.pending.oldest.age`: age of the oldest pending event in seconds.
- `basyx.eventing.delivery.failures`: failed delivery attempts or worker database errors.
