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
- Migration `1_1_0.sql`: adds v3.2 timestamp columns, history tables, enum migration for `Batch`, and initial history backfill.

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

History is stored as append-only JSON snapshots in dedicated tables:

- `aas_history`
- `submodel_history`
- `concept_description_history`
- `descriptor_history`

Each history row stores:

- `identifier`
- `change_type`: `Created`, `Updated`, or `Deleted`
- `snapshot`
- `deleted`
- `valid_from`
- `valid_to`
- `operation_time`
- administrative timestamp text values for `createdFrom` and `updatedFrom` filters

On every create, update, or delete, the current open history row for that identifier is closed by setting `valid_to`, and a new row is appended. This happens in the same database transaction as the current-table mutation where the persistence path supports transactions.

History lookup uses:

```text
valid_from <= requested_date < valid_to
```

If the matching row is marked as deleted, the history endpoint returns not found. This means a deleted entity can still be resolved for dates before deletion, but not after deletion.

## Submodel Element Changes

Submodel elements are not versioned independently. They are part of their owning Submodel.

When a submodel element is created, updated, patched, deleted, or when an attachment changes, the implementation records a new full Submodel snapshot. This makes `/submodels/{id}/$history` answer the practical question, "what did the complete Submodel look like then?", including nested element changes.

Tradeoff:

- This is simple and correct for API consumers.
- It can create large history rows when a small element changes inside a large Submodel.
- It avoids reconstructing historical Submodels from many normalized child-table deltas.

## Recent Changes

Recent-change endpoints read the same history tables. They are ordered by increasing `history_id`, with cursor-based pagination.

Current filters:

- `cursor`
- `limit`
- `createdFrom`
- `updatedFrom`
- AAS recent changes additionally apply asset-id filtering to non-deleted rows.
- Submodel recent changes additionally apply semantic-id filtering to non-deleted rows.

Delete rows are tombstones. For AAS and Submodel recent changes they are returned with the identifier and change metadata, but without reconstructing the deleted object. This avoids parsing tombstone payloads as full metamodel instances and avoids leaking stale metadata into filtered reads.

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

The migration also backfills one open `Created` history row for existing current records. This ensures history and recent-change endpoints have a baseline after upgrade.

Important limitation: the migration backfill creates baseline snapshots from the current tables available in SQL. Runtime-created history rows use the Go serialization path and are fuller snapshots. The migration baseline is good enough to establish an initial historical version, but it is not a complete reconstruction of every nested child row for complex Submodels or descriptors.

## Security

The new endpoints are mapped as read operations in the ABAC method-rights map.

Current behavior:

- Route-level authorization applies to history and recent-change endpoints.
- Normal current-entity reads still use their established ABAC filtering paths.
- Recent-change delete tombstones only expose identifiers and change metadata for AAS and Submodel rows.

Potential edge case:

- Historical snapshots are stored as JSON and are not re-querying the normalized current tables. If access rules become stricter after a historical snapshot was written, we need to ensure historical reads do not leak information that would be filtered from a current read. The current implementation relies on endpoint authorization and component-level access checks, but per-snapshot field redaction is not implemented.

Recommended follow-up:

- Add security integration tests for denied users reading `$history` and `$recent-changes`.
- Decide whether historical snapshots should be redacted by the same field-level ABAC formulas as current reads, or whether history access is an all-or-nothing read right.

## Scalability

Yes, the database can grow much faster now. Every write creates at least one history row with a JSON snapshot. Submodel-element writes are especially growth-heavy because they snapshot the whole owning Submodel.

Safeguards already implemented:

- History is stored separately from current tables, so normal GET/list endpoints continue to read current tables.
- Recent changes use indexed metadata instead of scanning current domain tables.
- History lookup is indexed by identifier and validity range.
- Recent-change pagination is cursor-based and reads one extra row for next-cursor detection.
- Administrative timestamps are extracted into metadata columns for filtering instead of repeatedly querying deep JSON.
- Delete rows are tombstones, not full copies, for AAS and Submodel deletes.
- The migration backfill inserts one baseline row per existing entity, not one row per child object.

Scalability risks that remain:

- There is no retention policy yet.
- There is no compaction strategy for high-frequency updates.
- There is no table partitioning for history tables yet.
- Full snapshots duplicate unchanged data across versions.
- Large Submodels with frequent element updates can grow history very quickly.
- JSONB snapshots are flexible but can be more expensive than narrow relational history for some queries.

Recommended follow-up options:

- Add configurable retention per component, for example keep history for `N` days or `N` versions per identifier.
- Partition history tables by time or by hash of identifier when installations expect heavy write volume.
- Add monitoring metrics for history row counts and table size.
- Add optional compaction that keeps all recent rows but collapses older rows to daily or version-tagged checkpoints.
- Consider storing attachment/file changes as metadata references rather than embedding large payloads. Current file bytes are stored outside the metamodel JSON, but file element snapshots can still change frequently.

## Edge Cases

### Date At Exact Update Boundary

The old row is valid until `valid_to`. The new row is valid from its `valid_from`. The lookup uses an exclusive upper bound, so at the exact update timestamp the new row wins.

### Delete And Historical Reads

Dates before deletion resolve to the previous snapshot. Dates after deletion return not found.

### Recent Changes After Delete

AAS and Submodel delete rows are returned as tombstones. They include the identifier and change type. Filtered recent-change queries skip tombstones when the filter requires data that the tombstone no longer contains.

### Migration Backfill For Existing Data

Backfill creates initial `Created` rows. It cannot know the real historical create/update sequence from before v3.2; it only captures the current state at migration time.

### AAS Environment

The AAS Environment delegates the component endpoints. Its behavior should stay aligned with the underlying repository and registry services. If a new v3.2 endpoint is added to a component, the environment OpenAPI and routing must be checked as well.

## What To Watch Before Release

The biggest implementation questions still worth reviewing are:

- Is route-level read authorization enough for historical snapshots, or do we need per-snapshot ABAC redaction?
- Do we need an operator-facing retention/compaction setting before enabling this in large deployments?
- Is the migration backfill baseline sufficient for upgraded installations, or do consumers expect full nested snapshots immediately after upgrade?
- Should descriptor recent changes include delete tombstones in the public response, or is skipping deleted descriptors acceptable for the registry profile?
- Do we want security tests for every new history/recent-change endpoint in addition to integration tests?
