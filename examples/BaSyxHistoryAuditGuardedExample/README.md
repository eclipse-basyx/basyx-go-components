<!--
Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
SPDX-License-Identifier: MIT
-->

# BaSyx History Audit Guarded Example

This example shows how to start the AAS Environment Service with AAS v3.2 audit history and PostgreSQL mutation guards enabled.

For the complete feature description, see:

- [AAS API v3.2 user guide](../../docu/user/aas_api_v3_2.md)
- [AAS API v3.2 runtime notes](../../docu/developer/aas_v3_2_runtime.md)

The important settings are:

- `BASYX_HISTORY_MODE=audit`
- `BASYX_HISTORY_RETENTION_DAYS=0`
- `BASYX_HISTORY_IMMUTABILITY=postgres_guarded`
- `BASYX_AUDIT_IDENTITY_MODE=none`

## Start

```bash
docker compose up -d
```

The configuration service initializes the `v1.1.1` schema first. The AAS Environment Service then enables the history guard switch at startup.

`BASYX_HISTORY_RETENTION_DAYS=0` is accepted as configuration, but automatic retention cleanup is not implemented yet. This example keeps every appended history row.

## What Guarded Mode Does

When guarded mode is active, PostgreSQL triggers block normal mutations on these history tables:

- `aas_history`
- `submodel_history`
- `concept_description_history`
- `descriptor_history`

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
