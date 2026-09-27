# Enum And Code Storage

The PostgreSQL schema intentionally uses very few database enum types.

## PostgreSQL Enum Types

`database/base.sql` currently creates one PostgreSQL enum:

```sql
CREATE TYPE security_type AS ENUM ('NONE', 'RFC_TLSA', 'W3C_DID');
```

It is used for security attribute data where a real database enum is appropriate.

## AAS Model Enums

Most AAS model enum values are stored as integer codes in queryable columns. Examples include:

- `aas.model_type`
- `asset_information.asset_kind`
- `submodel.kind`
- `submodel_element.model_type`
- `property_element.value_type`
- `range_element.value_type`
- `submodel_element_list.type_value_list_element`
- `submodel_element_list.value_type_list_element`
- `entity_element.entity_type`
- `operation_variable.role`
- `basic_event_element.direction`
- `basic_event_element.state`
- reference key `type` columns such as `submodel_semantic_id_reference_key.type`

Conversion between API strings and these integer values is handled in Go through the AAS SDK types and the repository's parsing/stringification helpers.

## Query Guidance

Prefer API-level filtering or the shared query-language mapping when possible. If you write SQL directly, compare against integer values only when the code path that owns the table already documents the corresponding SDK enum value.

Do not add new PostgreSQL enum types for AAS model concepts without checking the persistence and migration implications. Integer columns keep migrations and SDK compatibility simpler when the AAS metamodel evolves.
