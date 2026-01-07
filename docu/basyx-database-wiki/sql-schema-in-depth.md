# BaSyx SQL Schema In-Depth Documentation

This document provides a detailed explanation of the BaSyx SQL schema, including table purposes, field descriptions, relationships, and design rationale. It is intended for developers and database administrators who need to understand or extend the schema.

---

## Table of Contents
- [Extensions](#extensions)
- [Enums](#enums)
- [Core Tables](#core-tables)
  - [reference](#reference)
  - [reference_key](#reference_key)
  - [submodel](#submodel)
  - [submodel_element](#submodel_element)
- [Specialized Submodel Element Tables](#specialized-submodel-element-tables)
- [Collections and Lists](#collections-and-lists)
- [Entity, Operation, and Event Tables](#entity-operation-and-event-tables)
- [Qualifiers](#qualifiers)
- [Indexes and Performance](#indexes-and-performance)
- [Design Rationale](#design-rationale)

---

## Extensions

The schema uses PostgreSQL extensions for advanced features:
- `ltree`: Enables hierarchical path queries (used in `submodel_element.path_ltree`).
- `pg_trgm`: Provides fast trigram-based text search (used for text fields).

## Enums

Several enums are defined for strong typing and validation, e.g.:
- `modelling_kind`: 'Instance', 'Template'
- `aas_submodel_elements`: All AAS element types
- `data_type_def_xsd`: XSD-compatible data types
- ...and others (see [enums.md](./enums.md))

## Core Tables

### reference
Stores references (semantic IDs, etc.) as defined by the AAS standard.
- `id`: Primary key
- `type`: Enum (`reference_types`)

### reference_key
Stores ordered keys for a reference (to support multi-key references).
- `reference_id`: Foreign key to `reference`
- `position`: Array index (order matters)
- `type`, `value`: Key type and value

### submodel
Represents a Submodel.
- `id`: Primary key (AAS identifier)
- `id_short`, `category`, `kind`, `semantic_id`, `model_type`: Metadata fields

### submodel_element
Contains the common properties Submodel Element (tree node).
- `id`: Primary key
- `submodel_id`: Foreign key to `submodel`
- `parent_sme_id`: Parent element (for tree structure)
- `position`: Order among siblings (for lists/collections)
- `id_short`, `category`, `model_type`, `semantic_id`, `path_ltree`: Metadata and hierarchy

**Tree Structure:**
The `submodel_element` table implements a hierarchical tree structure, where each element can have a parent, forming nested submodel elements. The `parent_sme_id` field references the parent element, allowing for arbitrary depth and flexible modeling of complex AAS structures. Root elements have a `NULL` value in `parent_sme_id`, while child elements point to their immediate parent. The `position` field is used to maintain the order of elements among siblings, which is important for lists. The `path_ltree` column leverages PostgreSQL's `ltree` extension to store the full path from the root to each element, enabling efficient hierarchical queries, such as retrieving all descendants or ancestors of a node. This design supports fast traversal, insertion, and querying of deeply nested submodel element trees, which is essential for representing the flexible and extensible nature of AAS submodels.

## Specialized Submodel Element Tables

Each AAS element type with additional data has its own table, always with a 1:1 relationship to `submodel_element`:
- `property_element`: Stores typed property values (text, numeric, boolean, time, datetime, reference)
- `multilanguage_property`, `multilanguage_property_value`: Multi-language support
- `blob_element`, `file_element`: Binary and file data
- `range_element`: Min/max values for ranges
- `reference_element`: Reference values
- `relationship_element`, `annotated_rel_annotation`: Relationships and annotations

## Collections and Lists

- `submodel_element_collection`: Marker table for collections
- `submodel_element_list`: Stores list-specific metadata (order relevance, element type, value type, semantic ID for list elements)

## Entity, Operation, and Event Tables

- `entity_element`, `entity_specific_asset_id`: Entity type and asset IDs
- `operation_element`, `operation_variable`: Operations and their variables (in/out/inout)
- `basic_event_element`: Event elements with state, direction, and timing
- `capability_element`: Marker for capability elements

## Qualifiers

- `qualifier`: Qualifiers can be attached to any submodel element, with typed value fields and kind/type metadata

## Indexes and Performance

- **GIN/Trigram Indexes**: These indexes use PostgreSQL's Generalized Inverted Index (GIN) with the `pg_trgm` extension to enable fast, case-insensitive, and fuzzy text search on large text fields such as `value_text` and `key_value`. This is especially useful for searching and filtering submodel elements or properties by partial matches or similar strings.
- **GIST/Ltree Indexes**: The Generalized Search Tree (GIST) index, combined with the `ltree` extension, allows for highly efficient querying of hierarchical data stored in the `path_ltree` column. This enables rapid retrieval of all descendants, ancestors, or subtrees within the submodel element hierarchy, which is critical for navigating complex AAS structures.
- **Partial Indexes**: Partial indexes are created on value columns (such as numeric, date, or boolean fields) but only for rows matching specific criteria (e.g., a certain data type). This reduces index size and improves query performance by indexing only the relevant subset of data, making lookups for specific value types much faster.
- **Unique Constraints**: Unique constraints on combinations like `(submodel_id, parent_sme_id, id_short)` and `(submodel_id, parent_sme_id, position)` ensure that each submodel element has a unique short identifier and position among its siblings. This prevents data inconsistencies and supports reliable navigation and manipulation of the element tree.

## Design Rationale

- **Normalization**: References and semantic keys are normalized for reuse and efficient lookup.
- **Tree Structure**: All submodel elements are stored in a single table with a parent-child relationship, supporting arbitrary nesting and fast path queries.
- **Specialization**: Each AAS element type with extra data has its own table, linked 1:1 to `submodel_element`.
- **Extensibility**: The schema is designed to be extensible for future AAS versions and custom elements.
- **Performance**: Indexes and partial indexes are used to optimize common queries, especially for text and hierarchy.

---

For a full list of tables and fields, see [entities.md](./entities.md). For diagrams, see [relationships.md](./relationships.md).

For questions about the AAS standard, refer to the IDTA documentation. BaSyx implements this standard as a server platform.
