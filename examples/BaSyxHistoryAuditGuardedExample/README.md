<!--
Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
SPDX-License-Identifier: MIT
-->

# BaSyx History Audit Guarded Example

This example shows how to start the AAS Environment Service with AAS v3.2 audit history and PostgreSQL mutation guards enabled.
It also includes the BaSyx UI and preloads one AASX package at startup.

For the complete feature description, see:

- [AAS API v3.2 user guide](../../docu/user/aas_api_v3_2.md)
- [AAS API v3.2 runtime notes](../../docu/developer/aas_v3_2_runtime.md)

The important settings are:

- `BASYX_HISTORY_MODE=audit`
- `BASYX_HISTORY_RETENTION_DAYS=0`
- `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=1`
- `BASYX_HISTORY_IMMUTABILITY=postgres_guarded`
- `BASYX_AUDIT_IDENTITY_MODE=none`

## Start

```bash
docker compose up -d
```

This example is configured for local development:

- `aas-environment` and `basyx_configuration` use local Docker `build` entries.
- The published image lines are kept as comments so you can switch back quickly.

The configuration service initializes the `v1.1.1` schema first. The AAS Environment Service then enables the history guard switch at startup.

`BASYX_HISTORY_RETENTION_DAYS=0` is accepted as configuration, but automatic retention cleanup is not implemented yet. `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=1` keeps the current behavior where every appended history row stores a complete snapshot.

## UI And Preconfigured Data

- UI: [http://localhost:3000](http://localhost:3000)
- Backend: [http://localhost:8082](http://localhost:8082)
- Preconfiguration path: `./aas` mounted to `/app/preconfiguration`

The file `aas/IESEDriveMotorDM3000.aasx` is imported automatically via `GENERAL_AAS_PRECONFIG_PATHS=/app/preconfiguration`.

## What Guarded Mode Does

When guarded mode is active, PostgreSQL triggers block normal mutations on these history tables:

- `aas_history`
- `aas_history_payload`
- `submodel_history`
- `submodel_history_payload`
- `concept_description_history`
- `concept_description_history_payload`
- `descriptor_history`
- `descriptor_history_payload`

The blocked operations are `UPDATE`, `DELETE`, and `TRUNCATE`. PostgreSQL returns the error message `history tables are append-only`.

## Verify The Guard

After the service is running, this command should fail with `history tables are append-only`:

```bash
docker compose exec db psql -U admin -d basyxTestDB -c "TRUNCATE aas_history"
```

Normal API writes still append new history rows.

The guard switch is database-wide. Configure every BaSyx runtime service sharing this database consistently. Normal service startup can enable the guard but cannot disable it. A service configured with `BASYX_HISTORY_IMMUTABILITY=none` fails fast when the database guard is already enabled.

## Limitations

This mode protects against accidental or unauthorized mutation through normal database access. PostgreSQL superusers or operators with enough permissions can still alter triggers, functions, or schema objects. Use external anchoring, WORM storage, or transparency-log infrastructure when stronger tamper-resistance is required.

For local development or tests that truncate tables, use `BASYX_HISTORY_IMMUTABILITY=none` from the start. To disable an already-enabled guard for maintenance, stop the guarded services and explicitly update `history_guard_config` as a database operator before cleanup.

## Stop

```bash
docker compose down
```

Remove the database volume as well:

```bash
docker compose down -v
```
