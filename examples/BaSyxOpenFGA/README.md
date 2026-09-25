# OpenFGA ReBAC example

This experimental example runs a ReBAC-enabled BaSyx AAS Environment and the
BaSyx AAS Web UI. PostgreSQL, the Configuration Service, OpenFGA and Keycloak
are included as required infrastructure.

The example deliberately does not configure OpenTelemetry or immutable evidence
storage. Those capabilities are covered by their dedicated examples.

## Start

Run from this directory:

```sh
./start.sh
```

Open:

- AAS UI: [http://localhost:28030](http://localhost:28030)
- AAS Environment: [http://localhost:28082](http://localhost:28082)

The startup script builds the current Go checkout, generates and publishes its
OpenFGA model, and pins the resulting store and model IDs in `.env`.

Use one of these local accounts in the UI:

| User | Password | Purpose |
|---|---|---|
| `owner` | `demo-owner` | ABAC and ReBAC administrator; owns created resources |
| `reader` | `demo-reader` | Receives access through ReBAC grants |
| `fallback` | `demo-fallback` | Demonstrates unchanged ABAC read fallback |

The AAS Environment advertises
`https://basyx.org/aas/API/3/2/ResourceBoundAccessControl/1.0` in its authenticated
`/description` response. The UI uses that profile to enable its access-management
controls.

Run the compact authorization smoke check after startup:

```sh
python3 smoke.py
```

It verifies the advertised profile, creator ownership, sharing, revocation,
ABAC fallback and fail-closed behavior while OpenFGA is unavailable.

## Stop and reset

Retain the database and pinned OpenFGA IDs:

```sh
docker compose down
```

Reset the complete disposable example:

```sh
docker compose down --volumes
rm .env
```

Remove `.env` only together with the database volume. Existing BaSyx resources
are bound to the pinned OpenFGA store and authorization model.

All ports bind to localhost. The credentials in this example are for local
development only.
