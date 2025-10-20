# Reference Hierarchy Guide

## Overview

This guide explains how the BaSyx Go Components project fetches and rebuilds hierarchical reference structures from a normalized PostgreSQL database. References in the Asset Administration Shell (AAS) specification can contain nested `ReferredSemanticId` structures, creating tree-like hierarchies that require special handling during database retrieval and object reconstruction.

## Table of Contents

- [Background: Why Reference Hierarchies Matter](#background-why-reference-hierarchies-matter)
- [Database Structure](#database-structure)
- [The Challenge](#the-challenge)
- [Solution Architecture](#solution-architecture)
- [Step-by-Step Process](#step-by-step-process)
- [Code Examples](#code-examples)
- [Best Practices](#best-practices)

---

## Background: Why Reference Hierarchies Matter

In the AAS specification, a **Reference** is a pointer to a semantic concept, element, or resource. References consist of:

1. **Type**: The reference type (e.g., `ExternalReference`, `ModelReference`)
2. **Keys**: An ordered list of keys that form a path to the referenced element
3. **ReferredSemanticId**: An optional nested reference that provides additional semantic context

The `ReferredSemanticId` field allows references to point to other references, creating a **hierarchical tree structure**. For example:

```
Reference (Root)
├── Key[0]: GlobalReference, "https://example.com/concept"
├── Key[1]: ConceptDescription, "0173-1#01-ABC123#001"
└── ReferredSemanticId (Child Reference)
    ├── Key[0]: ExternalReference, "https://example.com/parent-concept"
    └── ReferredSemanticId (Grandchild Reference)
        └── Key[0]: GlobalReference, "https://example.com/grandparent-concept"
```

This hierarchy can be arbitrarily deep, with each `ReferredSemanticId` potentially containing its own `ReferredSemanticId`.

---

## Database Structure

### Normalized Reference Storage

References are stored in a normalized format across multiple tables:

#### 1. `reference` Table
Stores reference metadata and hierarchy information.

```sql
CREATE TABLE reference (
    id BIGSERIAL PRIMARY KEY,
    type reference_types NOT NULL,
    parentreference BIGINT,      -- Points to parent reference in hierarchy
    rootreference BIGINT          -- Points to root reference in hierarchy
);
```

**Key columns:**
- `id`: Unique identifier for the reference
- `type`: The reference type (`ExternalReference`, `ModelReference`, etc.)
- `parentreference`: Database ID of the immediate parent reference (NULL for root references)
- `rootreference`: Database ID of the root reference in the hierarchy (same as `id` for root references)

#### 2. `reference_key` Table
Stores the individual keys that make up a reference.

```sql
CREATE TABLE reference_key (
    id BIGSERIAL PRIMARY KEY,
    reference_id BIGINT REFERENCES reference(id),
    position INTEGER NOT NULL,
    type TEXT NOT NULL,
    value TEXT NOT NULL
);
```

**Key columns:**
- `reference_id`: Foreign key to the `reference` table
- `position`: Order of the key within the reference (0-indexed)
- `type`: Key type (e.g., `Submodel`, `GlobalReference`, `ConceptDescription`)
- `value`: The actual key value (URL, identifier, etc.)

### Example: Hierarchical References in Database

For a three-level reference hierarchy:

| reference.id | type              | parentreference | rootreference |
|--------------|-------------------|-----------------|---------------|
| 100          | ExternalReference | NULL            | NULL           |
| 101          | ModelReference    | 100             | 100           |
| 102          | ExternalReference | 101             | 100           |

| reference_key.id | reference_id | position | type              | value                          |
|------------------|--------------|----------|-------------------|--------------------------------|
| 1                | 100          | 0        | GlobalReference   | https://example.com/root       |
| 2                | 101          | 0        | ConceptDescription| 0173-1#01-ABC123#001           |
| 3                | 102          | 0        | ExternalReference | https://example.com/grandparent|

---

## The Challenge

### Problem 1: SQL Query Results are Flat

SQL queries return **flat rows**, even when using JOINs. For a reference hierarchy with multiple levels and multiple keys per reference, the database will return **multiple rows** that need to be:

1. **Grouped by reference ID** to collect all keys for each reference
2. **Organized hierarchically** to rebuild the parent-child relationships
3. **Deduplicated** to avoid creating duplicate keys or references when processing multiple rows

### Problem 2: JSON Aggregation Complexity

The solution uses PostgreSQL's JSON aggregation functions to bundle related data:

```sql
jsonb_agg(DISTINCT jsonb_build_object(
    'reference_id', ref.id, 
    'reference_type', ref.type,
    'parentReference', ref.parentreference,
    'rootReference', ref.rootreference,
    'key_id', rk.id,
    'key_type', rk.type,
    'key_value', rk.value
))
```

This produces a JSON array containing all reference and key data, but the hierarchical structure must still be **rebuilt in application code**.

---

## Solution Architecture

The solution uses a **two-phase approach**:

### Phase 1: Parsing and Initial Structure Creation
During database row processing:
1. Parse JSON arrays into typed Go structures (`ReferenceRow`, `ReferredReferenceRow`)
2. Create `Reference` objects and `ReferenceBuilder` instances for each unique reference
3. Add keys to each reference
4. Track parent-child relationships in a builder map

### Phase 2: Hierarchy Reconstruction
After all rows are processed:
1. Iterate through all `ReferenceBuilder` instances
2. Link child references to their parent's `ReferredSemanticId` field
3. Build the complete nested tree structure

---

## Step-by-Step Process

### Step 1: Execute SQL Query with JSON Aggregation

The query aggregates reference data as JSON arrays. Here's a simplified example for semantic IDs:

```sql
SELECT 
    s.id AS submodel_id,
    -- Root references with their keys
    COALESCE(
        (SELECT jsonb_agg(DISTINCT jsonb_build_object(
            'reference_id', r.id, 
            'reference_type', r.type,
            'key_id', rk.id, 
            'key_type', rk.type, 
            'key_value', rk.value
        ))
        FROM reference r
        LEFT JOIN reference_key rk ON rk.reference_id = r.id
        WHERE r.id = s.semantic_id),
        '[]'::jsonb
    ) AS semantic_id,
    -- Child references (ReferredSemanticIds) with hierarchy info
    COALESCE(
        (SELECT jsonb_agg(DISTINCT jsonb_build_object(
            'reference_id', ref.id,
            'reference_type', ref.type,
            'parentReference', ref.parentreference,
            'rootReference', ref.rootreference,
            'key_id', rk.id,
            'key_type', rk.type,
            'key_value', rk.value
        ))
        FROM reference ref
        LEFT JOIN reference_key rk ON rk.reference_id = ref.id
        WHERE ref.rootreference = s.semantic_id 
          AND ref.id != s.semantic_id),
        '[]'::jsonb
    ) AS referred_semantic_ids
FROM submodel s;
```

**Key Points:**
- Root references are queried directly by ID
- Child references are found by matching `rootreference` field
- `COALESCE` ensures empty arrays instead of NULL
- `DISTINCT` prevents duplicate entries in aggregation

### Step 2: Parse Root References

When processing each database row:

```go
// Create a map to track all reference builders
referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)

// Parse root references (e.g., semantic IDs)
semanticId, err := builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
if err != nil {
    return nil, err
}

// Assign to submodel if exactly one reference exists
if len(semanticId) == 1 {
    submodel.SemanticId = semanticId[0]
}
```

**What `ParseReferences` does:**

1. **Unmarshals JSON** into `[]ReferenceRow` structures
2. **Groups rows by `reference_id`** to collect all keys for each reference
3. **Creates `Reference` objects** and `ReferenceBuilder` instances
4. **Stores builders in the map** using database ID as key
5. **Returns unique references** (one per `reference_id`)

Example of what happens internally:

```go
// Input JSON:
[
    {"reference_id": 100, "reference_type": "ExternalReference", "key_id": 1, "key_type": "GlobalReference", "key_value": "https://example.com"},
    {"reference_id": 100, "reference_type": "ExternalReference", "key_id": 2, "key_type": "ConceptDescription", "key_value": "0173-1#01-ABC123#001"}
]

// Result:
// - One Reference object with ID 100
// - Two keys added to that reference
// - One ReferenceBuilder in referenceBuilderRefs[100]
```

### Step 3: Parse Referred References (Child References)

After parsing root references, parse their children:

```go
// Parse ReferredSemanticIds (child references)
err = builders.ParseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)
if err != nil {
    return nil, err
}
```

**What `ParseReferredReferences` does:**

1. **Unmarshals JSON** into `[]ReferredReferenceRow` structures
2. **Validates** that parent references exist in the builder map
3. **Creates child references** and adds them to the appropriate builder
4. **Tracks hierarchy** using `parentReference` and `rootReference` IDs
5. **Adds keys** to each child reference

Example flow:

```go
// Input JSON:
[
    {
        "reference_id": 101,
        "reference_type": "ModelReference",
        "parentReference": 100,    // Parent is reference 100
        "rootReference": 100,       // Root is also reference 100
        "key_id": 3,
        "key_type": "ConceptDescription",
        "key_value": "0173-1#01-XYZ456#001"
    }
]

// Process:
// 1. Look up parent builder: referenceBuilderRefs[100]
// 2. Create new child reference with ID 101
// 3. Add child to parent's builder
// 4. Add key to child reference
// 5. Store child's builder in referenceBuilderRefs[101]
```

### Step 4: Build Nested Structure

After processing **all rows**, build the complete hierarchy:

```go
// Build nested structures for all references
for _, referenceBuilder := range referenceBuilderRefs {
    referenceBuilder.BuildNestedStructure()
}
```

**What `BuildNestedStructure` does:**

1. **Iterates** through all child references tracked by the builder
2. **Finds parent references** using the parent ID
3. **Links children to parents** by setting the `ReferredSemanticId` field
4. **Recursively processes** all levels of the hierarchy

Example transformation:

```go
// Before BuildNestedStructure:
Reference (ID 100)
    Keys: [...]
    ReferredSemanticId: nil  // Not yet linked

Reference (ID 101)  // Exists separately
    Keys: [...]
    Parent: 100

Reference (ID 102)  // Exists separately
    Keys: [...]
    Parent: 101

// After BuildNestedStructure:
Reference (ID 100)
    Keys: [...]
    ReferredSemanticId: Reference (ID 101)
        Keys: [...]
        ReferredSemanticId: Reference (ID 102)
            Keys: [...]
            ReferredSemanticId: nil
```

---

## Code Examples

### Example 1: Simple Reference with One Level

```go
// Step 1: Create builder map
referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)

// Step 2: Parse root reference
semanticIds, _ := builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
submodel.SemanticId = semanticIds[0]

// Step 3: Parse one level of referred references
builders.ParseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)

// Step 4: Build hierarchy
for _, builder := range referenceBuilderRefs {
    builder.BuildNestedStructure()
}

// Result: submodel.SemanticId now contains nested ReferredSemanticId
```

### Example 2: Multiple Supplemental Semantic IDs

```go
// Supplemental semantic IDs can have multiple root references
supplementalSemanticIds, _ := builders.ParseReferences(
    row.SupplementalSemanticIds, 
    referenceBuilderRefs,
)

// Assign all to submodel
submodel.SupplementalSemanticIds = supplementalSemanticIds

// Parse referred references for ALL supplemental semantic IDs
builders.ParseReferredReferences(
    row.SupplementalReferredSemIds, 
    referenceBuilderRefs,
)

// Build hierarchy (affects all references in the map)
for _, builder := range referenceBuilderRefs {
    builder.BuildNestedStructure()
}
```

### Example 3: Complete Submodel Processing

Here's how references are processed in the actual codebase:

```go
func getSubmodels(db *sql.DB, submodelIdFilter string) ([]*gen.Submodel, error) {
    var result []*gen.Submodel
    
    // Step 1: Create shared builder map
    referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
    
    // Execute query
    rows, err := getSubmodelDataFromDbWithJSONQuery(db, submodelIdFilter)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    // Step 2: Process each row
    for rows.Next() {
        var row builders.SubmodelRow
        // ... scan row data ...
        
        submodel := &gen.Submodel{
            Id: row.Id,
            // ... other fields ...
        }

        // Step 3: Parse semantic ID (root reference)
        if isArrayNotEmpty(row.SemanticId) {
            semanticId, err := builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
            if err != nil {
                return nil, err
            }
            if len(semanticId) == 1 {
                submodel.SemanticId = semanticId[0]
                // Parse referred references for this semantic ID
                builders.ParseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)
            }
        }

        // Step 4: Parse supplemental semantic IDs
        if isArrayNotEmpty(row.SupplementalSemanticIds) {
            supplementalSemanticIds, err := builders.ParseReferences(
                row.SupplementalSemanticIds, 
                referenceBuilderRefs,
            )
            if err != nil {
                return nil, err
            }
            if len(supplementalSemanticIds) > 0 {
                submodel.SupplementalSemanticIds = supplementalSemanticIds
                // Parse referred references for supplemental IDs
                builders.ParseReferredReferences(
                    row.SupplementalReferredSemIds, 
                    referenceBuilderRefs,
                )
            }
        }

        result = append(result, submodel)
    }

    // Step 5: Build nested structures for ALL references
    for _, referenceBuilder := range referenceBuilderRefs {
        referenceBuilder.BuildNestedStructure()
    }

    return result, nil
}
```

---

## Best Practices

### 1. Use a Shared Builder Map

**Always use the same `referenceBuilderRefs` map** throughout the entire parsing process for a single entity (e.g., one submodel or all submodels in a query). This ensures:

- References are not duplicated
- Hierarchy relationships are correctly tracked
- The final `BuildNestedStructure()` call can process all references

```go
// ✅ CORRECT: Shared map
referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
builders.ParseReferences(row.SupplementalSemanticIds, referenceBuilderRefs)
builders.ParseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)

// ❌ WRONG: Separate maps
map1 := make(map[int64]*builders.ReferenceBuilder)
map2 := make(map[int64]*builders.ReferenceBuilder)
builders.ParseReferences(row.SemanticId, map1)
builders.ParseReferences(row.SupplementalSemanticIds, map2)  // Won't work!
```

### 2. Parse Root References Before Referred References

**Always call `ParseReferences()` before `ParseReferredReferences()`** for related references. The referred references need their parent references to exist in the builder map first.

```go
// ✅ CORRECT: Root first, then children
semanticId, _ := builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
builders.ParseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)

// ❌ WRONG: Children before root
builders.ParseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)  // Will fail!
semanticId, _ := builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
```

### 3. Build Nested Structure After All Parsing

**Call `BuildNestedStructure()` only after** all references and referred references have been parsed. Do this **once per builder** at the end of processing.

```go
// ✅ CORRECT: Build at the end
for rows.Next() {
    // ... parse references ...
}
for _, builder := range referenceBuilderRefs {
    builder.BuildNestedStructure()  // Called after all rows processed
}

// ❌ WRONG: Building too early
for rows.Next() {
    // ... parse references ...
    for _, builder := range referenceBuilderRefs {
        builder.BuildNestedStructure()  // Called too early!
    }
}
```

### 4. Check Array Emptiness

**Always check if JSON arrays are not empty** before parsing to avoid unnecessary processing and potential errors.

```go
// ✅ CORRECT: Check before parsing
if isArrayNotEmpty(row.SemanticId) {
    semanticId, err := builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
    // ... process ...
}

// Helper function
func isArrayNotEmpty(data json.RawMessage) bool {
    return len(data) > 0 && string(data) != "null"
}
```

### 5. Validate Reference Counts

**Validate reference counts** according to AAS specification rules:

```go
// Semantic ID: Exactly one reference expected
if len(semanticId) == 1 {
    submodel.SemanticId = semanticId[0]
}

// Supplemental Semantic IDs: Zero or more references
if len(supplementalSemanticIds) > 0 {
    submodel.SupplementalSemanticIds = supplementalSemanticIds
}
```

### 6. Handle Errors Gracefully

**Check for errors** at each parsing step and propagate them appropriately:

```go
semanticId, err := builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
if err != nil {
    return nil, fmt.Errorf("error parsing semantic ID: %w", err)
}

err = builders.ParseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)
if err != nil {
    return nil, fmt.Errorf("error parsing referred semantic IDs: %w", err)
}
```

### 7. Use SQL COALESCE for Clean JSON

**Use COALESCE in SQL queries** to ensure empty arrays instead of NULL values:

```sql
COALESCE(
    (SELECT jsonb_agg(...) FROM ...),
    '[]'::jsonb
) AS semantic_id
```

This simplifies Go code by avoiding nil checks for JSON data.

---

## Common Pitfalls

### Pitfall 1: Not Building Nested Structure

**Problem:** Forgetting to call `BuildNestedStructure()` results in references being created but not linked together.

**Symptom:** All references exist, but `ReferredSemanticId` fields are `nil`.

**Solution:** Always call `BuildNestedStructure()` on all builders after parsing.

### Pitfall 2: Creating New Maps Per Reference Type

**Problem:** Creating separate builder maps for different reference types breaks hierarchy tracking.

**Symptom:** Errors about missing parent references, or incomplete hierarchies.

**Solution:** Use one shared map for all reference types within an entity.

### Pitfall 3: Incorrect SQL Join Logic

**Problem:** Using `INNER JOIN` instead of `LEFT JOIN` for reference keys can exclude references without keys.

**Symptom:** Missing references in the result set.

**Solution:** Use `LEFT JOIN` for reference keys to include references even if they have no keys (rare but possible).

### Pitfall 4: Missing DISTINCT in JSON Aggregation

**Problem:** Not using `DISTINCT` in `jsonb_agg()` can create duplicate entries.

**Symptom:** Duplicate keys in references or excessive data.

**Solution:** Always use `jsonb_agg(DISTINCT ...)` in queries.

---

## Performance Considerations

### 1. Query Optimization

The current implementation uses **single-query JSON aggregation** which is more efficient than multiple queries:

- **One database round-trip** instead of N+1 queries
- **JSON aggregation** happens in PostgreSQL (optimized C code)
- **Result set size** is minimized by aggregating early

### 2. Memory Management

**Pre-sizing slices** based on total count improves memory efficiency:

```go
if result == nil {
    result = make([]*gen.Submodel, 0, row.TotalSubmodels)
}
```

### 3. Deduplication

**Database IDs are used for deduplication** to prevent processing the same key or reference multiple times when iterating through rows.

---

## Summary

The reference hierarchy rebuilding process follows this pattern:

1. **Query**: Fetch flattened reference data with JSON aggregation
2. **Parse Root**: Create root `Reference` objects and builders
3. **Parse Children**: Create child references and link to parents
4. **Build Hierarchy**: Connect all references into nested tree structure
5. **Use**: Fully constructed hierarchies are ready for use in AAS models

**Key Takeaways:**
- Use a shared builder map for all references in an entity
- Parse root references before referred references
- Build nested structure after all parsing is complete
- Validate reference counts and handle errors appropriately
- Use JSON aggregation in SQL for efficiency

This architecture efficiently handles complex reference hierarchies while maintaining clean separation between database concerns and business logic.
