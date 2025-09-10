# Entity Reference

This section describes the main tables (entities) in the BaSyx database schema.

## Table: `submodel`
- **id**: TEXT, primary key
- **id_short**: TEXT
- **category**: TEXT
- **kind**: modelling_kind (enum)
- **semantic_id**: BIGINT, references `reference(id)`
- **model_type**: TEXT (default: 'Submodel')

## Table: `submodel_element`
- **id**: BIGSERIAL, primary key
- **submodel_id**: TEXT, references `submodel(id)`
- **parent_sme_id**: BIGINT, references `submodel_element(id)`
- **position**: INTEGER (for list/collection order)
- **id_short**: TEXT
- **category**: TEXT
- **model_type**: aas_submodel_elements (enum)
- **semantic_id**: BIGINT, references `reference(id)`
- **path_ltree**: LTREE (hierarchical path)

## Table: `reference`
- **id**: BIGSERIAL, primary key
- **type**: reference_types (enum)

## Table: `reference_key`
- **id**: BIGSERIAL, primary key
- **reference_id**: BIGINT, references `reference(id)`
- **position**: INTEGER (array index)
- **type**: TEXT
- **value**: TEXT

## Table: `property_element`
- **id**: BIGINT, primary key, references `submodel_element(id)`
- **value_type**: data_type_def_xsd (enum)
- **value_text/value_num/value_bool/value_time/value_datetime/value_id**: Various value columns for typed property values

## Table: `submodel_element_collection`
- **id**: BIGINT, primary key, references `submodel_element(id)`

## Table: `submodel_element_list`
- **id**: BIGINT, primary key, references `submodel_element(id)`
- **order_relevant**: BOOLEAN
- **semantic_id_list_element**: BIGINT, references `reference(id)`
- **type_value_list_element**: aas_submodel_elements (enum)
- **value_type_list_element**: data_type_def_xsd (enum)

...and more. See the schema for all tables.

For relationships and diagrams, see [Relationships & Diagrams](./relationships.md).
