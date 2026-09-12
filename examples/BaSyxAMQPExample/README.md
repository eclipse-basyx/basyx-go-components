# AMQP 1.0 CloudEvents Example

Change a property in the BaSyx Web UI and inspect its event in RabbitMQ.
The BaSyx API uses anonymous access; RabbitMQ uses local demo credentials.

## Start

Requires Docker Compose and free ports `3000`, `8082`, `5672`, and `15672`.
From this directory, run:

```sh
docker compose up -d
```

## Change a property and find its event

1. Open the [BaSyx Web UI](http://localhost:3000). If another setup is selected,
   choose **Settings → Select Infrastructure → AMQP Playground**.
2. Open **AMQPPlayground → OperatingStatus**. Change **Status** from `ready`
   to another value and save it.
3. Open [RabbitMQ management](http://localhost:15672) and sign in with username
   `basyx` and password `basyx-demo`.
4. Select **Queues and Streams → basyx.events → Get messages**. Choose
   **Nack message requeue true** to keep the messages, then select **Get Message(s)**.
5. Find the event with subject `urn:example:amqp:submodel:status`. Its type is
   `io.admin-shell.submodel.updated.v1`, and its content type is
   `application/cloudevents+json`. The payload identifies the affected Submodel;
   it does not include the property's new value.

For a native AMQP 1.0 consumer, connect to `amqp://localhost:5672` with the same
credentials and receive from `/queues/basyx.events`. Acknowledging a message
removes it from this queue. Multiple consumers of this queue share work; use
an exchange with separate bound queues if each application needs its own copy.

For deployment settings and a Go consumer, see the
[AMQP configuration guide](../../docu/user/amqp_eventing.md).

## Check and stop

With Python 3 installed, check event publication automatically:

```sh
python3 smoke.py
```

The check creates and deletes a temporary Submodel and reads queue messages
with requeue enabled. For a very busy queue, run it in a fresh playground.

Stop with `docker compose stop` and resume with `docker compose start`.
To remove the playground and its data:

```sh
docker compose down -v
```
