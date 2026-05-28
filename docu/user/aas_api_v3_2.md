<!--
/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/
-->

# AAS API v3.2 User Guide

This guide summarizes the user-visible AAS API v3.2 changes in the BaSyx Go components.

The most important additions are:

- Historical reads for AAS and Submodels.
- Recent-change feeds for AAS, Submodels, Concept Descriptions, and AAS descriptors.
- Signed AAS and Submodel reads.
- The new `Batch` value for `AssetKind`.
- Extended administrative timestamp fields used by recent-change filters.

Endpoint examples below are written relative to the service base path. If your service uses a context path such as `/api/v3`, prefix the paths with that context path.

## Endpoint Availability

Implemented history, recent-change, and signing endpoints:

- AAS Repository: `GET /shells/$recent-changes`
- AAS Repository: `GET /shells/{aasIdentifier}/$history`
- AAS Repository: `GET /shells/{aasIdentifier}/$signed`
- Submodel Repository: `GET /submodels/$recent-changes`
- Submodel Repository: `GET /submodels/{submodelIdentifier}/$history`
- Submodel Repository: `GET /submodels/{submodelIdentifier}/$signed`
- Concept Description Repository: `GET /concept-descriptions/$recent-changes`
- AAS Registry and Digital Twin Registry: `GET /shell-descriptors/$recent-changes`
- AAS Environment: exposes the corresponding AAS, Submodel, Concept Description, and descriptor endpoints through the composed service.

Additional compatibility endpoint:

- Submodel Repository and AAS Environment: `GET /submodels/{submodelIdentifier}/$value/$signed`

Known v3.2 OpenAPI gaps in standalone services:

- Standalone AAS Repository `/serialization` is present in generated code but is not wired by the service entrypoint.
- Standalone Concept Description Repository `/serialization` is present in the OpenAPI file but is not currently exposed by a generated controller/service.
- Standalone Submodel Repository `/serialization` is routed but returns `501 Not Implemented`.
- AAS Environment `/serialization` and `/upload` are implemented and should be used when full environment import/export is needed.

## History Configuration

History behavior is controlled through lightweight, vendor-neutral configuration.

Environment variables:

- `BASYX_HISTORY_MODE`: `off`, `api`, or `audit`. Default is `api`.
- `BASYX_HISTORY_RETENTION_DAYS`: number of days to retain history. `0` means keep forever. Default is `0`.
- `BASYX_HISTORY_IMMUTABILITY`: `none`, `postgres_guarded`, or `external_anchor`. Default is `none`.
- `BASYX_AUDIT_IDENTITY_MODE`: `none`, `minimal`, or `extended`. Default is `minimal`.

Mode semantics:

- `off`: history writes are skipped. History/recent-change reads will not receive new rows while this mode is active.
- `api`: functional AAS v3.2 history and recent-change behavior for API consumers.
- `audit`: append-only hash-chained history rows for compliance-oriented deployments.

Current implementation note:

- Runtime history rows are append-only event rows in `api` and `audit` mode.
- `postgres_guarded` installs PostgreSQL triggers during schema migration and enables them at service startup. When enabled, `UPDATE`, `DELETE`, and `TRUNCATE` on history tables fail with `history tables are append-only`.
- `external_anchor` currently enables the same PostgreSQL guard foundation and reserves anchor metadata columns. It does not yet publish anchors to an external system.
- The implementation supports compliance-oriented deployments, but it does not by itself make a deployment legally compliant with any specific regulation.

Guarded PostgreSQL mode protects against normal accidental or unauthorized mutations through the application database user. PostgreSQL superusers or operators with permissions to alter triggers/functions can still bypass or remove this protection. Stronger guarantees require external anchoring, WORM storage, or a transparency-log style system.

The guard switch is database-wide. Configure all BaSyx services that share one database with the same history immutability mode so one service does not intentionally disable a guard expected by another service.

See `examples/BaSyxHistoryAuditGuardedExample` for a Docker Compose setup with audit history and `postgres_guarded` enabled.

Eventing placeholders:

- `BASYX_EVENTING_ENABLED`
- `BASYX_EVENTING_FORMAT`, currently expected to be `cloudevents`
- `BASYX_EVENTING_SINKS`
- `BASYX_EVENTING_OUTBOX_ENABLED`
- `BASYX_EVENTING_TOPIC_PREFIX`

These settings reserve the configuration shape for future CloudEvents-compatible outbox/event publishing. MQTT and Kafka publishing are not implemented yet.

## Historical Reads

Normal `GET` endpoints return the latest current version. Use `$history` when you need the entity that was valid at a specific point in time.

Example:

```sh
curl \
  'http://localhost:6004/shells/{base64urlAasIdentifier}/$history?date=2026-05-28T10:15:30Z'
```

```sh
curl \
  'http://localhost:6004/submodels/{base64urlSubmodelIdentifier}/$history?date=2026-05-28T10:15:30Z'
```

Behavior:

- The identifier path parameter is encoded the same way as other IDTA identifier path parameters.
- The `date` query parameter selects the version that was valid at that time.
- A date before deletion can still resolve the historical version.
- A date after deletion returns not found.
- If the requested date is exactly on an update boundary, the newer version is returned.

Submodel element changes are tracked as part of the owning Submodel. If a Submodel Element is added, changed, deleted, or has an attachment update, the next Submodel history entry contains a full Submodel snapshot.

## Recent Changes

Recent-change endpoints return append-only change metadata with cursor-based pagination.

Example:

```sh
curl 'http://localhost:6004/shells/$recent-changes?limit=50'
```

```sh
curl 'http://localhost:6004/submodels/$recent-changes?limit=50&updatedFrom=2026-05-28T00:00:00Z'
```

Common query parameters:

- `limit`: maximum number of changes to return.
- `cursor`: pagination cursor from the previous response.
- `createdFrom`: lower bound for administrative creation timestamps.
- `updatedFrom`: lower bound for administrative update timestamps.

Additional filters:

- AAS recent changes support asset-id filtering according to the AAS Repository profile.
- Submodel recent changes support semantic-id filtering according to the Submodel Repository profile.

Delete events are returned as tombstones where supported. Tombstones expose the identifier and change metadata, but not the deleted entity snapshot.

## Signed Reads

Signed endpoints return a compact JWS string for the requested AAS or Submodel.

Example:

```sh
curl 'http://localhost:6004/shells/{base64urlAasIdentifier}/$signed'
```

```sh
curl 'http://localhost:6004/submodels/{base64urlSubmodelIdentifier}/$signed'
```

If signing is not configured, the endpoint returns an error instead of an unsigned payload. Signed reads use the same read authorization rules as the corresponding normal read endpoints.

## AssetKind Batch

AAS API v3.2 adds `Batch` to `AssetKind`. Existing database values are migrated so older persisted enum indices keep their intended meaning after `Batch` is inserted.

For users this means:

- New payloads may use `Batch`.
- Existing data with older asset-kind values is adjusted during the schema migration.
- Integration tests cover both accepting `Batch` in new payloads and migrating old numeric values.

## Security

The new history, recent-change, and signed endpoints are protected as read endpoints by the same ABAC middleware used by the rest of the service.

Important operational note:

- Historical snapshots are stored independently from the current normalized tables. If your deployment relies on very fine-grained field-level redaction, validate whether historical reads need additional policy review before exposing them broadly.

## Operational Considerations

History support increases database growth because each write creates a history row.

Growth is reduced by:

- Keeping current tables separate from history tables.
- Using indexed metadata for recent-change queries.
- Storing delete events as tombstones where supported.
- Using cursor pagination for recent-change feeds.
- Backfilling one baseline row per existing entity during migration, not one row per nested child object.
- Hashing canonical JSON snapshots instead of signing or anchoring every row by default.

For large installations, plan retention and monitoring:

- Monitor history table row counts and table sizes.
- Decide how long historical versions must be retained.
- Consider partitioning or compaction for high-write deployments.
- Pay special attention to large Submodels with frequent Submodel Element changes.
