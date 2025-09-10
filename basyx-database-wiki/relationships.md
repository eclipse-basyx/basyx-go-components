# Relationships & Diagrams

This section provides an overview of the main relationships between tables in the BaSyx database schema.

> **Note:** The Asset Administration Shell (AAS) is a standard provided and maintained by the IDTA (Industrial Digital Twin Association). Eclipse BaSyx is an implementation/server provider for this standard.

## Entity-Relationship Diagram (Mermaid)

```mermaid
erDiagram
  SUBMODEL {
    TEXT id PK
    TEXT id_short
    TEXT category
    modelling_kind kind
    BIGINT semantic_id FK
    TEXT model_type
  }
  SUBMODEL_ELEMENT {
    BIGSERIAL id PK
    TEXT submodel_id FK
    BIGINT parent_sme_id FK
    INTEGER position
    TEXT id_short
    TEXT category
    aas_submodel_elements model_type
    BIGINT semantic_id FK
    LTREE path_ltree
  }
  REFERENCE {
    BIGSERIAL id PK
    reference_types type
  }
  REFERENCE_KEY {
    BIGSERIAL id PK
    BIGINT reference_id FK
    INTEGER position
    TEXT type
    TEXT value
  }

  SUBMODEL ||--o{ SUBMODEL_ELEMENT : contains
  SUBMODEL_ELEMENT ||--o{ SUBMODEL_ELEMENT : parent
  SUBMODEL_ELEMENT }o--|| SUBMODEL : belongs_to
  SUBMODEL_ELEMENT }o--|| REFERENCE : semantic_id
  REFERENCE ||--o{ REFERENCE_KEY : has_keys

```

## Notes
- `submodel` contains many `submodel_element` (root elements have `parent_sme_id` NULL)
- `submodel_element` can be nested (tree structure via `parent_sme_id`)
- `reference` and `reference_key` model references and their keys
- Specialized tables (e.g., `property_element`, `submodel_element_list`) extend `submodel_element` by 1:1 relationship

For a full list of entities, see [Entity Reference](./entities.md).
