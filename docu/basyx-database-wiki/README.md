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

File SME metadata such as `content_type`, `file_name`, and path-like `value` lives in `file_element`. Internal File and thumbnail payloads share one canonical PostgreSQL Large Object per SHA-256 and byte-length pair in `binary_content`. Owner-scoped references carry fresh opaque path tokens and preserve authorization boundaries even when bytes are deduplicated. Managed model values use `/aasx/files/<token>/<safe-filename>` as an AASX package-part path; it is not an HTTP endpoint. The legacy `file_data` and `thumbnail_file_data` tables remain available only for interrupted-upgrade compatibility reads.

Patch `1_1_8.sql` converts legacy File and thumbnail Large Objects to the shared representation without generating history rows or WORM evidence. Existing binaries therefore remain readable after upgrade but receive no retroactive WORM receipt.

## Enums And Integer Codes

The only PostgreSQL enum type currently created by `base.sql` is `security_type`. AAS model enums such as model type, value type, key type, modelling kind, asset kind, direction, and event state are stored as integer codes. The conversion rules are implemented in Go and the AAS SDK types used by the services.

See [enums.md](enums.md) for practical guidance.

## Query Examples

See [examples.md](examples.md) for SQL snippets that match the current schema.
