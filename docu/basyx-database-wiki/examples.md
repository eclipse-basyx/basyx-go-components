# Examples

> **Note**: The following examples provide developers with a comprehensive understanding of the database schema architecture and its inherent relational constraints. These practical demonstrations illustrate the proper implementation patterns and constraint dependencies within the BaSyx data model.

In the ``sql_examples`` folder you will find two things:
1) Demo SQL Queries
2) A docker-compose file with a PostgreSQL Server and Adminer as a Web Viewer for the PostgreSQL Database.

## Create a Submodel
```sql
INSERT INTO reference (id, type)
VALUES (1, 'ModelReference');

INSERT INTO reference_key (id, reference_id, position, type, value)
VALUES (1, 1, 0, 'Submodel', 'http://example.com/keys/123');

INSERT INTO submodel (id, id_short, category, kind, semantic_id, model_type)
VALUES ('http://iese.fraunhofer.de/id/sm/DemoSubmodel', 'DemoSubmodel', 'DemoCategory', 'Instance', 1, 'Submodel');
```

## Understanding Submodel Creation

The Submodel creation process demonstrates the fundamental pattern used throughout the BaSyx database schema. The creation follows a two-phase approach that establishes semantic meaning before creating the actual entity:

### Step-by-Step Breakdown

1. **Semantic Reference Setup** (`reference` and `reference_key` tables)
   - Creates a semantic identifier that defines the meaning and context of the Submodel
   - The `reference` table establishes the reference type ('ModelReference' in this case)
   - The `reference_key` table stores the actual semantic reference with its type and URI value
   - The `position` field allows for multiple reference keys if needed (here set to 0 for the first/only key)

2. **Submodel Entity Creation** (`submodel` table)
   - Creates the actual Submodel record with all its core properties
   - Links to the semantic reference via the `semantic_id` foreign key (references the `reference.id`)
   - Establishes the Submodel's identity through its unique `id` (typically a URI)

### Important Design Principles

**Semantic ID: Optional but Constraint-Dependent**: 
- Semantically, a Submodel can exist without a semantic ID (the `semantic_id` field can be NULL)
- However, if you want to assign a semantic meaning to your Submodel (as shown in this example), you **must** create the semantic reference first
- The database enforces referential integrity: any non-NULL value in `submodel.semantic_id` must correspond to an existing `reference.id`

**Foreign Key Dependencies**: The `submodel` table has an optional foreign key reference to the `reference` table:
- If `semantic_id` is NULL: No constraint violation occurs, and the Submodel exists without semantic meaning
- If `semantic_id` has a value: The referenced `reference.id` **must** exist before creating the Submodel
- This design allows flexibility while maintaining data integrity when semantic references are used

**Best Practice**: Even though semantic IDs are optional, it's recommended to always provide them for better interoperability and standardization in Industry 4.0 contexts.

## Create a Property on the root level of a Submodel
```sql
INSERT INTO reference (id, type)
VALUES (2, 'ModelReference');

INSERT INTO reference_key (id, reference_id, position, type, value)
VALUES (2, 2, 0, 'Submodel', 'http://iese.fraunhofer.de/id/sm/DemoSubmodel');

INSERT INTO submodel_element(id, submodel_id, parent_sme_id, position, id_short, category, model_type, semantic_id, idshort_path)
VALUES (2, 'http://iese.fraunhofer.de/id/sm/DemoSubmodel', NULL, 0, 'DemoProperty', 'DemoCategory', 'Property', 2, 'DemoProperty');

INSERT INTO property_element (id, value_type, value_text)
VALUES(1, 'xs:string', 'Demo Property Value');
```

## Understanding the Query Structure

The above SQL statements demonstrate the hierarchical relationship between database tables when creating a Property Submodel Element. The insertion process follows a specific order that respects the database constraints:

### Step-by-Step Breakdown

1. **Semantic ID Creation** (`reference` and `reference_key` tables)
   - First, we establish a semantic reference that identifies the meaning and context of our element
   - The `reference` table stores the reference type (e.g., 'ModelReference')
   - The `reference_key` table contains the actual reference details including type and value

2. **Base Submodel Element Creation** (`submodel_element` table)
   - This creates the fundamental structure that all submodel elements share
   - Contains core properties like `id_short`, `category`, `model_type`, and links to the semantic ID
   - Serves as the parent record for type-specific element data

3. **Property-Specific Data** (`property_element` table)
   - Stores the actual value and data type specific to Property elements
   - Links to the base submodel element via the shared `id` column

### Important Constraints

**Foreign Key Relationship**: The `property_element` table has a foreign key constraint that requires a corresponding record in the `submodel_element` table. This means:
- You **must** create the `submodel_element` record before creating the `property_element` record
- Both records must share the same `id` value to maintain referential integrity
- Attempting to create a `property_element` without its corresponding `submodel_element` will result in a constraint violation and the operation will fail