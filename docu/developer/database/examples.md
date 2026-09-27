# SQL Examples

These examples target the current schema after `database/base.sql` and all registered patches have been applied.

## List Submodels

```sql
SELECT
  id,
  submodel_identifier,
  id_short,
  kind,
  db_created_at,
  db_updated_at
FROM submodel
ORDER BY submodel_identifier
LIMIT 50;
```

## Get Root Submodel Elements By External Submodel ID

`submodel_element.submodel_id` is an internal BIGINT foreign key. Join `submodel` when filtering by the external submodel identifier.

```sql
SELECT
  se.id,
  se.id_short,
  se.model_type,
  se.idshort_path,
  se.position
FROM submodel_element se
JOIN submodel s ON s.id = se.submodel_id
WHERE s.submodel_identifier = 'urn:example:submodel:123'
  AND se.parent_sme_id IS NULL
ORDER BY
  CASE WHEN se.position IS NULL THEN 1 ELSE 0 END,
  se.position,
  se.idshort_path;
```

## Read One Element By Path

```sql
SELECT
  se.id,
  se.id_short,
  se.model_type,
  se.idshort_path,
  se.depth
FROM submodel_element se
JOIN submodel s ON s.id = se.submodel_id
WHERE s.submodel_identifier = 'urn:example:submodel:123'
  AND se.idshort_path = 'TechnicalData.Weight';
```

## Read Descendants By Path Prefix

`idshort_path` is `TEXT`, so use text predicates rather than `ltree` operators.

```sql
SELECT
  se.id,
  se.id_short,
  se.model_type,
  se.idshort_path,
  se.depth
FROM submodel_element se
JOIN submodel s ON s.id = se.submodel_id
WHERE s.submodel_identifier = 'urn:example:submodel:123'
  AND (
    se.idshort_path = 'TechnicalData'
    OR se.idshort_path LIKE 'TechnicalData.%'
  )
ORDER BY se.idshort_path;
```

## Read Direct Children

```sql
SELECT
  child.id,
  child.id_short,
  child.model_type,
  child.idshort_path,
  child.position
FROM submodel_element parent
JOIN submodel s ON s.id = parent.submodel_id
JOIN submodel_element child ON child.parent_sme_id = parent.id
WHERE s.submodel_identifier = 'urn:example:submodel:123'
  AND parent.idshort_path = 'TechnicalData'
ORDER BY
  CASE WHEN child.position IS NULL THEN 1 ELSE 0 END,
  child.position,
  child.idshort_path;
```

## Read Property Values

```sql
SELECT
  se.idshort_path,
  pe.value_type,
  COALESCE(
    pe.value_text,
    pe.value_num::text,
    pe.value_bool::text,
    pe.value_time::text,
    pe.value_date::text,
    pe.value_datetime::text
  ) AS value
FROM submodel_element se
JOIN submodel s ON s.id = se.submodel_id
JOIN property_element pe ON pe.id = se.id
WHERE s.submodel_identifier = 'urn:example:submodel:123'
ORDER BY se.idshort_path;
```

## Read Multi-Language Property Values

```sql
SELECT
  se.idshort_path,
  mlp.language,
  mlp.text
FROM submodel_element se
JOIN submodel s ON s.id = se.submodel_id
JOIN multilanguage_property_value mlp ON mlp.submodel_element_id = se.id
WHERE s.submodel_identifier = 'urn:example:submodel:123'
ORDER BY se.idshort_path, mlp.language;
```

## Read File SME Metadata And Large Object OID

```sql
SELECT
  se.idshort_path,
  fe.content_type,
  fe.file_name,
  fe.value,
  fd.file_oid
FROM submodel_element se
JOIN submodel s ON s.id = se.submodel_id
JOIN file_element fe ON fe.id = se.id
LEFT JOIN file_data fd ON fd.id = fe.id
WHERE s.submodel_identifier = 'urn:example:submodel:123'
ORDER BY se.idshort_path;
```

## Find Submodels By Semantic ID Key Value

```sql
SELECT DISTINCT
  s.submodel_identifier,
  s.id_short
FROM submodel s
JOIN submodel_semantic_id_reference r ON r.id = s.id
JOIN submodel_semantic_id_reference_key rk ON rk.reference_id = r.id
WHERE rk.value = 'https://admin-shell.io/idta/AssetInterfacesDescription/1/0';
```

## Find Elements By Semantic ID Key Value

```sql
SELECT DISTINCT
  s.submodel_identifier,
  se.idshort_path,
  se.model_type
FROM submodel_element se
JOIN submodel s ON s.id = se.submodel_id
JOIN submodel_element_semantic_id_reference r ON r.id = se.id
JOIN submodel_element_semantic_id_reference_key rk ON rk.reference_id = r.id
WHERE rk.value = '0173-1#02-AAO677#003'
ORDER BY s.submodel_identifier, se.idshort_path;
```

## Read AAS Descriptor Endpoints

```sql
SELECT
  ad.id AS aas_identifier,
  ad.id_short,
  ade.interface,
  ade.href,
  ade.endpoint_protocol
FROM aas_descriptor ad
JOIN aas_descriptor_endpoint ade ON ade.descriptor_id = ad.descriptor_id
WHERE ad.id = 'urn:example:aas:123'
ORDER BY ade.position;
```

## Read Specific Asset IDs For An AAS Descriptor

```sql
SELECT
  ad.id AS aas_identifier,
  sai.name,
  sai.value
FROM aas_descriptor ad
JOIN descriptor d ON d.id = ad.descriptor_id
JOIN specific_asset_id sai ON sai.descriptor_id = d.id
WHERE ad.id = 'urn:example:aas:123'
ORDER BY sai.position;
```

## Operational Tips

- Filter by the owner first, such as `submodel.submodel_identifier` or `aas_descriptor.id`.
- Join through internal numeric IDs when querying child tables.
- Use `idshort_path = ...` for exact path lookups and prefix `LIKE` only for path-subtree scans.
- Keep queryable data out of `*_payload` tables.
