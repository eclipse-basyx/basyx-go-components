# Indexes & Performance

The schema uses several indexes to optimize query performance, especially for text search and hierarchical queries.

## Key Indexes
- **GIN/Trigram Indexes**: For fast text search on fields like `value_text`, `key_value`, `idshort_path` etc.
- **Partial Indexes**: On value columns, filtered by type (e.g., numeric, date, boolean).

## Example Index Definitions
```sql
CREATE INDEX ix_prop_text_trgm ON property_element USING GIN (value_text gin_trgm_ops) WHERE value_type = 'xs:string';
CREATE INDEX ix_sme_path_gin ON submodel_element USING GIN (idshort_path gin_trgm_ops);
```

For more details, see the schema file.
