# Event Feed Example

A local playground for the experimental CloudEvents REST feed, with an AAS
Environment, BaSyx Web UI, and PostgreSQL. Eventing is explicitly enabled here;
the standard service configurations keep it disabled.
The Web UI uses the `mono-all` infrastructure template with one AAS Environment URL.

The example binds to localhost and uses anonymous access (`ABAC_ENABLED=false`).
It is intended for functional exploration. See the [Event Feed documentation](../../docu/user/event_feed.md)
for authorization behavior and secured deployments.

## Start with locally built Go images

Requires Docker Compose and free ports `8082` and `3001`. From this directory:

```sh
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build
```

This builds the AAS Environment and configuration service from the current
checkout. The configuration service initializes the database before the backend
starts. The Web UI uses its published image.

To use published images for all services instead, run `docker compose up -d`.
The published Go images must include the Event Feed feature and schema `v1.2.0`;
use the local build while this feature is still under review.

## Explore

| Endpoint | URL |
| --- | --- |
| Web UI | http://localhost:3001 |
| Swagger UI | http://localhost:8082/swagger |
| Event Feed | http://localhost:8082/events |
| Capabilities | http://localhost:8082/.well-known/event-feed.json |

The startup configuration loads the `EventFeedPlayground` AAS from
[`aas/playground.json`](aas/playground.json), with two Submodels:

- **NoSemanticId** has a `Status` property and deliberately has no semantic ID.
- **ProductChangeNotifications** contains the initial PCN record `CN1`.

Select the AAS in the Web UI and edit `NoSemanticId.Status`. Refresh `/events`
to see the resulting Submodel update. You can also use Swagger for CRUD requests.
Records appear after the background publisher runs, normally within a second.
If your browser remembers another connection on this port, choose
**Settings → Select Infrastructure → Event Feed Playground** first.

Fetch the compact presentation:

```sh
curl --fail --silent 'http://localhost:8082/events?presentation=COMPACT'
```

Add a second PCN record from this directory:

```sh
curl --fail --request POST \
  'http://localhost:8082/submodels/dXJuOmV4YW1wbGU6ZXZlbnRpbmc6c3VibW9kZWw6cGNu/submodel-elements/Records' \
  --header 'Content-Type: application/json' \
  --data-binary @pcn-cn2.json
```

This emits a Submodel update and a PCN notification for `CN2`. A second POST of
the same record conflicts with its existing `idShort`; change the record's
`idShort` and `ManufacturerChangeID` to add further notifications.

Use `limit=1` to inspect pagination and follow the returned `cursor`. Cursors
preserve filters and presentation. For ongoing polling with `lastEventId`,
deduplicate events by ID. Event schemas are linked through each record's
`dataschema` URL and served by this backend.

## Smoke test

With the stack running and Python 3 installed:

```sh
./smoke.sh
```

The smoke test checks the seeded models and the UI's infrastructure configuration,
then uses temporary models to verify capabilities, served schemas,
events without semantic IDs, CRUD events, both presentations, and PCN delivery.
It cleans up its models and leaves the playground sample intact. The resulting
test events remain in the feed until retention removes them. Override
`BASYX_EVENT_FEED_BASE_URL` and `BASYX_EVENT_FEED_UI_BASE_URL` to check another
instance of this example.

The `Event Feed Example` job in the Examples Smoke Tests workflow builds the
Go images from the PR, starts this stack, and runs the same smoke test.

## Stop and restart

```sh
docker compose -f docker-compose.yml -f docker-compose.local.yml down
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build
```

The named database volume preserves your changes and events. Startup
preconfiguration follows the hosting service's import behavior. To reset the
playground, stop it with `down -v` instead; this deletes this example's database.
