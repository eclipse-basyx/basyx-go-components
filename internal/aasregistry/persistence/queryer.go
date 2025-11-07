package aasregistrydatabase

import (
	"context"
	"database/sql"
)

// Queryer abstracts *sql.DB and *sql.Tx for read operations.
// Both *sql.DB and *sql.Tx implement these methods.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
