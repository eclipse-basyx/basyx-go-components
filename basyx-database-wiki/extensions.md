# Extensions

The schema uses the following PostgreSQL extensions:

- **ltree**: For representing and querying hierarchical data (e.g., `path_ltree` in `submodel_element`).
- **pg_trgm**: For fast trigram-based text search (e.g., on `value_text`, `key_value`).

These extensions must be enabled in your database:

```sql
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
```
