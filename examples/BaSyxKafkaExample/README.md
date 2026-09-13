# Kafka CloudEvents Example

Change a property in the BaSyx Web UI and inspect its event in Kafbat UI.
This local playground uses anonymous access.

## Start

Requires Docker Compose and free ports `3000`, `8082`, `8080`, and `9092`.
From this directory, run:

```sh
docker compose up -d
```

## Change a property and find its event

1. Open the [BaSyx Web UI](http://localhost:3000). If another setup is selected,
   choose **Settings → Select Infrastructure → Kafka Playground**.
2. Select **AAS Editor** from the top-left navigation menu.
3. Open **KafkaPlayground → OperatingStatus**. Change **Status** from `ready`
   to another value and save it.
4. Open [Kafbat UI](http://localhost:8080) and select
   **BaSyx → Topics → basyx.events → Messages**. Refresh the messages and find
   the subject `urn:example:kafka:submodel:status`.
5. Inspect the event's key, headers, and JSON value. Its type is
   `io.admin-shell.submodel.updated.v1`. The payload identifies the affected
   Submodel; it does not include the property's new value.

To read the events from a terminal instead:

```sh
docker compose exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server kafka:19092 --topic basyx.events --from-beginning \
  --property print.key=true --property print.headers=true
```

For your own deployment or Kafka client, see the
[Kafka configuration guide](../../docu/user/kafka_eventing.md).

## Check and stop

With Python 3 installed, check event publication automatically:

```sh
python3 smoke.py
```

Stop with `docker compose stop` and resume with `docker compose start`.
To remove the playground and its data:

```sh
docker compose down -v
```
