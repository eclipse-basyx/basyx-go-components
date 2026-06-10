<!--
Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
SPDX-License-Identifier: MIT
-->

# BaSyx History Audit Guarded Example

This example shows how to start the AAS Environment Service with AAS v3.2 audit history, PostgreSQL mutation guards, Keycloak-backed OIDC/ABAC authorization, and an S3-compatible WORM evidence test backend enabled.
It also includes the BaSyx UI and preloads one AASX package at startup.

For the complete feature description, see:

- [AAS API v3.2 user guide](../../docu/user/aas_api_v3_2.md)
- [AAS API v3.2 runtime notes](../../docu/developer/aas_v3_2_runtime.md)

The important settings are:

- `BASYX_HISTORY_MODE=audit`
- `BASYX_HISTORY_RETENTION_DAYS=0`
- `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=5`
- `BASYX_HISTORY_IMMUTABILITY=postgres_guarded`
- `BASYX_AUDIT_IDENTITY_MODE=extended`
- `BASYX_HISTORY_EVIDENCE_ENABLED=true`
- `BASYX_HISTORY_EVIDENCE_PROVIDER=s3`
- `BASYX_HISTORY_EVIDENCE_BUCKET=basyx-history-evidence`
- `BASYX_HISTORY_EVIDENCE_RETENTION_MODE=compliance`
- `BASYX_HISTORY_EVIDENCE_RETENTION_DAYS=7`
- `BASYX_HISTORY_EVIDENCE_WRITE_TIMEOUT_SECONDS=10`
- `BASYX_HISTORY_INTEGRITY_ANCHOR_PROVIDER=none`
- `ABAC_ENABLED=true`
- `ABAC_MODELPATH=/security_env/access-rules.json`
- `ABAC_POLICY_FILE_IMPORT=if_missing`
- `ABAC_MANAGEMENT_API_ENABLED=true`
- `OIDC_TRUSTLISTPATH=/security_env/trustlist.json`

## Start

```bash
docker compose up -d
```

This example is configured for local development:

- `aas-environment` and `basyx_configuration` use local Docker `build` entries.
- The published image lines are kept as comments so you can switch back quickly.

The configuration service initializes the current schema first. Keycloak imports the local `basyx` realm, the MinIO init sidecar creates a versioned object-lock bucket for local evidence tests, and the AAS Environment Service then enables the history guard switch at startup.
The demo `basyx` realm sets `sslRequired=none`, and the Keycloak startup command relaxes the local `master` realm the same way so the admin console works over HTTP at [http://keycloak.localhost:8080](http://keycloak.localhost:8080). Do not carry those settings into a production Keycloak realm.
The `keycloak-healthcheck` container is a one-shot readiness gate and exits after Keycloak is ready and the local master realm reports `sslRequired=none`; the `keycloak` service itself should keep running.

`BASYX_HISTORY_RETENTION_DAYS=0` is accepted as configuration, but automatic retention cleanup is not implemented yet. `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=5` stores periodic full checkpoints with RFC 6902 diff rows between them.
When evidence is enabled, history mode must be `api` or `audit`. Each successful history append writes a WORM `history_event` artifact to MinIO before PostgreSQL commits. The artifact stores the recovery payload plus `effective_diff`, which records the actual JSON Patch relative to the previous version for audit attribution. The startup ABAC access-rule import also writes an `abac_policy_version` artifact when the policy version is activated. If MinIO is unavailable or rejects a required object write, the API mutation or ABAC policy activation fails and the PostgreSQL transaction rolls back.

`ABAC_POLICY_FILE_IMPORT=if_missing` imports `security_env/access-rules.json` only when no active database-backed policy exists for the AAS Environment Service scope. This keeps the first startup file import auditable and avoids replacing later database-managed policy versions on every restart. `ABAC_MANAGEMENT_API_ENABLED=true` opts this example into the protected `/security/abac/policy-versions` API and makes the same endpoints visible in Swagger UI.

`BASYX_AUDIT_IDENTITY_MODE=extended` stores request/correlation IDs from `X-Request-ID` and `X-Correlation-ID` when clients or trusted ingress set them, authenticated OIDC subject and issuer, client id, ABAC allow metadata, operation, endpoint, method, trusted source IP, user agent, and a hash of the configured ABAC policy where available. BaSyx does not generate HTTP request/correlation IDs when those headers are missing. Startup preconfiguration rows are created outside an HTTP request, so they use a synthetic system audit context such as `system:aas-preconfiguration`, `basyx:aasenvironmentservice`, `aasenvironmentservice`, `SYSTEM_INTERNAL`, and `SYSTEM`. Perform an authenticated API mutation to see end-user OIDC and ABAC audit context.

## UI And Preconfigured Data

- UI: [http://localhost:3000](http://localhost:3000)
- Backend: [http://localhost:8082](http://localhost:8082)
- Keycloak: [http://keycloak.localhost:8080](http://keycloak.localhost:8080)
- MinIO API: [http://localhost:9000](http://localhost:9000)
- MinIO Console: [http://localhost:9001](http://localhost:9001), login `minioadmin` / `minioadmin`
- Preconfiguration path: `./aas` mounted to `/app/preconfiguration`

The file `aas/IESEDriveMotorDM3000.aasx` is imported automatically via `GENERAL_AAS_PRECONFIG_PATHS=/app/preconfiguration`.

## Security And Audit Demo

The example uses a compact Keycloak realm and ABAC model:

- Keycloak Admin Console: `admin` / `admin`
- BaSyx UI/API admin user: `admin` / `pwd`
- BaSyx UI/API read-only user: `usera` / `pwd`
- Access rules: [`security_env/access-rules.json`](security_env/access-rules.json)
- OIDC trust list: [`security_env/trustlist.json`](security_env/trustlist.json)

The admin role has full access to the AAS Environment routes. The viewer role has read access only. Anonymous requests can read `/description` and `/health`; writes require an authenticated admin token.

Open the UI at [http://localhost:3000](http://localhost:3000), log in as `admin` / `pwd`, and update an AAS or Submodel. You can also run a repeatable API mutation from a shell:
The shell commands below use `jq` for JSON extraction and `openssl` for base64url encoding.

```bash
TOKEN=$(curl -s -X POST \
  'http://keycloak.localhost:8080/realms/basyx/protocol/openid-connect/token' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'client_id=basyx-ui' \
  -d 'grant_type=password' \
  -d 'username=admin' \
  -d 'password=pwd' \
  -d 'scope=openid profile email' | jq -r .access_token)

SM_ID=$(printf '%s' 'urn:fraunhofer:iese:dte:sm:nameplate:drivemotor-dm3000:001' \
  | openssl base64 -A | tr '+/' '-_' | tr -d '=')

curl -i -X PATCH "http://localhost:8082/submodels/${SM_ID}/\$metadata" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-ID: history-audit-demo-001' \
  -H 'X-Correlation-ID: history-audit-demo' \
  -H 'User-Agent: basyx-history-audit-demo' \
  -d '{
    "description": [
      {
        "language": "en",
        "text": "Authenticated history audit demo update"
      }
    ]
  }'
```

Treat access tokens as credentials. Do not paste them into issue reports.

Inspect the PostgreSQL audit columns after the authenticated mutation:

```bash
docker compose exec db psql -U admin -d basyxTestDB -c \
  "SELECT history_id,
          payload_type,
          request_id,
          correlation_id,
          actor_subject,
          actor_issuer,
          client_id,
          authorization_result,
          COALESCE(policy_id, '') <> '' AS policy_id_present,
          source_ip,
          user_agent,
          operation,
          endpoint,
          http_method
     FROM submodel_history
 ORDER BY history_id DESC
    LIMIT 5"
```

For the authenticated API mutation, expect `actor_subject` and `actor_issuer` from Keycloak, `client_id` from the OIDC client, `authorization_result=ALLOW`, the request/correlation headers from the `curl` command, and the route/method that produced the row. `matched_rule_id` contains deterministic ABAC rule identifiers such as `rule:2:<hash-prefix>`; when multiple allow rules match, the identifiers are stored comma-separated in configured rule order.

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

Inspect the ABAC policy-version evidence receipt written during startup access-rule activation:

```bash
docker compose exec db psql -U admin -d basyxTestDB -c \
  "SELECT artifact_type, history_table, identifier, history_id, object_key, sha256 FROM history_evidence_artifacts WHERE artifact_type = 'abac_policy_version' ORDER BY artifact_id"
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
BASYX_HISTORY_EVIDENCE_RETENTION_MODE=compliance \
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

History-event artifacts provide recovery evidence for acknowledged writes while evidence is enabled. With `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=5`, recovery from WORM starts from the latest WORM-stored snapshot event and replays up to four WORM-stored diff payloads. Use `BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL=1` when every history row must be recoverable as a full WORM snapshot without diff replay. For audit attribution, inspect `effective_diff`: a full snapshot event can be a recovery checkpoint, while `effective_diff` shows what the request actually changed.

Export a recovery catalog and recover verified JSON from WORM artifacts:

```bash
go run ./cmd/historyevidenceverifier \
  -table submodel_history \
  -identifier '<submodel-identifier>' \
  -from 1 \
  -to 5 \
  -catalog-export \
  -out ./recovery-catalog.json
```

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
  -recover \
  -recovery-catalog ./recovery-catalog.json \
  -out ./recovered-history.json
```

The recovery command exports verified JSON only. Restoring rows into PostgreSQL remains an operator-controlled disaster-recovery procedure.

To inspect audit metadata recovered from WORM `history_event` artifacts, run:

```bash
jq '.recovered_rows[] | {history_id, audit}' ./recovered-history.json
```

The audit object in the recovered JSON should match the PostgreSQL audit columns for the same `history_id`.

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
