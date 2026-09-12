# Kafka CloudEvents Example

Edit a model in the BaSyx Web UI and inspect its CloudEvents in Kafbat UI.
This local playground includes an AAS Environment with sample data, the BaSyx
Web UI, PostgreSQL, Apache Kafka in KRaft mode, and a Kafka record browser.
It uses anonymous access and binds published ports to localhost.

## Start

Requires Docker Compose and free ports `3000`, `8082`, `8080`, and `9092`.
From this directory, start the published SNAPSHOT images:

```sh
docker compose up -d
```

Use a BaSyx SNAPSHOT containing Kafka eventing. To try an unmerged checkout,
build its images first from the repository root:

```sh
docker build -t eclipsebasyx/aasenvironment-go:SNAPSHOT -f cmd/aasenvironmentservice/Dockerfile .
docker build -t eclipsebasyx/basyxconfigurationservice-go:SNAPSHOT -f cmd/basyxconfigurationservice/Dockerfile .
```

## Change a property and inspect its event

1. Open the [BaSyx Web UI](http://localhost:3000). Select **Kafka Playground**
   under **Settings → Select Infrastructure** if another setup is remembered.
2. Open **KafkaPlayground → OperatingStatus** and change **Status** from `ready`
   to another value. Save the change.
3. Open [Kafbat UI](http://localhost:8080), select **BaSyx → Topics → basyx.events**,
   and open **Messages**. Fetch records from the beginning and find the event
   with subject `urn:example:kafka:submodel:status`.
4. Inspect the key, headers, and JSON value. The type is
   `io.admin-shell.submodel.updated.v1`. The payload identifies the changed
   model; it does not contain the property's new value.

The three-partition topic contains all event families. The entity key keeps
related events in the same partition. Kafka retains earlier records, so a
consumer can connect after the change and still read it.

Alternatively, inspect headers, keys, and values from a terminal:

```sh
docker compose exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server kafka:19092 --topic basyx.events --from-beginning \
  --property print.key=true --property print.headers=true
```

Host clients use `localhost:9092`; container clients use `kafka:19092`.
Kafbat UI is configured read-only. This single-broker anonymous setup is a local
demo; see the [Kafka guide](../../docu/user/kafka_eventing.md) for deployment settings.

## Verify and stop

```sh
python3 smoke.py
docker compose stop
```

Resume with `docker compose start`. To remove the example and its data:

```sh
docker compose down -v
```
