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

History behavior is controlled through lightweight, vendor-neutral configuration. Versioning is opt-in: `history.mode` defaults to `off`.

Environment variables:

- `BASYX_HISTORY_MODE`: `off`, `api`, or `audit`. Default is `off`.
- `BASYX_HISTORY_RETENTION_DAYS`: must remain `0`. Automatic cleanup is not implemented yet.
- `BASYX_HISTORY_IMMUTABILITY`: `none` or `postgres_guarded`. Default is `none`.
- `BASYX_AUDIT_IDENTITY_MODE`: must remain `none`. Automatic identity enrichment is not implemented yet.

Equivalent YAML:

```yaml
history:
  mode: off
  retentionDays: 0
  immutability: none
  auditIdentityMode: none
```

Mode semantics:

- `off`: new history writes are skipped. Existing history rows remain readable.
- `api`: functional AAS v3.2 history and recent-change behavior for API consumers.
- `audit`: the same runtime snapshot writes as `api`, intended for audit-oriented deployments where guarded storage is configured explicitly.

Current implementation status:

- Runtime history rows are append-only, hash-chained event rows in `api` and `audit` mode.
- Schema migration installs PostgreSQL guard triggers. `postgres_guarded` enables them at service startup. When enabled, `UPDATE`, `DELETE`, and `TRUNCATE` on history metadata and payload tables fail with `history tables are append-only`.
- `external_anchor`, non-zero `retentionDays`, and identity enrichment modes currently fail fast during configuration loading. Their runtime implementations are planned for later work.
- Audit metadata columns are present, but regular API middleware does not populate the audit context yet.
- The implementation supports compliance-oriented deployments, but it does not by itself make a deployment legally compliant with any specific regulation.

Guarded PostgreSQL mode protects against normal accidental or unauthorized mutations through the application database user. PostgreSQL superusers or operators with permissions to alter triggers/functions can still bypass or remove this protection. Stronger guarantees require external anchoring, WORM storage, or a transparency-log style system.

The guard switch is database-wide. Configure all BaSyx services that share one database with the same history immutability mode. Runtime services may enable guarded mode, but normal service startup cannot disable an enabled database guard. A service configured as unguarded fails during startup when it encounters an already-enabled database guard. Disabling guarded mode is an explicit operator maintenance action.

See `examples/BaSyxHistoryAuditGuardedExample` for a Docker Compose setup with audit history and `postgres_guarded` enabled.

## What Activating Versioning Means

When versioning is active, each supported identifiable create, update, or delete appends a new row to a dedicated history table. The row stores a complete identifiable snapshot, not only the changed field. A small nested change can therefore create a history row close to the size of the owning identifiable.

```mermaid
flowchart LR
    Client["API write"] --> Current["Update current PostgreSQL tables"]
    Current --> Mode{"history.mode"}
    Mode -->|off| Done["No history append"]
    Mode -->|api or audit| Snapshot["Build complete identifiable snapshot"]
    Snapshot --> History["INSERT metadata and *_history_payload rows"]
    History --> Read["$history and $recent-changes reads"]
```

The owning identifiable depends on the write:

```mermaid
flowchart TD
    AAS["AAS write or nested AAS update"] --> AASSnapshot["AAS snapshot"]
    SME["Submodel write, SME write, or attachment change"] --> SMSnapshot["Submodel snapshot"]
    CD["Concept Description write"] --> CDSnapshot["Concept Description snapshot"]
    Descriptor["AAS descriptor or nested Submodel descriptor write"] --> DescriptorSnapshot["AAS descriptor snapshot"]
```

This has operational consequences:

- History consumes additional storage for every write and adds hashing plus one metadata insert and one payload insert to the write request.
- Writes for the same identifiable are serialized briefly while the next hash-chain row is appended. Different identifiers can still proceed independently.
- Partial updates usually derive the new snapshot from the previous history snapshot. This reduces reads from the normalized backend, but the stored history row is still a complete snapshot.
- Snapshot JSON is stored in a one-to-one payload table so indexed history metadata rows stay narrow.
- Schema migration does not backfill existing entities. Historical state from before activation is unavailable.
- If an existing entity has no history row yet, its first partial update falls back to materializing the current complete identifiable once. Later partial updates can derive snapshots from history.
- While history is active, an unclassified write endpoint is rejected before its handler runs with `HISTORY-COVERAGE-UNCLASSIFIED`. This prevents a newly added endpoint from silently changing current state without appending its required version.

Eventing placeholders:

- `BASYX_EVENTING_ENABLED`
- `BASYX_EVENTING_FORMAT`, currently expected to be `cloudevents`
- `BASYX_EVENTING_SINKS`
- `BASYX_EVENTING_OUTBOX_ENABLED`
- `BASYX_EVENTING_TOPIC_PREFIX`

These settings reserve the configuration shape for future CloudEvents-compatible outbox/event publishing. MQTT and Kafka publishing are not implemented yet. Enabling eventing, configuring sinks, or enabling the outbox currently fails fast during configuration loading.

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

## Submodel Element History FAQ

Submodel Elements do not have independent history streams. They are part of their owning Submodel. An SME write therefore appends an `Updated` event for the Submodel, even when the SME itself was newly created or deleted.

| SME operation | History effect |
| --- | --- |
| Create a top-level SME with `POST /submodels/{submodelIdentifier}/submodel-elements` | Append an `Updated` Submodel snapshot containing the new SME. |
| Create a nested SME with `POST /submodels/{submodelIdentifier}/submodel-elements/{idShortPath}` | Append an `Updated` Submodel snapshot. The path identifies the existing parent below which the new SME is added. |
| Create or replace an SME with `PUT /submodels/{submodelIdentifier}/submodel-elements/{idShortPath}` | Append an `Updated` Submodel snapshot. The path identifies the target SME. |
| Patch an SME, its metadata, or its value | Append an `Updated` Submodel snapshot containing the changed SME state. |
| Delete an SME with `DELETE /submodels/{submodelIdentifier}/submodel-elements/{idShortPath}` | Append an `Updated` Submodel snapshot without the deleted SME. Deleting a container also removes its nested children from the new snapshot. |
| Upload or delete a file attachment | Append an `Updated` Submodel snapshot containing the changed File SME metadata. |

**Why is an SME create or delete reported as `Updated`?**

The event describes the owning identifiable. An SME is nested content of a Submodel, so creating, changing, or deleting an SME updates that Submodel.

**What happens for nested `idShortPath` values?**

For nested paths such as `Measurements.temperature`, history still stores the complete Submodel snapshot. Internally, the implementation refreshes the affected top-level SME subtree, `Measurements` in this example, and combines it with the previous Submodel snapshot. This avoids reading the entire current Submodel after every nested change.

**What happens if the Submodel has no earlier snapshot?**

The first partial mutation falls back to reading the complete current Submodel once. Later partial mutations can derive the new snapshot from history.

**Does deleting an SME container create a separate history entry for every nested SME?**

No. One SME request appends one `Updated` snapshot for the owning Submodel. The new snapshot no longer contains the deleted container or its descendants.

**Do AAS superpath routes also create an AAS history entry?**

The AAS Repository and AAS Environment also expose SME routes below the AAS superpath:

```text
/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/...
```

These routes delegate to the same Submodel behavior. An SME-only change appends an `Updated` entry to `submodel_history`; it does not append an AAS history row because the AAS-to-Submodel reference did not change.

## Recent Changes

Recent-change endpoints return append-only, cursor-paged results from the history tables. Their result shape depends on the component:

| Component | Result fields |
| --- | --- |
| AAS Repository | `type`, `createdAt`, `updatedAt`, `id`, and optional `globalAssetId` and `specificAssetIds` |
| Submodel Repository | `type`, `createdAt`, `updatedAt`, `id`, and optional `semanticId` and `supplementalSemanticIds` |
| Concept Description Repository | `type`, `createdAt`, `updatedAt`, and `id` |
| AAS Registry and DTR | Complete AAS descriptor snapshots |

Example:

```sh
curl 'http://localhost:6004/shells/$recent-changes?limit=50'
```

```sh
curl 'http://localhost:6004/submodels/$recent-changes?limit=50&updatedFrom=2026-05-28T00:00:00Z'
```

Common query parameters:

- `limit`: maximum number of changes to return. The default is `100`; the maximum accepted value is `1000`.
- `cursor`: pagination cursor from the previous response.
- `createdFrom`: lower bound for administrative creation timestamps.
- `updatedFrom`: lower bound for administrative update timestamps.

Additional filters:

- AAS recent changes support `assetIds`. Each value is a base64url-encoded `SpecificAssetId`.
- Submodel recent changes support `semanticId`. Its semantic-reference value is base64url encoded.
- AAS descriptor recent changes support `assetKind`, base64url-encoded UTF-8 `assetType`, and base64url-encoded `assetIds`.

For AAS, Submodels, and Concept Descriptions, deleted identifiables remain visible as tombstones with `id`, `type`, `createdAt`, and `updatedAt`. Optional resource metadata is absent from tombstones. Descriptor recent-change endpoints omit deleted descriptor rows because their response type requires complete descriptors.

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

- `$history` and `$recent-changes` are route-authorized resources in this release. They do not apply current-table ABAC filters or logical-expression redaction to stored snapshots.
- Only assign access to these routes to principals that may read the complete historical snapshot or complete recent-change feed returned by the endpoint.
- Identifier-aware authorization for these endpoints is planned as a separate follow-up. Fine-grained snapshot filtering is intentionally out of scope.

## Operational Considerations

History support increases database growth because each write creates a full identifiable snapshot row.

Runtime overhead is reduced by:

- Keeping current tables separate from history tables.
- Using indexed metadata for recent-change queries.
- Keeping complete JSONB snapshots in one-to-one payload tables outside indexed history metadata rows.
- Using cursor pagination for recent-change feeds.
- Deriving partial-update snapshots from the latest history row when possible.
- Reading only the affected top-level Submodel Element subtree for nested SME changes.
- Storing compact delete tombstones where supported.
- Hashing canonical JSON snapshots instead of signing or anchoring every row by default.

For large installations, plan retention and monitoring:

- Monitor history table row counts and table sizes.
- Decide how long historical versions must be retained and implement an operator-controlled cleanup process if needed.
- Consider partitioning or compaction for high-write deployments.
- Pay special attention to large Submodels with frequent Submodel Element changes.
- Keep the PostgreSQL guard implications in mind: guarded mode intentionally blocks direct `UPDATE`, `DELETE`, and `TRUNCATE` maintenance on history tables until the guard is disabled.
