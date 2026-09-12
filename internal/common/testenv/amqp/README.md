# RabbitMQ integration fixture

RabbitMQ 4 provides AMQP 1.0 and an HTTP management endpoint for provisioning
isolated subscriber queues. Tests publish to the `basyx.events` fanout exchange.
The durable fallback queue keeps publications routable between subscriptions.

Certificates are copies of the disposable MQTT test certificate set: the server
covers localhost and 127.0.0.1. These publicly committed keys are for tests only.
The TLS listener requires a client certificate and SASL PLAIN credentials.
