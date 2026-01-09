# Database Query Examples

Practical SQL examples for working with the BaSyx database schema.

## Table of Contents

1. [Submodel Queries](#submodel-queries)
2. [Submodel Element Queries](#submodel-element-queries)
3. [Property Searches](#property-searches)
4. [Reference & Semantic ID Queries](#reference--semantic-id-queries)
5. [Descriptor & Registry Queries](#descriptor--registry-queries)
6. [Hierarchical Path Queries](#hierarchical-path-queries)

---

## Submodel Queries

### Get all submodels with their basic info

```sql
SELECT 
    s.id,
    s.id_short,
    s.kind,
    lstt.text as description
FROM submodel s
LEFT JOIN lang_string_text_type_reference lsttr ON s.description_id = lsttr.id
LEFT JOIN lang_string_text_type lstt ON lsttr.id = lstt.lang_string_text_type_reference_id
WHERE lstt.language = 'en' OR lstt.language IS NULL;
```

### Find submodels by semantic ID

```sql
SELECT DISTINCT s.id, s.id_short
FROM submodel s
JOIN reference r ON s.semantic_id = r.id
JOIN reference_key rk ON r.id = rk.reference_id
WHERE rk.value LIKE '%Temperature%';
```

### Count elements in each submodel

```sql
SELECT 
    s.id,
    s.id_short,
    COUNT(se.id) as element_count
FROM submodel s
LEFT JOIN submodel_element se ON s.id = se.submodel_id
GROUP BY s.id, s.id_short
ORDER BY element_count DESC;
```

---

## Submodel Element Queries

### Get all root elements of a submodel (ordered)

```sql
SELECT 
    id,
    id_short,
    model_type,
    position
FROM submodel_element
WHERE submodel_id = 'urn:example:submodel:123'
  AND parent_sme_id IS NULL
ORDER BY position;
```

### Get children of a specific element

```sql
SELECT 
    id,
    id_short,
    model_type,
    idshort_path,
    depth
FROM submodel_element
WHERE parent_sme_id = 12345
ORDER BY position;
```

### Find element by path

```sql
SELECT * FROM submodel_element
WHERE submodel_id = 'urn:example:submodel:123'
  AND idshort_path = 'TechnicalData.Weight';
```

### Get all descendants of an element (using ltree)

```sql
-- Find all children, grandchildren, etc. under 'TechnicalData'
SELECT 
    id_short,
    idshort_path,
    depth,
    model_type
FROM submodel_element
WHERE submodel_id = 'urn:example:submodel:123'
  AND idshort_path <@ 'TechnicalData'  -- <@ means "is descendant of"
ORDER BY idshort_path;
```

### Get element tree with nesting levels

```sql
SELECT 
    REPEAT('  ', depth) || id_short as tree_view,
    model_type,
    depth
FROM submodel_element
WHERE submodel_id = 'urn:example:submodel:123'
ORDER BY idshort_path;
```

---

## Property Searches

### Find properties by name (fuzzy search)

```sql
SELECT 
    se.id_short,
    se.idshort_path,
    pe.value_text,
    pe.value_num,
    pe.value_type
FROM submodel_element se
JOIN property_element pe ON se.id = pe.id
WHERE se.submodel_id = 'urn:example:submodel:123'
  AND se.id_short % 'temperature'  -- % is the trigram similarity operator
ORDER BY similarity(se.id_short, 'temperature') DESC;
```

### Find properties with numeric values in range

```sql
SELECT 
    se.id_short,
    pe.value_num,
    pe.value_type
FROM submodel_element se
JOIN property_element pe ON se.id = pe.id
WHERE se.submodel_id = 'urn:example:submodel:123'
  AND pe.value_num BETWEEN 20 AND 100
  AND pe.value_type IN ('xs:double', 'xs:float', 'xs:int');
```

### Search property values (text)

```sql
SELECT 
    se.id_short,
    se.idshort_path,
    pe.value_text
FROM submodel_element se
JOIN property_element pe ON se.id = pe.id
WHERE pe.value_text ILIKE '%motor%'
  AND pe.value_type = 'xs:string';
```

### Get all properties of a submodel with values

```sql
SELECT 
    se.id_short,
    se.idshort_path,
    pe.value_type,
    COALESCE(
        pe.value_text,
        pe.value_num::text,
        pe.value_bool::text,
        pe.value_datetime::text
    ) as value
FROM submodel_element se
JOIN property_element pe ON se.id = pe.id
WHERE se.submodel_id = 'urn:example:submodel:123'
ORDER BY se.idshort_path;
```

---

## Reference & Semantic ID Queries

### Get full reference chain

```sql
SELECT 
    rk.position,
    rk.type,
    rk.value
FROM reference r
JOIN reference_key rk ON r.id = rk.reference_id
WHERE r.id = 123
ORDER BY rk.position;
```

### Find all submodels with a specific semantic ID pattern

```sql
SELECT DISTINCT 
    s.id,
    s.id_short,
    rk.value as semantic_id_value
FROM submodel s
JOIN reference r ON s.semantic_id = r.id
JOIN reference_key rk ON r.id = rk.reference_id
WHERE rk.value LIKE '%0173-1%'
  AND rk.type = 'ConceptDescription';
```

### Find elements by semantic ID

```sql
SELECT 
    se.id_short,
    se.idshort_path,
    se.model_type,
    rk.value as semantic_id
FROM submodel_element se
JOIN reference r ON se.semantic_id = r.id
JOIN reference_key rk ON r.id = rk.reference_id
WHERE rk.value = 'https://example.com/ids/cd/Temperature'
ORDER BY se.submodel_id, se.idshort_path;
```

---

## Descriptor & Registry Queries

### Get all AAS descriptors with endpoints

```sql
SELECT 
    ad.id,
    ad.id_short,
    ad.asset_kind,
    ade.interface,
    ade.href,
    ade.endpoint_protocol
FROM aas_descriptor ad
JOIN descriptor d ON ad.descriptor_id = d.id
LEFT JOIN aas_descriptor_endpoint ade ON d.id = ade.descriptor_id
ORDER BY ad.id, ade.position;
```

### Find submodel descriptors by semantic ID

```sql
SELECT 
    sd.id,
    sd.id_short,
    rk.value as semantic_id,
    ade.href
FROM submodel_descriptor sd
JOIN descriptor d ON sd.descriptor_id = d.id
LEFT JOIN reference r ON sd.semantic_id = r.id
LEFT JOIN reference_key rk ON r.id = rk.reference_id
LEFT JOIN aas_descriptor_endpoint ade ON d.id = ade.descriptor_id
WHERE rk.value LIKE '%Nameplate%';
```

### Search by specific asset ID

```sql
SELECT 
    ad.id,
    ad.id_short,
    sai.name as asset_id_name,
    sai.value as asset_id_value
FROM aas_descriptor ad
JOIN descriptor d ON ad.descriptor_id = d.id
JOIN specific_asset_id sai ON d.id = sai.descriptor_id
WHERE sai.name = 'SerialNumber'
  AND sai.value = 'SN-12345';
```

### Get all submodels belonging to an AAS

```sql
SELECT 
    sd.id,
    sd.id_short,
    ad.id as aas_id,
    ad.id_short as aas_id_short
FROM submodel_descriptor sd
JOIN aas_descriptor ad ON sd.aas_descriptor_id = ad.descriptor_id
WHERE ad.id = 'urn:example:aas:123'
ORDER BY sd.position;
```

---

## Hierarchical Path Queries

### Find all elements at a specific depth

```sql
SELECT 
    id_short,
    idshort_path,
    model_type
FROM submodel_element
WHERE submodel_id = 'urn:example:submodel:123'
  AND depth = 2
ORDER BY idshort_path;
```

### Get direct children using ltree

```sql
-- Direct children of 'TechnicalData'
SELECT 
    id_short,
    idshort_path,
    model_type
FROM submodel_element
WHERE submodel_id = 'urn:example:submodel:123'
  AND idshort_path ~ 'TechnicalData.*{1}'  -- regex: exactly 1 level down
ORDER BY position;
```

### Search paths by pattern

```sql
-- Find all paths containing 'weight' anywhere
SELECT 
    id_short,
    idshort_path,
    model_type
FROM submodel_element
WHERE submodel_id = 'urn:example:submodel:123'
  AND idshort_path::text LIKE '%Weight%'
ORDER BY idshort_path;
```

---

## Complex Queries

### Full submodel export (elements + properties)

```sql
SELECT 
    se.id_short,
    se.idshort_path,
    se.model_type,
    se.depth,
    pe.value_type,
    pe.value_text,
    pe.value_num,
    pe.value_bool
FROM submodel_element se
LEFT JOIN property_element pe ON se.id = pe.id
WHERE se.submodel_id = 'urn:example:submodel:123'
ORDER BY se.idshort_path;
```

### Multi-language property values

```sql
SELECT 
    se.id_short,
    se.idshort_path,
    mlpv.language,
    mlpv.text
FROM submodel_element se
JOIN multilanguage_property mlp ON se.id = mlp.id
JOIN multilanguage_property_value mlpv ON mlp.id = mlpv.mlp_id
WHERE se.submodel_id = 'urn:example:submodel:123'
ORDER BY se.idshort_path, mlpv.language;
```

### Get qualifiers for elements

```sql
SELECT 
    se.id_short,
    q.type as qualifier_type,
    q.kind as qualifier_kind,
    q.value_text,
    q.value_num
FROM submodel_element se
JOIN submodel_element_qualifier seq ON se.id = seq.sme_id
JOIN qualifier q ON seq.qualifier_id = q.id
WHERE se.submodel_id = 'urn:example:submodel:123'
ORDER BY se.idshort_path, q.type;
```

### Operations with variables

```sql
SELECT 
    se.id_short,
    ov.role,
    ov.position,
    ov.value->>'idShort' as variable_id_short,
    ov.value->>'modelType' as variable_type
FROM submodel_element se
JOIN operation_element oe ON se.id = oe.id
JOIN operation_variable ov ON oe.id = ov.operation_id
WHERE se.submodel_id = 'urn:example:submodel:123'
ORDER BY se.id_short, ov.role, ov.position;
```

---

## Performance Tips

1. **Always filter by `submodel_id` first** - It's the primary partition key
2. **Use ltree operators** for path queries instead of LIKE on text
3. **Leverage indexes** - Check `ix_*` indexes for optimized columns
4. **Use EXISTS** instead of IN for large subqueries
5. **Trigram search** - Use `%` operator for fuzzy matching on indexed columns

## Common Pitfalls

❌ **Don't**: `WHERE idshort_path::text LIKE 'parent%'`  
✅ **Do**: `WHERE idshort_path <@ 'parent'`

❌ **Don't**: Multiple separate queries for parent + children  
✅ **Do**: Single query with ltree operators

❌ **Don't**: Full table scans without submodel_id  
✅ **Do**: Always include submodel_id in WHERE clause
