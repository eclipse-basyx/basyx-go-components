# BaSyx Database Schema Notes

This folder documents the PostgreSQL schema used by the DB-backed BaSyx Go components.

The source of truth is:

- `database/base.sql`: baseline schema loaded by `basyxconfigurationservice`
- `database/patches/*.sql`: versioned migrations applied after the baseline
- `cmd/basyxconfigurationservice/main.go`: patch registration order
- `internal/common/database.go`: expected current schema version used by runtime validation

Runtime services expect the schema state in `basyxsystem` to be `clean` and the schema version to match `common.CURRENT_DATABASE_VERSION`.

## Main Storage Areas

- AAS Repository: `aas`, `aas_payload`, `asset_information`, AAS submodel-reference tables, and owner-scoped thumbnail references
- Submodel Repository: `submodel`, `submodel_payload`, `submodel_element`, type-specific SME tables, qualifier tables, and file/blob storage
- Registries and Discovery: descriptor tables, AAS identifiers, specific asset IDs, endpoint rows, and descriptor payload tables
- Concept Description Repository: `concept_description`
- Company Lookup: `company_descriptor`, `company_descriptor_name_option`, and `company_descriptor_asset_id_regex`
- AASX File Server: `aasx_package` and `aasx_package_aas_id`
- Shared binary storage: `binary_content`, `file_binary_reference`, `thumbnail_binary_reference`, and `binary_evidence_receipt`
- History and evidence: `*_history`, `*_history_payload`, `history_guard_config`, `mutation_evidence_state`, `mutation_evidence_artifacts`, `binary_reference_evidence_artifacts`, and the legacy manifest/artifact catalogs
- ABAC policy repository: `abac_policy_versions`, `abac_policy_rules`, and `abac_policy_events`

## Payload Tables

`*_payload` tables store JSONB fields that are returned through the AAS APIs but are not usually queried directly. Queryable columns belong on the owner table or a dedicated child table, not on a payload table.

## References

The current schema does not use one global `reference` or `reference_key` table. References are stored in context-specific table groups, for example:

- `aas_submodel_reference`, `aas_submodel_reference_key`
- `submodel_semantic_id_reference`, `submodel_semantic_id_reference_key`
- `submodel_element_semantic_id_reference`, `submodel_element_semantic_id_reference_key`
- `submodel_descriptor_semantic_id_reference`, `submodel_descriptor_semantic_id_reference_key`
- `specific_asset_id_external_subject_id_reference`, `specific_asset_id_external_subject_id_reference_key`

Supplemental semantic IDs have their own context-specific tables added by schema patches where query support is needed.

## Submodel Elements

`submodel_element.submodel_id` is a BIGINT foreign key to `submodel.id`. Queries that start with an external submodel identifier must join `submodel` and filter `submodel.submodel_identifier`.

`submodel_element.idshort_path` is `TEXT`, not `ltree`. The schema keeps `pg_trgm` indexes for path searches and also maintains `(submodel_id, idshort_path)`, parent, root, type, and depth indexes for common traversals.

Type-specific SME data is stored in child tables:

- `property_element`
- `multilanguage_property_value`
- `blob_element`
- `file_element` and `file_data`
- `range_element`
- `reference_element`
- `relationship_element`
- `annotated_relationship_element`
- `submodel_element_collection`
- `submodel_element_list`
- `entity_element`
- `operation_element` and `operation_variable`
- `basic_event_element`
- `capability_element`

File SME metadata such as `content_type`, `file_name`, and path-like `value` lives in `file_element`. Internal File and thumbnail payloads share one canonical PostgreSQL Large Object per SHA-256 and byte-length pair in `binary_content`. Owner-scoped references carry fresh opaque path tokens and preserve authorization boundaries even when bytes are deduplicated. Managed model values use `/aasx/files/<token>/<safe-filename>` as an AASX package-part path; it is not an HTTP endpoint. The legacy `file_data` and `thumbnail_file_data` tables remain available for existing installations and dual-read compatibility.

Patch `1_1_8.sql` adds the shared representation without scanning or rewriting legacy Large Objects. Existing binaries remain readable in their original tables and receive no retroactive WORM receipt. Replacement uploads use canonical storage and remove the replaced legacy association in the same transaction.

### v1.1.8 Upgrade And Rollback

This patch requires a quiesced upgrade. Stop all database-backed BaSyx services, take a restorable backup including PostgreSQL Large Objects, run the configuration service alone, and confirm a clean v1.1.8 schema before starting v1.1.8 services. Exact startup schema validation rejects a newly started mismatched binary, but it cannot stop an older process that was already connected during the patch.

Do not run v1.1.7 and v1.1.8 services against the upgraded database at the same time. Rollback means stopping v1.1.8, restoring the complete pre-upgrade database backup, and then restarting v1.1.7. A binary-only rollback is unsafe because v1.1.7 does not understand canonical binary references. WORM objects written after the backup may remain as immutable orphans after a restore; only objects with committed catalog receipts are valid evidence.

### v1.2.4 Temporal Value Migration

Patch `1_2_4.sql` moves legacy Property values and Range bounds from `TIME`, `DATE`, and `TIMESTAMPTZ` columns to `value_text`, `min_text`, and `max_text`. Times and dates keep their previous text. Timestamps are written in UTC with a `+00:00` offset, independent of the session TimeZone and DateStyle. That equals the previous API output only where the PostgreSQL session TimeZone was UTC and the value is not before year 1; deployments with another `postgres.timezone` or server TimeZone return the same instants with a `+00:00` offset afterwards, and clients may hold cached responses with the old text. Existing text is preserved exactly, and text that already equals this rendering may coexist with a legacy value. Timezone offsets and fractional precision already discarded by legacy storage cannot be recovered. The patch stops if a value or bound has conflicting stored representations and reports the first row ID.

The upgrade is not compatible with rolling updates. Previous binaries that are still running fail on Submodel element reads and writes once the columns are dropped, and upgraded binaries refuse to start before the schema is `v1.2.4`. Review custom queries, views, indexes, and constraints that use the legacy columns; dependent views stop the patch. Stop all services sharing the database (in Kubernetes, scale every BaSyx deployment to zero), take a restorable backup, run the Configuration Service alone, confirm a clean `v1.2.4` schema, then start the upgraded services.

The patch holds exclusive locks on `property_element` and `range_element` for the whole backfill and drops the columns in the same transaction. The five-second lock timeout bounds lock acquisition only, and the patch disables the statement timeout and, on PostgreSQL 17 and later, the transaction timeout for its transaction. It needs no TEMPORARY privilege. In a benchmark, the backfill cost about 8 µs and 0.8 KB of WAL per legacy temporal value plus two sequential scans of each table, for example about 15 seconds for 8 million Properties with 1.6 million legacy temporal values. Rehearse on a database copy to size the maintenance window, raise `max_wal_size` for large tables, and run `VACUUM (ANALYZE)` on both tables afterwards.

A failed patch rolls back its data and schema changes, and the Configuration Service marks the schema state as dirty. Services of every version, including the previous one, refuse to start while the state is dirty. Resolve the reported conflict or dependency and rerun the Configuration Service until the patch succeeds, or restore the backup, before starting any service. After a successful upgrade, rollback requires restoring the pre-upgrade backup before restarting the previous binaries.

## Enums And Integer Codes

The only PostgreSQL enum type currently created by `base.sql` is `security_type`. AAS model enums such as model type, value type, key type, modelling kind, asset kind, direction, and event state are stored as integer codes. The conversion rules are implemented in Go and the AAS SDK types used by the services.

See [enums.md](enums.md) for practical guidance.

## Query Examples

See [examples.md](examples.md) for SQL snippets that match the current schema.
