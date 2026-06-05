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

# AAS API v3.2 Runtime Notes

This document describes the runtime behavior added for AAS API v3.2 support. It focuses on the implementation choices that are easy to miss when reading only the OpenAPI files.

## Scope

The v3.2 OpenAPI update adds history and recent-change endpoints to repository and registry components, introduces the `Batch` value for `AssetKind`, extends administrative timestamps, and exposes composed endpoints through the AAS environment.

Implemented history, recent-change, and signing runtime areas from the v3.2 OpenAPI files:

- AAS Repository: `/shells/$recent-changes`, `/shells/{aasIdentifier}/$history`, `/shells/{aasIdentifier}/$signed`.
- Submodel Repository: `/submodels/$recent-changes`, `/submodels/{submodelIdentifier}/$history`, `/submodels/{submodelIdentifier}/$signed`.
- Submodel Repository compatibility route: `/submodels/{submodelIdentifier}/$value/$signed` is exposed by the generated Go router and existing integration coverage, although it is not listed in the current local v3.2 OpenAPI file.
- Concept Description Repository: `/concept-descriptions/$recent-changes`.
- AAS Registry and Digital Twin Registry: `/shell-descriptors/$recent-changes`.
- AAS Environment: `/serialization`, `/upload`, `/shell-descriptors/$recent-changes`, `/shells/$recent-changes`, `/shells/{aasIdentifier}/$history`, `/shells/{aasIdentifier}/$signed`, `/submodels/$recent-changes`, `/submodels/{submodelIdentifier}/$history`, `/submodels/{submodelIdentifier}/$signed`, `/submodels/{submodelIdentifier}/$value/$signed`, `/concept-descriptions/$recent-changes`, and the composed asynchronous operation result/status endpoints.
- Migration `1_1_0.sql`: adds v3.2 timestamp columns and the enum migration for `Batch`.
- Migration `1_1_1.sql`: adds history metadata and payload tables, indexes, and PostgreSQL mutation guards.
- Migration `1_1_2.sql`: adds snapshot-checkpoint indexes for diff-backed restore.
- Migration `1_1_3.sql`: adds WORM evidence manifest and artifact receipt catalogs.

The Submodel Registry does not have a recent-changes endpoint in the official v3.2 profile currently used here.

The current v3.2 Submodel Repository OpenAPI also defines `PUT`, `PATCH`, and `DELETE` on `/submodels/{submodelIdentifier}/$signed`. These operations use the normal Submodel request bodies and are routed to the same runtime behavior as `PUT`, `PATCH`, and `DELETE` on `/submodels/{submodelIdentifier}`.

OpenAPI endpoints checked outside the history/recent/signing scope:

- AAS Repository, Submodel Repository, Concept Description Repository, and AAS Environment OpenAPI files contain `/serialization`.
- The AAS Environment `/serialization` and `/upload` endpoints are custom implemented and covered by integration tests.
- The Submodel Repository `/serialization` route is wired and currently returns `501 Not Implemented`.
- The standalone AAS Repository generated serialization handler is present in `pkg/aasrepositoryapi`, but it is not wired by `cmd/aasrepositoryservice`.
- The standalone Concept Description Repository OpenAPI contains `/serialization`, but the current generated Go package only contains the interface, not a registered controller/service implementation.
- AAS Repository, Submodel Repository, and AAS Environment OpenAPI files contain asynchronous operation result/status endpoints. These are separate from the new history/recent-change storage described below.

## History Model

History metadata is stored in dedicated append-only tables:

- `aas_history`
- `submodel_history`
- `concept_description_history`
- `descriptor_history`

The complete JSON snapshot is stored in a one-to-one payload table:

- `aas_history_payload`
- `submodel_history_payload`
- `concept_description_history_payload`
- `descriptor_history_payload`

Each metadata row stores:

- `identifier`
- `change_type`: `Created`, `Updated`, or `Deleted`
- `deleted`
- `valid_from`
- `valid_to`: reserved for interval-style history, but not populated or used by the current runtime history resolution
- `operation_time`
- administrative timestamp text values plus typed `TIMESTAMPTZ` columns for `createdFrom` and `updatedFrom` filters
- audit metadata columns such as `actor_subject`, `request_id`, `endpoint`, and `http_method`
- payload metadata: `payload_type` and `payload_hash`
- tamper-evidence columns: `previous_hash`, `content_hash`, and `row_hash`

On every create, update, or delete, a new immutable metadata row and one payload row are appended. Existing history rows are not updated by the runtime. Both rows are inserted in the same database transaction as the current-table mutation, including value-only SME updates.

Keeping JSONB outside the indexed metadata row narrows recent-change and latest-hash access paths. With `history.fullSnapshotInterval: 1`, every payload row stores `snapshot`. With larger intervals, the runtime stores one `snapshot` checkpoint followed by up to `N-1` RFC 6902 `diff` payload rows, and it may checkpoint earlier when the diff JSON is not smaller than the full snapshot payload.

History lookup uses:

```text
latest event where valid_from <= requested_date
ORDER BY valid_from DESC, history_id DESC
```

If the latest matching event is marked as deleted, the history endpoint returns not found. This means a deleted entity can still be resolved for dates before deletion, but not after deletion.

Each runtime-created row stores a deterministic SHA-256 hash of the reconstructed canonical JSON snapshot (`content_hash`), a separate hash of the stored snapshot or diff payload (`payload_hash`), and a per-identifier chain hash (`row_hash`) that includes the previous row hash and selected audit metadata.

### Shared Append Algorithm

The shared implementation lives in `internal/common/history`.

- `AppendVersionTx` receives a complete snapshot supplied by the persistence layer and stores it as either a snapshot checkpoint or a diff payload according to `history.fullSnapshotInterval` and payload size.
- `AppendMutatedVersionTx` loads the latest reconstructed snapshot for the identifier, applies a scoped mutation, and stores the resulting version with the same interval and payload-size logic.
- Both functions acquire a transaction-level PostgreSQL advisory lock derived from `<history-table>:<identifier>`.
- The lock serializes hash-chain appends for the same identifiable while allowing unrelated identifiers to proceed independently.
- Both functions append with `INSERT`; they never modify an existing history row.

```mermaid
sequenceDiagram
    participant API as Persistence write path
    participant Live as Current normalized tables
    participant Metadata as *_history table
    participant Payload as *_history_payload table
    participant Evidence as WORM EvidenceStore
    API->>Live: Apply typical current-state mutation in transaction
    API->>Metadata: Advisory lock(table, identifier)
    Metadata-->>API: Latest history row
    Payload-->>API: Restore nearest snapshot checkpoint plus diffs when required
    API->>API: Apply scoped snapshot mutation
    API->>Metadata: INSERT event metadata with previous_hash
    API->>Payload: INSERT snapshot or RFC 6902 diff payload
    opt history.evidence.enabled
        API->>Evidence: PUT history-event artifact before commit
        Evidence-->>API: Evidence receipt
        API->>Metadata: INSERT evidence artifact receipt
    end
    API->>Live: Commit transaction
```

This reduces reads against the normalized backend for partial updates and bounds restore work to the configured interval.

`history.fullSnapshotInterval: 1` preserves the full-snapshot behavior. Values greater than `1` allow at most `N-1` diff rows after each full checkpoint. A full snapshot can appear earlier when the diff payload would be equal to or larger than the snapshot payload.

### Per-Identifiable Write Paths

| Identifiable | Complete write path | Optimized partial write path | Missing-history fallback |
| --- | --- | --- | --- |
| AAS | Create and full replace append a complete AAS snapshot. Delete appends an `{id}` tombstone. | Submodel-reference add/remove, asset-information updates, and thumbnail changes mutate the previous AAS snapshot. Thumbnail upload reads only the stored thumbnail metadata needed for the snapshot. | Materialize the complete current AAS once. |
| Submodel | Create, full replace, and full patch append a complete Submodel snapshot. Delete appends an `{id}` tombstone. | Metadata updates replace metadata while preserving `submodelElements`. SME create/update/patch/delete, value-only changes, and attachment changes reload only the affected top-level SME root subtree and splice it into the previous snapshot. | Materialize the complete current Submodel once. |
| Concept Description | Create and replace append the supplied complete Concept Description snapshot. Delete appends an `{id}` tombstone. | No nested partial write path is required. | Not applicable. |
| AAS descriptor | Create and full replace append the stored complete AAS descriptor. Delete appends the complete descriptor marked as deleted. | Nested Submodel descriptor add/replace/remove mutates the owning AAS descriptor snapshot. | Materialize the complete current AAS descriptor once. |

Submodel elements and nested Submodel descriptors are not versioned independently. A child mutation creates a new snapshot for its owning identifiable. For SMEs, reloading the top-level subtree after the current-state mutation covers nested edits, renamed `idShort` values, and list-position changes without re-reading the entire Submodel.

### Submodel Element Lifecycle

Every SME mutation appends `Updated` to `submodel_history`. The history event type describes the owning identifiable, not the nested SME action.

| SME write | Path meaning | Snapshot mutation |
| --- | --- | --- |
| `POST /submodels/{sm}/submodel-elements` | Add a new top-level SME. | Append the new root SME to `submodelElements`. |
| `POST /submodels/{sm}/submodel-elements/{idShortPath}` | Add a new direct child below the existing SME container at `idShortPath`. | Reload and replace the affected top-level root subtree. |
| `PUT /submodels/{sm}/submodel-elements/{idShortPath}` when missing | Create the SME at the target path. Creating by list-index path is rejected. | Append a new top-level root or reload the parent root subtree. |
| `PUT /submodels/{sm}/submodel-elements/{idShortPath}` when present | Replace the target SME. | Reload and replace the affected top-level root subtree. |
| `PATCH /submodels/{sm}/submodel-elements/{idShortPath}` | Merge and update the target SME. | Reload and replace the affected top-level root subtree. |
| `PATCH /submodels/{sm}/submodel-elements/{idShortPath}/$metadata` | Update SME metadata. | Reload and replace the affected top-level root subtree. |
| `PATCH /submodels/{sm}/submodel-elements/{idShortPath}/$value` | Update the value-only representation. | Reload and replace the affected top-level root subtree after the value write. |
| `DELETE /submodels/{sm}/submodel-elements/{idShortPath}` | Delete the target SME and any nested children. | Remove the root when deleting a top-level SME; otherwise reload and replace the surviving root subtree. |
| `PUT` or `DELETE /submodels/{sm}/submodel-elements/{idShortPath}/attachment` | Change File SME attachment content. | Reload and replace the affected top-level root subtree. |

For `Measurements.temperature`, the root path is `Measurements`. For a nested update, the current `Measurements` subtree is read with deep content after the normalized mutation and replaces the old root in the previous snapshot. If a top-level `idShort` changes, the previous path locates the old root and the resolved current path loads the renamed root.

If the Submodel has no prior history row, the optimized mutation path cannot splice into a previous snapshot. It falls back to a one-time complete Submodel materialization and appends that result.

### Endpoint History Matrix

The table lists direct write endpoints. The AAS Environment exposes the corresponding component routes with the same history effects.

| Endpoint family | Verb | Owning history table | Event type |
| --- | --- | --- | --- |
| `/shells` | `POST` | `aas_history` | `Created` |
| `/shells/{aasIdentifier}` | `PUT` | `aas_history` | `Created` or `Updated` |
| `/shells/{aasIdentifier}` | `DELETE` | `aas_history` | `Deleted` |
| `/shells/{aasIdentifier}/asset-information` | `PUT` | `aas_history` | `Updated` |
| `/shells/{aasIdentifier}/asset-information/thumbnail` | `PUT`, `DELETE` | `aas_history` | `Updated` |
| `/shells/{aasIdentifier}/submodel-refs` | `POST` | `aas_history` | `Updated` |
| `/shells/{aasIdentifier}/submodel-refs/{submodelIdentifier}` | `DELETE` | `aas_history` | `Updated` |
| `/submodels` | `POST` | `submodel_history` | `Created` |
| `/submodels/{submodelIdentifier}` | `PUT` | `submodel_history` | `Created` or `Updated` |
| `/submodels/{submodelIdentifier}`, `/submodels/{submodelIdentifier}/$metadata`, `/submodels/{submodelIdentifier}/$value` | `PATCH` | `submodel_history` | `Updated` |
| `/submodels/{submodelIdentifier}` | `DELETE` | `submodel_history` | `Deleted` |
| `/submodels/{submodelIdentifier}/submodel-elements...` SME write routes listed above | `POST`, `PUT`, `PATCH`, `DELETE` | `submodel_history` | `Updated` |
| `/concept-descriptions` | `POST` | `concept_description_history` | `Created` |
| `/concept-descriptions/{cdIdentifier}` | `PUT` | `concept_description_history` | `Created` or `Updated` |
| `/concept-descriptions/{cdIdentifier}` | `DELETE` | `concept_description_history` | `Deleted` |
| `/shell-descriptors` | `POST` | `descriptor_history` | `Created` |
| `/shell-descriptors/{aasIdentifier}` | `PUT` | `descriptor_history` | `Created` or `Updated` |
| `/shell-descriptors/{aasIdentifier}` | `DELETE` | `descriptor_history` | `Deleted` |
| `/shell-descriptors/{aasIdentifier}/submodel-descriptors` | `POST` | `descriptor_history` | `Updated` |
| `/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}` | `PUT`, `DELETE` | `descriptor_history` | `Updated` |
| `/bulk/shell-descriptors` | `POST`, `PUT`, `DELETE` | `descriptor_history` | One corresponding event per descriptor after asynchronous processing succeeds |

The environment import portion of AAS Environment `/upload` invokes the corresponding identifiable `PUT` paths. One upload can therefore append multiple rows across the Concept Description, Submodel, and AAS streams rather than one special upload event.

Read endpoints and operation invocation endpoints do not append history rows.

### Superpath Effects

The AAS Repository and AAS Environment expose Submodel operations below:

```text
/shells/{aasIdentifier}/submodels/{submodelIdentifier}
```

These superpath routes reuse the Submodel persistence layer. They can affect more than one identifiable when the relationship itself changes:

| Superpath write | History effect |
| --- | --- |
| `PUT /shells/{aas}/submodels/{sm}` | Append `Created` or `Updated` to `submodel_history`. If a new AAS-to-Submodel reference is added, also append `Updated` to `aas_history`. |
| `DELETE /shells/{aas}/submodels/{sm}` | Append `Deleted` to `submodel_history` and `Updated` to `aas_history` because the reference is removed. |
| `PATCH /shells/{aas}/submodels/{sm}` and representation variants | Append `Updated` to `submodel_history`. The AAS reference itself is unchanged. |
| `/shells/{aas}/submodels/{sm}/submodel-elements...` SME write routes | Append `Updated` to `submodel_history` only. The AAS reference itself is unchanged. |

Registry synchronization can append additional descriptor history entries when configured. For example, adding, replacing, or removing a nested Submodel descriptor appends `Updated` to the owning AAS descriptor stream.

### Guarded PostgreSQL Mode

Schema patch `1_1_1.sql` installs guard triggers on all four history metadata tables and all four payload tables. The triggers are disabled by default through the singleton `history_guard_config` row. Each history-aware DB-backed runtime service applies its expected guard state at startup.

```mermaid
flowchart TD
    Insert["INSERT metadata or payload row"] --> Allowed["Allowed"]
    Mutate["UPDATE or DELETE metadata or payload row"] --> Guard{"history_guard_config.enabled"}
    Truncate["TRUNCATE metadata or payload table"] --> Guard
    Guard -->|false| Allowed
    Guard -->|true| Reject["Reject: history tables are append-only"]
```

The guard is enabled when history is active and `history.immutability` is `postgres_guarded`. It blocks direct maintenance mutations as well as accidental application mutations. Enabling is monotonic during normal service startup: a runtime service can enable the database-wide guard, but it cannot disable an enabled guard. A service configured as unguarded fails fast when it encounters an already-enabled database guard. Services sharing one database can start concurrently when their configuration is consistent. Disabling guarded mode requires an explicit operator maintenance action. The guard is not equivalent to WORM storage: sufficiently privileged PostgreSQL operators can alter schema objects.

### WORM Evidence Manifests

The default stronger-integrity architecture is:

```text
PostgreSQL history tables -> hash chain -> synchronous history-event artifact -> signed manifest -> S3-compatible WORM object storage
```

The HTTP APIs are unchanged. When `history.evidence.enabled` is active, `history.mode` must be `api` or `audit`. The history append path writes one WORM `history_event` artifact synchronously before the surrounding PostgreSQL transaction can commit. The artifact stores the same history payload selected for PostgreSQL: either a full snapshot or an RFC 6902 diff according to `history.fullSnapshotInterval`. It also stores `effective_diff`, an RFC 6902 JSON Patch from the previous reconstructed version to the current version. If the WORM write fails, the history append returns an error and the caller rolls back the PostgreSQL transaction.

The `cmd/historyevidenceverifier` tool can additionally publish signed range manifests, backfill per-row `history_event` artifacts for existing rows, and publish checkpoint artifacts using the shared `EvidenceStore` interface. The current implementation includes an S3-compatible `EvidenceStore`; MinIO can be used for local or CI-style object-lock testing, while production deployments should use an S3-compatible WORM service with versioning/object lock configured by operations.

For a selected history range, evidence publication:

- verifies PostgreSQL payload hashes and per-identifier row-hash chains first;
- writes canonical `history_event` artifacts for every snapshot and diff row in the range;
- writes full snapshot checkpoint artifacts for every `payload_type = snapshot` row in the range;
- builds a canonical `HistoryManifest` containing first/last `history_id`, first/last `row_hash`, row count, ordered range digest, timestamp, signature metadata, and snapshot artifact references;
- signs the canonical manifest as compact RS256 JWS when an evidence signing key is configured, otherwise stores canonical JSON with `signature_state = unsigned`;
- writes object-store receipts to `history_evidence_manifests` and `history_evidence_artifacts`.

Per-row `history_event` artifacts provide recovery evidence for every acknowledged write while evidence is enabled. With `history.fullSnapshotInterval: 5`, recovery from WORM replays up to four WORM-stored diff payloads after the latest WORM-stored snapshot event. Use `history.fullSnapshotInterval: 1` when each individual WORM event must be recoverable without replaying diffs. The `effective_diff` field is the attribution trail: for snapshot checkpoint rows it prevents a full recovery snapshot from being mistaken for the set of fields changed by the actor. Automated PostgreSQL restore from WORM artifacts is not implemented in this ticket.

Verification can compare PostgreSQL rows against the hash chain, verify every per-row `history_event` receipt and object hash, compare a stored manifest against the live range digest, and verify an object-store artifact hash. This detects missing, modified, reordered, or overwritten records when the expected receipts and object hashes are known.

### Configuration Status

| Setting | Current runtime behavior |
| --- | --- |
| `history.mode: off` | Skip new snapshot writes. Existing rows remain readable. This is the default. |
| `history.mode: api` | Append history rows. |
| `history.mode: audit` | Append the same runtime history rows as `api`; intended for audit-oriented deployments with explicit storage controls. |
| `history.retentionDays` | Must remain `0`. Non-zero values fail fast until cleanup is implemented. |
| `history.fullSnapshotInterval` | `1` stores all payloads as snapshots. Values greater than `1` store one full checkpoint plus up to `N-1` diff rows, with earlier checkpoints when the diff payload is not smaller than the snapshot payload. |
| `history.immutability: none` | Keep PostgreSQL mutation guards disabled. |
| `history.immutability: postgres_guarded` | Enable PostgreSQL mutation guards at service startup. |
| `history.immutability: external_anchor` | Reserved for a future `IntegrityAnchor` backend and still fails fast unless a real provider is implemented. |
| `history.evidence.enabled` | Enables fail-closed WORM history-event artifact writes. It does not change HTTP response shapes, but mutating requests fail if the evidence artifact cannot be stored. |
| `history.evidence.provider: s3` | Configures the S3-compatible `EvidenceStore`. Requires bucket, region, retention mode, and positive retention days. Endpoint override and path-style mode support MinIO-style tests. |
| `history.evidence.writeTimeoutSeconds` | Bounds synchronous WORM writes while the PostgreSQL transaction is open. Default is `10`. |
| `history.evidence.signing.privateKeyPath` | Optional RS256 manifest signing key. Falls back to `jws.privateKeyPath` when empty. |
| `history.integrityAnchor.provider: none` | Default. Non-`none` providers such as immudb, Rekor, Trillian, or timestamping services are reserved for later work. |
| `history.auditIdentityMode` | Must remain `none`. Identity enrichment modes fail fast until middleware populates audit context. |
| Active eventing, configured event sinks, or enabled outbox processing | Fail fast until outbox publishing is implemented. |

`AuditContext`, `ChangeEvent`, `EvidenceStore`, and `IntegrityAnchor` remain extension points. Current runtime middleware does not populate `AuditContext`, and no external ledger anchor client is invoked by the append path yet.

Example verifier/publisher usage:

```sh
go run ./cmd/historyevidenceverifier \
  -config ./config.yaml \
  -table submodel_history \
  -identifier 'https://example.com/submodels/1' \
  -from 1 \
  -to 25 \
  -write
```

```sh
go run ./cmd/historyevidenceverifier \
  -config ./config.yaml \
  -table submodel_history \
  -identifier 'https://example.com/submodels/1' \
  -from 1 \
  -to 25 \
  -manifest-object-key 'history-evidence/history-manifests/submodel_history/https:%2F%2Fexample.com%2Fsubmodels%2F1/1-25-...json' \
  -manifest-sha256 '<expected-sha256>'
```

### Diff-Backed Storage

There is intentionally no separate `history.storageMode` setting. Full-snapshot history is represented by `history.fullSnapshotInterval: 1`; compact storage is enabled by values greater than `1`.

Diff-backed rows use the existing `payload_type`, `payload_hash`, and nullable `diff` payload column:

- Diff payloads are deterministic RFC 6902 JSON Patch operation arrays.
- `content_hash` is the reconstructed full-snapshot hash.
- `payload_hash` is the stored snapshot or diff payload hash.
- Restore walks back to the nearest full checkpoint and applies diffs in order, so worst-case work is bounded by the configured interval.
- Existing snapshot-only history remains readable without backfill.

### Fail-Closed Mutation Coverage

History-aware HTTP services install a shared mutation-coverage middleware. Every `POST`, `PUT`, `PATCH`, or `DELETE` request must match an explicitly classified route whenever history is active:

- Versioned routes are allowed and carry a `MutationCoverage` context value with `Versioned: true`.
- Deliberately non-versioned writes, such as query, invocation, discovery-link, and standalone Submodel Registry routes, are explicit exemptions with `Versioned: false`.
- An unclassified mutation is rejected before its handler runs with `HISTORY-COVERAGE-UNCLASSIFIED`.

Generated component routes are classified centrally by operation name during server startup. Hand-written routes such as `/bulk/shell-descriptors`, AAS Environment `/upload`, and `/bulk/submodel-descriptors` are classified where they are registered. This makes a forgotten trigger point fail closed instead of committing a current-state write without its required snapshot.

## Recent Changes

Recent-change endpoints read indexed metadata from the history tables, then restore the full snapshot for rows that are returned or need post-snapshot filtering. They are ordered by decreasing `history_id`, with cursor-based pagination from newest changes to older changes.
The default page size is `100`; requests above `1000` are rejected.

```mermaid
flowchart LR
    Historical["GET .../{id}/$history?date=..."] --> Validity["identifier + valid_from index"]
    Validity --> Snapshot["Latest version at or before date"]
    Recent["GET .../$recent-changes?cursor=..."] --> Cursor["descending history_id cursor + typed timestamp indexes"]
    Cursor --> Page["Newest-first page plus next cursor"]
```

Current filters:

- `cursor`
- `limit`
- `createdFrom`
- `updatedFrom`
- AAS recent changes additionally apply asset-id filtering to non-deleted rows.
- Submodel recent changes additionally apply semantic-id filtering to non-deleted rows.
- Descriptor recent changes additionally apply `assetKind`, `assetType`, and asset-id filtering to non-deleted rows.

The published Part 2 OpenAPI schema is the source of truth for the response projection. The result shapes are intentionally component-specific:

- AAS results contain the shared `type`, `createdAt`, and `updatedAt` fields plus `id`, `globalAssetId`, and `specificAssetIds`.
- Submodel results contain the shared fields plus `id`, `semanticId`, and `supplementalSemanticIds`.
- Concept Description results contain the shared fields plus `id`. This fills the missing shared-schema result type consistently with the other identifiable repositories.
- Descriptor results contain complete AAS descriptor snapshots as required by the registry profile.

For AAS and Submodel reads, resource-specific metadata is projected from the restored history snapshot, never from current normalized tables. Deleted AAS, Submodel, and Concept Description rows remain id-based tombstones with the shared change metadata. Descriptor recent changes skip deleted descriptor rows because there is no complete descriptor snapshot to return.

The encoded query contract is applied consistently: `assetIds` contain base64url-encoded `SpecificAssetId` JSON objects, Submodel `semanticId` contains a base64url-encoded reference value, and descriptor `assetType` is base64url-encoded UTF-8. Filtered feeds continue scanning history pages until the requested result limit is filled or the feed ends, so post-snapshot filtering does not underfill pages incorrectly.

When a payload does not carry administrative timestamps, the recent-change projection uses the history operation timestamp. This keeps `createdAt` and `updatedAt` populated without re-reading current tables.

## Migration Behavior

The v3.2 `Batch` asset kind is inserted at enum index `2`. Existing persisted values with index `2` or higher must be shifted by one:

```sql
UPDATE asset_information
SET asset_kind = asset_kind + 1
WHERE asset_kind >= 2;

UPDATE aas_descriptor
SET asset_kind = asset_kind + 1
WHERE asset_kind >= 2;
```

History storage is added by `1_1_1.sql`. The patch creates the metadata and payload tables, access-pattern indexes, guard switch, and mutation triggers. It does not backfill existing AAS, Submodels, Concept Descriptions, or descriptors.

After upgrade:

- State from before activation is unavailable through `$history`.
- A complete create or replace writes its supplied complete snapshot directly.
- A partial update first tries to derive the next version from the previous history snapshot.
- If an existing identifiable has no history snapshot yet, that first partial update materializes the current complete identifiable from the normalized backend and appends it. Later partial updates can derive from history.

## Security

The new endpoints are mapped as read operations in the ABAC method-rights map.

Current behavior:

- Route-level authorization applies to history and recent-change endpoints.
- Normal current-entity reads still use their established ABAC filtering paths.
- Recent-change delete tombstones only expose identifiers and shared change metadata for AAS, Submodel, and Concept Description rows.

Intentional scope boundary for this release:

- Historical snapshots are stored as JSON and are not re-querying the normalized current tables.
- `$history` and `$recent-changes` do not apply current-table ABAC formula filters or logical-expression redaction to snapshot JSON.
- Route assignments for these endpoints must only be granted to principals allowed to read the complete resource returned by the endpoint.

Recommended follow-up:

- Add identifier-aware access rules for history and recent-change endpoints.
- Keep fine-grained snapshot filtering out of the historical read contract unless a future specification requirement changes that decision.

## Scalability

Yes, the database can still grow quickly. Every versioned write creates at least one history metadata row and one JSON payload row. Configure `history.fullSnapshotInterval` above `1` to trade bounded restore work for lower payload storage.

Safeguards already implemented:

- History is stored separately from current tables, so normal GET/list endpoints continue to read current tables.
- Recent changes use indexed metadata instead of scanning current domain tables.
- JSONB snapshot/diff payloads live in one-to-one payload tables, keeping indexed event rows narrow.
- History lookup is indexed by identifier and validity range.
- Latest-version derivation is indexed by identifier and descending `history_id`.
- Recent-change pagination is cursor-based and reads one extra row for next-cursor detection.
- Administrative timestamps are extracted into typed, indexed metadata columns for filtering instead of repeatedly querying deep JSON or comparing timestamp strings.
- Partial AAS and descriptor changes derive the next snapshot from the previous reconstructed history row.
- SME changes reload only the affected top-level SME root subtree and splice it into the previous Submodel snapshot.
- Transaction-level advisory locks serialize appends only for the same history table and identifier.
- Guarded PostgreSQL mode blocks normal `UPDATE`, `DELETE`, and `TRUNCATE` operations on history tables when enabled.
- Active history mode rejects unclassified HTTP mutations before their handlers run.
- Delete rows are tombstones, not full copies, for AAS, Submodel, and Concept Description deletes.

Scalability risks that remain:

- There is no retention policy yet.
- There is no table partitioning for history tables yet.
- Very large replacements can still create large diff rows.
- Large Submodels with frequent element updates still need careful interval and retention planning.
- The first partial update for a pre-existing identifiable without history still requires a complete live-table materialization.
- JSONB snapshots are flexible but can be more expensive than narrow relational history for some queries.
- PostgreSQL guards are not equivalent to WORM storage; privileged database operators can still alter or remove them.

Recommended follow-up options:

- Add configurable retention per component, for example keep history for `N` days or `N` versions per identifier.
- Partition history tables by time or by hash of identifier when installations expect heavy write volume.
- Add monitoring metrics for history row counts and table size.
- Add optional compaction that keeps all recent rows but collapses older rows to daily or version-tagged checkpoints.
- Consider storing attachment/file changes as metadata references rather than embedding large payloads. Current file bytes are stored outside the metamodel JSON, but file element snapshots can still change frequently.

## Edge Cases

### Date At Exact Update Boundary

Runtime history is event-only. At the exact update timestamp, lookup ordering by `valid_from DESC, history_id DESC` makes the newest event win.

### Delete And Historical Reads

Dates before deletion resolve to the previous snapshot. Dates after deletion return not found.

### Recent Changes After Delete

AAS, Submodel, and Concept Description delete rows are returned as tombstones. They include the identifier and shared change metadata. Filtered recent-change queries skip tombstones when the filter requires data that the tombstone no longer contains.

### Existing Data After Migration

Migration does not create history rows for existing data. The first complete write records the supplied snapshot. The first partial write falls back to a one-time complete current-state materialization if no prior history snapshot exists.

### AAS Environment

The AAS Environment delegates the component endpoints. Its behavior should stay aligned with the underlying repository and registry services. If a new v3.2 endpoint is added to a component, the environment OpenAPI and routing must be checked as well.

## Planned Follow-Up Work

The shared append points are intentionally kept independent of a specific event broker or immutability provider. Future additions should build on them without changing repository write APIs:

- Add a transactional outbox table written in the same PostgreSQL transaction as each history row.
- Publish CloudEvents from an asynchronous outbox worker with retry and idempotency.
- Anchor row hashes in immudb from an asynchronous worker. Store append-only anchor receipts in a separate table instead of updating guarded history rows.
- Add identifier-aware access rules for `$history` and `$recent-changes`.
- Populate `AuditContext` through middleware before enabling `minimal` or `extended` identity modes.
- Implement operator-controlled retention, partitioning, monitoring metrics, and guarded-mode maintenance procedures.
- Decide whether upgraded installations need an explicit baseline backfill tool.
