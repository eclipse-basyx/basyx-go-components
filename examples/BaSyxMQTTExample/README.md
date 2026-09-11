# MQTT CloudEvents Example

Edit a model in the BaSyx Web UI and watch its changes arrive as CloudEvents in
an MQTT client. This local playground includes an AAS Environment with sample
data, a Web UI configured with the `mono-all` infrastructure template, and Mosquitto.
It uses anonymous access.

## Start the playground

Requires Docker Compose and free ports `3000`, `8082`, and `1883`.
From this directory, start the published SNAPSHOT images:

```sh
docker compose up -d
```

Open the [Web UI](http://localhost:3000) and select **MQTTPlayground**.
If your browser remembers another setup, choose
**Settings → Select Infrastructure → MQTT Playground** first.

## Connect MQTT Explorer

Create a connection with these settings:

- Host: `localhost`
- Port: `1883`
- Encryption/TLS: off
- Username and password: empty
- Subscription: `basyx/#` (the default `#` subscription also includes these events)

Alternatively, subscribe from a terminal:

```sh
docker compose exec mqtt mosquitto_sub -V mqttv5 -t 'basyx/#' -v
```

Connect before editing: messages are not retained, so startup events and changes
made before subscribing will not appear.

## Change a property

1. In the Web UI, open **MQTTPlayground → OperatingStatus**.
2. Edit the **Status** property from `ready` to another value and save it.
3. In MQTT Explorer, open `basyx/submodelrepository/submodel/updated`.

The event has type `io.admin-shell.submodel.updated.v1` and subject
`urn:example:mqtt:submodel:status`. Its `data` identifies the affected model;
it does not contain the new property value. Change the value again to generate
another event.

## Create a Submodel

You can also use the [Swagger UI](http://localhost:8082/swagger), or create a
Submodel from another terminal:

```sh
curl --fail -X POST http://localhost:8082/submodels \
  -H 'Content-Type: application/json' \
  -d '{"id":"urn:example:mqtt:submodel","modelType":"Submodel","submodelElements":[]}'
```

The subscriber receives a CloudEvent on
`basyx/submodelrepository/submodel/created`. Its `subject` identifies the Submodel,
`type` identifies the change, and `data` contains the affected model references.

For authentication, TLS, topic names, and delivery behavior, see the
[MQTT guide](../../docu/user/mqtt_eventing.md).

## Check and stop

With Python 3 installed, verify publication automatically:

```sh
python3 smoke.py
```

Stop the running services with `docker compose stop`. Resume with
`docker compose start`.

To remove the example and its data:

```sh
docker compose down -v
```
