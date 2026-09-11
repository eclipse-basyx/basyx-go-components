# MQTT CloudEvents Example

Publish AAS and Submodel changes from an AAS Environment to Mosquitto.
Requires Docker Compose and free ports `8084` and `1883`.

## Start and subscribe

From this directory, start the published images and subscribe to events:

```sh
docker compose up -d
docker compose exec mqtt mosquitto_sub -V mqttv5 -t 'basyx/#' -v
```

This example uses anonymous access on localhost. For authentication and TLS,
see the [MQTT guide](../../docu/user/mqtt_eventing.md).

## Create a Submodel

In another terminal:

```sh
curl --fail -X POST http://localhost:8084/submodels \
  -H 'Content-Type: application/json' \
  -d '{"id":"urn:example:mqtt:submodel","modelType":"Submodel","submodelElements":[]}'
```

The subscriber receives a CloudEvent on
`basyx/submodelrepository/submodel/created`. Its `subject` identifies the Submodel,
`type` identifies the change, and `data` contains the affected model references.

## Check and stop

With Python 3 installed, verify publication automatically:

```sh
python3 smoke.py
```

Stop the example and remove its data:

```sh
docker compose down -v
```
