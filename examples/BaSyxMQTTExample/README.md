# MQTT and HTTP CloudEvents example

Start this checkout's services with PostgreSQL and Mosquitto:

```sh
cd examples/BaSyxMQTTExample
docker compose -f docker-compose.yml -f docker-compose.local.yml up --build -d
```

The AAS Environment API is at `http://localhost:8084`, with its HTTP feed at
`http://localhost:8084/events`. MQTT listens on localhost port 1883. This local
example has anonymous access; production authentication, TLS, and broker ACLs
are described in the [MQTT guide](../../docu/user/mqtt_eventing.md).
History and WORM evidence are disabled, and both event transports are enabled.

Subscribe before changing a resource:

```sh
docker compose exec mqtt mosquitto_sub -V mqttv5 -t 'basyx/#' -v
```

Create a Submodel in another terminal:

```sh
curl -X POST http://localhost:8084/submodels \
  -H 'Content-Type: application/json' \
  -d '{"id":"urn:example:mqtt:submodel","modelType":"Submodel","submodelElements":[]}'
```

The broker receives a structured CloudEvent on
`basyx/submodelrepository/submodel/created`. The HTTP feed returns the same event
ID, timestamp, schema, and data. Run the automated parity smoke check:

```sh
python3 smoke.py
```

Pause and resume the broker to try durable delivery:

```sh
docker compose pause mqtt
# Make further model changes through the HTTP API.
docker compose unpause mqtt
```

Pending events resume automatically. To run only MQTT, set
`BASYX_EVENTING_FEED_ENABLED` to `false`. Stop the example with:

```sh
docker compose -f docker-compose.yml -f docker-compose.local.yml down -v
```
