# Kafka Eventing (experimental)

Publish existing AAS, asset, Submodel, and PCN CloudEvents to Kafka from AAS
Repository, Submodel Repository, or AAS Environment. Try the
[Kafka playground](../../examples/BaSyxKafkaExample/README.md) with BaSyx Web UI
and Kafbat UI.

## Enable Kafka

Use database schema v1.2.1 or later (the existing event outbox). No additional
migration is required. Configure the public API URL and provision the Kafka topic:

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

Set `sinks: [mqtt, kafka]` to publish to both brokers. Enable `eventing.feed.enabled`
independently to also expose the HTTP event feed. Each event is generated once;
all enabled destinations persist the same ID, timestamp, schema, and REGULAR
payload within the model transaction. Sink names and sink IDs must be unique.

Kafka alone also enables the existing event schema endpoints. Source/schema URL
overrides remain `eventing.sourceBaseUrl` and `eventing.schemaBaseUrl`.

## Configuration

Settings below are under `eventing.kafka`. Environment variables have prefix
`BASYX_EVENTING_KAFKA_` followed by the suffix shown.

| Setting | Environment suffix | Default / behavior |
| --- | --- | --- |
| `brokers` | `BROKERS` | Required bootstrap `host:port` list; comma-separated in the environment. No schemes or embedded credentials. |
| `topic` | `TOPIC` | `basyx.events`; one topic for all event families. |
| `sinkId` | `SINK_ID` | `kafka`; stable outbox destination identifier. |
| `clientId` | `CLIENT_ID` | `basyx`; client name reported to Kafka. |
| `tlsEnabled` | `TLS_ENABLED` | `false`; enable verified TLS, minimum TLS 1.2. |
| `caFile` | `CA_FILE` | Optional PEM CA bundle; otherwise system trust. |
| `certificateFile`, `keyFile` | `CERTIFICATE_FILE`, `KEY_FILE` | Optional PEM client certificate/key pair for mutual TLS. Requires TLS. |
| `saslMechanism` | `SASL_MECHANISM` | Empty disables SASL; supports `PLAIN`, `SCRAM-SHA-256`, `SCRAM-SHA-512`. |
| `username`, `password` | `USERNAME`, `PASSWORD` | Required when SASL is enabled; omitted from effective configuration output. |
| `usernameFile`, `passwordFile` | `USERNAME_FILE`, `PASSWORD_FILE` | Mounted credential alternatives; trailing line endings are removed. |

Use a value or file for each credential, never both. TLS certificate and key must
be supplied together. Credentials, certificate material, and invalid local
configuration are checked at startup; broker unavailability does not block startup.
Use TLS with SASL credentials, particularly PLAIN. Configure broker ACLs for the
topic's producer and consumers; HTTP authorization does not filter Kafka readers.

Shared activation variables are `BASYX_EVENTING_ENABLED`, `BASYX_EVENTING_SINKS`,
and `BASYX_EVENTING_OUTBOX_ENABLED`. `eventing.topicPrefix` continues to configure
MQTT topics; Kafka uses `eventing.kafka.topic`.

## CloudEvents mapping

The sink implements structured JSON from the
[CloudEvents Kafka binding v1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/kafka-protocol-binding.md).
Each Kafka record contains:

- Value: the complete unchanged CloudEvent JSON envelope, with `specversion: "1.0"`
  and `datacontenttype: "application/json"`.
- Header: `content-type: application/cloudevents+json`.
- Key: `aas_history:<AAS ID>` or `submodel_history:<Submodel ID>`, matching the
  outbox's mutated-entity identity. Derived asset and PCN events use the parent's key.

No Kafka-specific CloudEvent attributes, schema registry framing, or binary
encoding are added. The binding leaves topic layout to the application. Filter
by the existing event `type` or `subject` when consuming the shared topic.

## Delivery and operations

The outbox stores topic and key with the immutable event before commit. Four
workers per sink deliver committed records and require acknowledgment from all
in-sync replicas. Broker replication and `min.insync.replicas` determine durability;
configure them for the required availability and durability of production topics.

Failures retain the delivery indefinitely and use existing exponential backoff
with jitter (one second to one minute). Each publish attempt is bounded by ten
seconds. The producer supports cancellation of in-flight idempotent writes to
respect this deadline. Ambiguous acknowledgments or a crash after publication can
cause duplicates with the same CloudEvents ID; consumers must deduplicate.

Workers serialize events per mutated entity and use deterministic key partitioning.
Other entities and sinks can progress while one delivery retries. Kafka orders
records within a partition; there is no global ordering across partitions. Keep
partition count stable while relying on entity ordering. Deduplicate retries before
processing changes. Replicas sharing a database use the same sink ID and destination.

Provision topics before use; the application does not create them. Drain pending
deliveries before changing broker, sink ID, topic, or partition layout. Reenabling
the same sink resumes its queue. Set topic retention for consumer replay needs;
log compaction keeps only the latest record per entity key and discards event history.
Broker rejections, including oversized records or missing topic permissions, retain
the outbox row and require operator correction. Monitor database capacity during outages.

The existing `basyx.eventing.*` delivered/failure counters and pending/retry/oldest-age
gauges also cover Kafka, labeled by sink ID. The connection gauge reflects the latest
broker probe or publish result; probes and queue gauges refresh every 15 seconds.
