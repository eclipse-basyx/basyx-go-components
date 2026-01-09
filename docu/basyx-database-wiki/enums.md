# PostgreSQL Types & Enums Reference

Complete reference for all custom PostgreSQL types and enums used in the BaSyx database schema.

---

## PostgreSQL Extensions

```sql
CREATE EXTENSION ltree;    -- Hierarchical tree paths
CREATE EXTENSION pg_trgm;  -- Trigram fuzzy text search
```

---

## Custom ENUM Types

### `modelling_kind`

Distinguishes between instance data and template definitions.

```sql
CREATE TYPE modelling_kind AS ENUM ('Instance', 'Template');
```

| Value | Description |
|-------|-------------|
| `Instance` | Actual data instances |
| `Template` | Templates/patterns for creating instances |

**Used in**: `submodel`, `submodel_element`

---

### `aas_submodel_elements`

All possible submodel element types in the AAS metamodel.

```sql
CREATE TYPE aas_submodel_elements AS ENUM (
  'AnnotatedRelationshipElement',
  'BasicEventElement',
  'Blob',
  'Capability',
  'DataElement',
  'Entity',
  'EventElement',
  'File',
  'MultiLanguageProperty',
  'Operation',
  'Property',
  'Range',
  'ReferenceElement',
  'RelationshipElement',
  'SubmodelElement',
  'SubmodelElementCollection',
  'SubmodelElementList'
);
```

**Key types**:
- **Property**: Single typed value (string, int, bool, etc.)
- **File**: Reference to file resource
- **Blob**: Binary data inline
- **Operation**: Invokable operation with input/output variables
- **SubmodelElementCollection**: Container for mixed child elements
- **SubmodelElementList**: Typed list of elements
- **Range**: Min/max value pair
- **Entity**: Logical entity with statements

**Used in**: `submodel_element.model_type`

---

### `data_type_def_xsd`

XML Schema data types for typed values.

```sql
CREATE TYPE data_type_def_xsd AS ENUM (
  'xs:anyURI',
  'xs:base64Binary',
  'xs:boolean',
  'xs:byte',
  'xs:date',
  'xs:dateTime',
  'xs:decimal',
  'xs:double',
  'xs:duration',
  'xs:float',
  'xs:gDay',
  'xs:gMonth',
  'xs:gMonthDay',
  'xs:gYear',
  'xs:gYearMonth',
  'xs:hexBinary',
  'xs:int',
  'xs:integer',
  'xs:long',
  'xs:negativeInteger',
  'xs:nonNegativeInteger',
  'xs:nonPositiveInteger',
  'xs:positiveInteger',
  'xs:short',
  'xs:string',
  'xs:time',
  'xs:unsignedByte',
  'xs:unsignedInt',
  'xs:unsignedLong',
  'xs:unsignedShort'
);
```

**Common types**:
- `xs:string` - Text values
- `xs:int`, `xs:long` - Integers
- `xs:double`, `xs:float` - Floating point numbers
- `xs:boolean` - True/false
- `xs:dateTime`, `xs:date`, `xs:time` - Temporal values

**Used in**: `property_element.value_type`, `range_element.value_type`, `qualifier.value_type`

---

### `reference_types`

Type of semantic reference.

```sql
CREATE TYPE reference_types AS ENUM ('ExternalReference', 'ModelReference');
```

| Value | Description |
|-------|-------------|
| `ExternalReference` | References to external resources (URLs, IRIs) |
| `ModelReference` | References within the AAS model itself |

**Used in**: `reference.type`

---

### `key_type`

Types of keys in a reference chain.

```sql
CREATE TYPE key_type AS ENUM (
  'AnnotatedRelationshipElement',
  'AssetAdministrationShell',
  'BasicEventElement',
  'Blob',
  'Capability',
  'ConceptDescription',
  'DataElement',
  'Entity',
  'EventElement',
  'File',
  'FragmentReference',
  'GlobalReference',
  'Identifiable',
  'MultiLanguageProperty',
  'Operation',
  'Property',
  'Range',
  'Referable',
  'ReferenceElement',
  'RelationshipElement',
  'Submodel',
  'SubmodelElement',
  'SubmodelElementCollection',
  'SubmodelElementList'
);
```

**Key values**:
- `GlobalReference` - External semantic ID (ECLASS, IEC CDD, etc.)
- `ConceptDescription` - Reference to concept definition
- `Submodel` - Reference to a submodel
- `Property`, `File`, etc. - References to specific element types

**Used in**: `reference_key.type`

---

### `qualifier_kind`

Classification of qualifier types.

```sql
CREATE TYPE qualifier_kind AS ENUM (
  'ConceptQualifier',
  'TemplateQualifier',
  'ValueQualifier'
);
```

| Value | Description |
|-------|-------------|
| `ConceptQualifier` | Qualifier defining concept constraints |
| `TemplateQualifier` | Template-level metadata |
| `ValueQualifier` | Instance value constraints |

**Used in**: `qualifier.kind`

---

### `entity_type`

Type of entity element.

```sql
CREATE TYPE entity_type AS ENUM ('CoManagedEntity', 'SelfManagedEntity');
```

| Value | Description |
|-------|-------------|
| `CoManagedEntity` | Managed externally |
| `SelfManagedEntity` | Self-contained management |

**Used in**: `entity_element.entity_type`

---

### `direction`

Data flow direction.

```sql
CREATE TYPE direction AS ENUM ('input', 'output');
```

**Used in**: `basic_event_element.direction`

---

### `state_of_event`

Event state.

```sql
CREATE TYPE state_of_event AS ENUM ('off', 'on');
```

**Used in**: `basic_event_element.state`

---

### `operation_var_role`

Role of operation variables.

```sql
CREATE TYPE operation_var_role AS ENUM ('in', 'out', 'inout');
```

| Value | Description |
|-------|-------------|
| `in` | Input parameter |
| `out` | Output/return value |
| `inout` | Both input and output |

**Used in**: `operation_variable.role`

---

### `data_type_iec61360`

IEC 61360 data types for data specifications.

```sql
CREATE TYPE data_type_iec61360 AS ENUM (
  'BLOB',
  'BOOLEAN',
  'DATE',
  'FILE',
  'HTML',
  'INTEGER_COUNT',
  'INTEGER_CURRENCY',
  'INTEGER_MEASURE',
  'IRDI',
  'IRI',
  'RATIONAL',
  'RATIONAL_MEASURE',
  'REAL_COUNT',
  'REAL_CURRENCY',
  'REAL_MEASURE',
  'STRING',
  'STRING_TRANSLATABLE',
  'TIME',
  'TIMESTAMP',
  'URL'
);
```

**Used in**: `data_specification_iec61360.data_type`

---

### `asset_kind`

Classification of assets.

```sql
CREATE TYPE asset_kind AS ENUM ('Instance', 'Type', 'Role', 'NotApplicable');
```

| Value | Description |
|-------|-------------|
| `Instance` | Physical/logical instance |
| `Type` | Asset type/class |
| `Role` | Role in a system |
| `NotApplicable` | Not applicable |

**Used in**: `aas_descriptor.asset_kind`

---

### `security_type`

Security mechanism types.

```sql
CREATE TYPE security_type AS ENUM ('NONE', 'RFC_TLSA', 'W3C_DID');
```

| Value | Description |
|-------|-------------|
| `NONE` | No security |
| `RFC_TLSA` | TLS authentication (RFC 6698) |
| `W3C_DID` | Decentralized identifier |

**Used in**: `security_attributes.security_type`

---

## Type Usage Matrix

| Table | Column | ENUM Type |
|-------|--------|-----------|
| `submodel` | `kind` | `modelling_kind` |
| `submodel_element` | `kind` | `modelling_kind` |
| `submodel_element` | `model_type` | `aas_submodel_elements` |
| `property_element` | `value_type` | `data_type_def_xsd` |
| `range_element` | `value_type` | `data_type_def_xsd` |
| `qualifier` | `value_type` | `data_type_def_xsd` |
| `qualifier` | `kind` | `qualifier_kind` |
| `reference` | `type` | `reference_types` |
| `reference_key` | `type` | `key_type` |
| `entity_element` | `entity_type` | `entity_type` |
| `basic_event_element` | `direction` | `direction` |
| `basic_event_element` | `state` | `state_of_event` |
| `operation_variable` | `role` | `operation_var_role` |
| `data_specification_iec61360` | `data_type` | `data_type_iec61360` |
| `aas_descriptor` | `asset_kind` | `asset_kind` |
| `security_attributes` | `security_type` | `security_type` |

---

## Working with ENUMs

### Querying ENUM values

```sql
-- Get all properties of type string
SELECT * FROM property_element 
WHERE value_type = 'xs:string';

-- Get all instance submodels
SELECT * FROM submodel 
WHERE kind = 'Instance';
```

### Filtering by multiple ENUM values

```sql
-- Get numeric properties
SELECT * FROM property_element 
WHERE value_type IN ('xs:int', 'xs:double', 'xs:float', 'xs:decimal');
```

### Type conversion

```sql
-- Cast ENUM to text
SELECT model_type::text FROM submodel_element;

-- Cast text to ENUM (must be valid value)
SELECT 'Property'::aas_submodel_elements;
```

---

## Data Type Storage

### Property Values by Type

The `property_element` table has separate columns for different data types:

```sql
property_element (
  value_type     data_type_def_xsd,  -- Which type
  value_text     TEXT,                -- xs:string, xs:anyURI
  value_num      NUMERIC,             -- xs:int, xs:double, xs:float, etc.
  value_bool     BOOLEAN,             -- xs:boolean
  value_datetime TIMESTAMPTZ,         -- xs:dateTime, xs:date
  value_time     TIME,                -- xs:time
  value_id       BIGINT               -- Reference type values
)
```

**Indexes match data types**:
- `ix_prop_num` - Numeric values
- `ix_prop_text_trgm` - Text with fuzzy search
- `ix_prop_bool` - Boolean values
- `ix_prop_dt` - DateTime values
- `ix_prop_time` - Time values

---

## Best Practices

1. **Use ENUMs for filtering**: Database can use efficient index scans
2. **Check constraints**: ENUMs prevent invalid values at DB level
3. **Type safety**: Application code can rely on valid enum values
4. **Performance**: ENUM comparison is faster than string comparison
5. **Documentation**: ENUM types serve as inline documentation

---

## Extending ENUMs

⚠️ **Warning**: Modifying ENUMs requires careful migration:

```sql
-- Add new value to existing ENUM
ALTER TYPE aas_submodel_elements ADD VALUE 'NewElementType';

-- Remove requires recreating the type (complex migration)
```

For production systems, consider:
1. Adding new values at the end
2. Never removing values that might be in use
3. Testing migrations thoroughly
