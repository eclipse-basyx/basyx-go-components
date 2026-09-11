# Event Feed Example

See how changes to an AAS become events: edit a property, add a product change
notification (PCN), and read the resulting feed. This local playground includes
a BaSyx Web UI and an AAS Environment with sample data and eventing already enabled.
It uses anonymous access, like the minimal example.

## Start the playground

Requires Docker Compose and free ports `8082` and `3000`. From this directory:

```sh
docker compose up -d
```

This uses the published SNAPSHOT images, like the minimal example.
Once startup completes, open the [Web UI](http://localhost:3000) and select
**EventFeedPlayground**. If your browser remembers another setup, first choose
**Settings → Select Infrastructure → Event Feed Playground**.

To build the Go services from your checkout instead, use the optional override:

```sh
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build
```

## Change a property

1. Open the **NoSemanticId** Submodel and change its **Status** property from
   `ready` to another value in the editor. Save the change.
2. Open the [Event Feed in Swagger](http://localhost:8082/swagger) and execute
   `GET /events`, or open the [JSON feed](http://localhost:8082/events) directly.
3. Look for an `io.admin-shell.submodel.updated.v1` event whose `subject` is
   `urn:example:eventing:submodel:no-semantic-id`.

Allow a moment for the event to appear, then refresh. This Submodel deliberately
has no semantic ID: it can still produce events.

## Add a product change notification

The **ProductChangeNotifications** Submodel starts with **CN1**. The supplied
records demonstrate notification delivery and omit fields needed for a complete
PCN document. Add the **CN2** record from this directory:

```sh
curl --fail --request POST \
  'http://localhost:8082/submodels/dXJuOmV4YW1wbGU6ZXZlbnRpbmc6c3VibW9kZWw6cGNu/submodel-elements/Records' \
  --header 'Content-Type: application/json' \
  --data-binary @pcn-cn2.json
```

Refresh the feed. This one action produces **two events** for the PCN Submodel:
a Submodel update and a PCN notification. To add another record, give it a new
`idShort` and `ManufacturerChangeID`; posting CN2 again conflicts with the existing record.

## Understand what you see

The feed includes retained events across the environment, including startup
imports and earlier experiments. Reading it does not remove events. Check each
event's `subject`, `type`, and `time` to identify the change you made.

With only `limit=1`, you start at the first published event still available to
you, then follow the returned `cursor` forward. This is not a newest-first
activity list. To focus on your PCN changes, set Swagger's `filter` to:

```text
rsql:event.subject==urn:example:eventing:submodel:pcn
```

The [Event Feed user guide](../../docu/user/event_feed.md) explains filtering,
polling for new events, presentation modes, and configuration for your own setup.

## Check or stop the playground

With Python 3 installed, run `./smoke.sh` to check the example. The test creates
and removes its own models; their events remain in the feed.

Stop the services with:

```sh
docker compose down
```

If you started with the local build override, include both `-f` arguments when
stopping as well.

Like the minimal example, this setup does not declare a named database volume.
`docker compose stop` followed by `docker compose start` keeps the existing data
and events. After `docker compose down`, the next `up` uses a fresh database and
loads the sample data again. Use `docker compose down -v` to also remove the
anonymous database volume created by the PostgreSQL image.

## Differences from the minimal example

Ports, CORS, database settings, and UI endpoint configuration follow the minimal
example. The differences relevant to this playground are:

- `BASYX_EVENTING_FEED_ENABLED=true` enables the event feed.
- The seed data demonstrates updates without a semantic ID and PCN notifications.
- `pcn-cn2.json` and `smoke.sh` provide a sample change and a feed verification tool.

The optional local build override is useful for development but is not required
to run the example. The UI waits for a healthy backend before starting. Compose
generates container names, and a YAML anchor shares the database settings between
services. These are setup conveniences, not event-feed requirements. The minimal
example's JWS signing-key mount is omitted because this demo does not use signed
responses.
