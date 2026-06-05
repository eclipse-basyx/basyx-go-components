<!--
Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
SPDX-License-Identifier: MIT
-->

# BaSyx History Audit Guarded Example

This example shows how to start the AAS Environment Service with AAS v3.2 audit history, PostgreSQL mutation guards, and an S3-compatible WORM evidence test backend enabled.
It also includes the BaSyx UI and preloads one AASX package at startup.

For the complete feature description, see:

- [AAS API v3.2 user guide](../../docu/user/aas_api_v3_2.md)
- [AAS API v3.2 runtime notes](../../docu/developer/aas_v3_2_runtime.md)

The important settings are:

- `BASYX_HISTORY_MODE=audit`
- `BASYX_HISTORY_RETENTION_DAYS=0`
- `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=5`
- `BASYX_HISTORY_IMMUTABILITY=postgres_guarded`
- `BASYX_AUDIT_IDENTITY_MODE=none`
- `BASYX_HISTORY_EVIDENCE_ENABLED=true`
- `BASYX_HISTORY_EVIDENCE_PROVIDER=s3`
- `BASYX_HISTORY_EVIDENCE_BUCKET=basyx-history-evidence`
- `BASYX_HISTORY_EVIDENCE_RETENTION_MODE=governance`
- `BASYX_HISTORY_EVIDENCE_RETENTION_DAYS=7`
- `BASYX_HISTORY_EVIDENCE_WRITE_TIMEOUT_SECONDS=10`
- `BASYX_HISTORY_INTEGRITY_ANCHOR_PROVIDER=none`

## Start

```bash
docker compose up -d
```

This example is configured for local development:

- `aas-environment` and `basyx_configuration` use local Docker `build` entries.
- The published image lines are kept as comments so you can switch back quickly.

The configuration service initializes the current schema first. The AAS Environment Service then enables the history guard switch at startup. The MinIO init sidecar creates a versioned object-lock bucket for local evidence tests.

`BASYX_HISTORY_RETENTION_DAYS=0` is accepted as configuration, but automatic retention cleanup is not implemented yet. `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=5` stores periodic full checkpoints with RFC 6902 diff rows between them.
When evidence is enabled, history mode must be `api` or `audit`. Each successful history append writes a WORM `history_event` artifact to MinIO before PostgreSQL commits. The artifact stores the recovery payload plus `effective_diff`, which records the actual JSON Patch relative to the previous version for audit attribution. If MinIO is unavailable or rejects the object write, the API mutation fails and the PostgreSQL transaction rolls back.

## UI And Preconfigured Data

- UI: [http://localhost:3000](http://localhost:3000)
- Backend: [http://localhost:8082](http://localhost:8082)
- MinIO API: [http://localhost:9000](http://localhost:9000)
- MinIO Console: [http://localhost:9001](http://localhost:9001), login `minioadmin` / `minioadmin`
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

## Inspect, Publish, And Verify WORM Evidence

MinIO is included only as a local/test S3-compatible Object Lock backend. Production deployments should use an operated S3-compatible WORM service, such as [AWS S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html), with versioning and retention configured by operations. MinIO Object Lock behavior is documented in the [MinIO object retention guide](https://minio.community/community/minio-object-store/administration/object-management/object-retention.html).

Create or update an AAS or Submodel through the UI or API, then inspect the history rows:

```bash
docker compose exec db psql -U admin -d basyxTestDB -c \
  "SELECT history_id, identifier, payload_type, row_hash FROM submodel_history ORDER BY history_id"
```

Inspect the automatic WORM history-event receipts. These are written during the API transaction and exist independently from later manifest publication:

```bash
docker compose exec db psql -U admin -d basyxTestDB -c \
  "SELECT artifact_type, history_table, identifier, history_id, object_key, sha256 FROM history_evidence_artifacts WHERE artifact_type = 'history_event' ORDER BY artifact_id"
```

Publish a signed or unsigned manifest for a Submodel history range from the repository root. This verifies the PostgreSQL hash chain, writes range manifest/checkpoint artifacts, and records their receipts. Adjust `-identifier`, `-from`, and `-to` to values from the query above:

```bash
POSTGRES_HOST=localhost \
POSTGRES_PORT=5432 \
POSTGRES_USER=admin \
POSTGRES_PASSWORD=admin123 \
POSTGRES_DBNAME=basyxTestDB \
BASYX_HISTORY_MODE=audit \
BASYX_HISTORY_IMMUTABILITY=postgres_guarded \
BASYX_HISTORY_EVIDENCE_ENABLED=true \
BASYX_HISTORY_EVIDENCE_PROVIDER=s3 \
BASYX_HISTORY_EVIDENCE_BUCKET=basyx-history-evidence \
BASYX_HISTORY_EVIDENCE_PREFIX=history-evidence \
BASYX_HISTORY_EVIDENCE_REGION=us-east-1 \
BASYX_HISTORY_EVIDENCE_ENDPOINT=http://localhost:9000 \
BASYX_HISTORY_EVIDENCE_ACCESS_KEY_ID=minioadmin \
BASYX_HISTORY_EVIDENCE_SECRET_ACCESS_KEY=minioadmin \
BASYX_HISTORY_EVIDENCE_PATH_STYLE=true \
BASYX_HISTORY_EVIDENCE_RETENTION_MODE=governance \
BASYX_HISTORY_EVIDENCE_RETENTION_DAYS=7 \
BASYX_HISTORY_EVIDENCE_WRITE_TIMEOUT_SECONDS=10 \
go run ./cmd/historyevidenceverifier \
  -table submodel_history \
  -identifier '<submodel-identifier>' \
  -from 1 \
  -to 5 \
  -write
```

The command prints the manifest, object-store receipt, per-row history-event receipts, snapshot checkpoint receipts, and PostgreSQL catalog id. The same object receipts are also stored in `history_evidence_manifests` and `history_evidence_artifacts`.

Verify PostgreSQL against the per-row WORM history-event artifacts and, when supplied, a stored manifest:

```bash
POSTGRES_HOST=localhost \
POSTGRES_PORT=5432 \
POSTGRES_USER=admin \
POSTGRES_PASSWORD=admin123 \
POSTGRES_DBNAME=basyxTestDB \
BASYX_HISTORY_EVIDENCE_PROVIDER=s3 \
BASYX_HISTORY_EVIDENCE_BUCKET=basyx-history-evidence \
BASYX_HISTORY_EVIDENCE_REGION=us-east-1 \
BASYX_HISTORY_EVIDENCE_ENDPOINT=http://localhost:9000 \
BASYX_HISTORY_EVIDENCE_ACCESS_KEY_ID=minioadmin \
BASYX_HISTORY_EVIDENCE_SECRET_ACCESS_KEY=minioadmin \
BASYX_HISTORY_EVIDENCE_PATH_STYLE=true \
go run ./cmd/historyevidenceverifier \
  -table submodel_history \
  -identifier '<submodel-identifier>' \
  -from 1 \
  -to 5 \
  -manifest-object-key '<object-key-from-write-output>' \
  -manifest-sha256 '<sha256-from-write-output>'
```

History-event artifacts provide recovery evidence for acknowledged writes while evidence is enabled. With `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=5`, recovery from WORM starts from the latest WORM-stored snapshot event and replays up to four WORM-stored diff payloads. Use `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=1` when every history row must be recoverable as a full WORM snapshot without diff replay. For audit attribution, inspect `effective_diff`: a full snapshot event can be a recovery checkpoint, while `effective_diff` shows what the request actually changed. Automated PostgreSQL restore from WORM artifacts is not implemented in this example.

## Limitations

This mode protects against accidental or unauthorized mutation through normal database access. PostgreSQL superusers or operators with enough permissions can still alter triggers, functions, or schema objects. Signed manifests and WORM object storage strengthen tamper evidence, but they must be operated with appropriate retention, key management, monitoring, and backup procedures.

For local development or tests that truncate tables, use `BASYX_HISTORY_IMMUTABILITY=none` from the start. To disable an already-enabled guard for maintenance, stop the guarded services and explicitly update `history_guard_config` as a database operator before cleanup.

## Stop

```bash
docker compose down
```

Remove the database volume as well:

```bash
docker compose down -v
```
