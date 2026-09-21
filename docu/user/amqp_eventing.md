# AMQP Eventing (experimental)

Publish AAS, asset, Submodel, and PCN changes as CloudEvents over AMQP 1.0 from
AAS Repository, Submodel Repository, or AAS Environment. To try it locally,
use the [RabbitMQ example](../../examples/BaSyxAMQPExample/README.md).

## Enable AMQP

Apply database schema v1.2.1 or later using the configuration service. Provision
the destination in your broker before starting the application. For RabbitMQ 4,
create a durable queue named `basyx.events` and configure:

```yaml
general:
  externalUrl: https://example.com/api/v3
eventing:
  enabled: true
  format: cloudevents
  sinks: [amqp]
  outboxEnabled: true
  amqp:
    broker: amqps://rabbitmq.example.com:5671
    address: /queues/basyx.events
    usernameFile: /run/secrets/amqp-user
    passwordFile: /run/secrets/amqp-password
```

All three activation settings are required: `enabled`, `sinks`, and `outboxEnabled`.
AMQP may run alongside MQTT, Kafka, and the optional event feed. The public API
URL supplies the event source and hosted schema URLs; override them through
`eventing.sourceBaseUrl` and `eventing.schemaBaseUrl` when necessary.

## Connection settings

Settings below are under `eventing.amqp`. Environment variables use the prefix
`BASYX_EVENTING_AMQP_` followed by the suffix shown.

| Setting | Environment suffix | Default / behavior |
| --- | --- | --- |
| `broker` | `BROKER` | Required `amqp://host[:port]` or `amqps://host[:port]`; default ports 5672 and 5671. No embedded credentials, paths, queries, or fragments. |
| `address` | `ADDRESS` | Required AMQP target address. RabbitMQ 4 accepts `/queues/basyx.events` or `/exchanges/<exchange>/<routing-key>`. |
| `sinkId` | `SINK_ID` | `amqp`; stable delivery queue identifier. Replicas sharing a database use the same ID and broker destination. Other transports require distinct IDs. |
| `hostName` | `HOST_NAME` | Optional AMQP connection hostname. RabbitMQ selects a virtual host with `vhost:<name>`; empty uses the broker host. TLS verification always uses the broker's network hostname. |
| `username`, `password` | `USERNAME`, `PASSWORD` | Supply together for SASL PLAIN; omitted from effective configuration output. Without credentials, SASL is disabled. |
| `usernameFile`, `passwordFile` | `USERNAME_FILE`, `PASSWORD_FILE` | Mounted credential alternatives; trailing line endings are removed. |
| `caFile` | `CA_FILE` | Optional PEM CA bundle; otherwise system trust. Requires `amqps`. |
| `certificateFile`, `keyFile` | `CERTIFICATE_FILE`, `KEY_FILE` | PEM client certificate/key pair for mutual TLS. Requires `amqps`. |

Supply either a value or a file for each credential, and supply client certificate
and key together. `amqps` verifies certificates using TLS 1.2 or later. Use TLS
when transmitting passwords. Mutual TLS can accompany SASL PLAIN; SASL EXTERNAL
and AMQP 0-9-1 are not supported by this sink.

Broker ACLs control publication and consumption; HTTP authorization does not
filter broker subscribers. RabbitMQ queue targets require write permission to
the default exchange. BaSyx does not create queues, exchanges, or bindings.

## Consume events

The sink implements the
[CloudEvents AMQP 1.0 structured JSON binding v1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/amqp-protocol-binding.md).
The AMQP data section contains the unchanged JSON envelope, with message
content type `application/cloudevents+json` and the durable header set to true.
The envelope's `datacontenttype` remains `application/json`.

Filter by `type` or `subject`. Change payloads identify affected models; PCN
payloads contain the new record's value-only representation. `dataschema` links
to the payload schema hosted by BaSyx, including when the event feed is disabled.

For example, inside a Go application with a caller-provided `ctx`, the
`github.com/Azure/go-amqp` client can consume the example queue:

```go
conn, err := amqp.Dial(ctx, "amqp://localhost:5672", &amqp.ConnOptions{
    SASLType: amqp.SASLTypePlain("basyx", "basyx-demo"),
})
if err != nil { return err }
defer conn.Close()
session, err := conn.NewSession(ctx, nil)
if err != nil { return err }
receiver, err := session.NewReceiver(ctx, "/queues/basyx.events", nil)
if err != nil { return err }
message, err := receiver.Receive(ctx, nil)
if err != nil { return err }
// Decode message.GetData(), process the CloudEvent, then acknowledge success.
if err := processCloudEvent(message.GetData()); err != nil { return err }
return receiver.AcceptMessage(ctx, message)
```

Queue consumers share work. For independent subscribers, publish to an exchange
and provision a separate bound queue for each application.

## Delivery and operation

Delivery is at least once: deduplicate using CloudEvents ID. BaSyx preserves the
existing outbox order per entity; there is no global order. Consumer concurrency,
redelivery, and ambiguous acknowledgements can affect observed processing order.

Only an AMQP `Accepted` outcome clears an outbox entry. Rejected, released
(unroutable), modified, and failed deliveries remain pending for retry. Broker
acceptance is separate from downstream consumer processing. Durable messages
require suitable durable queues and broker replication for your durability needs.

Connection establishment is bounded to five seconds and delivery attempts to ten
seconds, or an earlier caller deadline. Invalid local settings fail startup;
broker outages do not prevent startup or model writes. Failed connections are
recreated on retry. Each sink uses independent existing outbox workers.

Pending events stay in PostgreSQL without expiry and retry indefinitely. Allow
capacity for the expected outage; a failed event delays later events for the same
entity. Missing destinations, access denial, and oversized messages require
operator correction. Align broker message-size limits with PCN payload sizes.

Drain pending deliveries before changing broker, sink ID, or destination.
Destinations are stored with queued events; restarting does not rewrite them.
Reenabling the same sink resumes its queue.

Use existing OpenTelemetry metrics, filtered by sink ID:

- `basyx.eventing.pending`: queued deliveries.
- `basyx.eventing.pending.oldest.age`: oldest pending event age in seconds.
- `basyx.eventing.delivery.failures`: failed attempts or worker database errors.
