// Package helpers contains database helper functions for AAS persistence.
package helpers

import "database/sql"

// ResolveByID returns a value from a map keyed by FK ID,
// or a provided empty value if the FK is NULL or missing.
func ResolveByID[T any](
	id sql.NullInt64,
	byID map[int64]T,
	empty T,
) T {
	if !id.Valid {
		return empty
	}
	if v, ok := byID[id.Int64]; ok {
		return v
	}
	return empty
}
