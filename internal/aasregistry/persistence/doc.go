// Package aasregistrydatabase provides a PostgreSQL-backed persistence layer
// for the AAS Registry. It offers creation, retrieval, listing, replacement,
// and deletion of Asset Administration Shell (AAS) descriptors and their
// related entities (endpoints, specific asset IDs, extensions, and submodel
// descriptors). The package uses goqu to build SQL and database/sql for query
// execution, and applies cursor-based pagination where appropriate.
package aasregistrydatabase

