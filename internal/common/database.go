//nolint:all
package common

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	CURRENT_DATABASE_VERSION = "v1.0.1"
)

// NewDatabaseConnection establishes a PostgreSQL database connection with optional schema initialization.
//
// This function creates a database connection pool with optimized settings for high-concurrency
// applications. It supports automatic schema loading from SQL files for database initialization.
//
// Connection pool settings:
//   - MaxOpenConns: 500 (maximum concurrent connections)
//   - MaxIdleConns: 500 (maximum idle connections in pool)
//   - ConnMaxLifetime: 5 minutes (connection recycling interval)
//
// Parameters:
//   - dsn: PostgreSQL Data Source Name (connection string)
//     Format: "postgres://user:password@host:port/dbname?sslmode=disable"
//   - schemaFilePath: Path to SQL schema file for initialization.
//     If empty, schema loading is skipped.
//
// Returns:
//   - *sql.DB: Configured database connection pool
//   - error: Error if connection fails or schema loading fails
//
// Example:
//
//	dsn := "postgres://admin:password@localhost:5432/basyx_db?sslmode=disable"
//	db, err := NewDatabaseConnection(dsn, "schema/basyx_schema.sql")
//	if err != nil {
//	    log.Fatal("Database initialization failed:", err)
//	}
//	defer db.Close()
func NewDatabaseConnection(dsn string) (*sql.DB, error) {
	encodedDSN := NormalizePostgresDSN(dsn)
	db, err := sql.Open("postgres", encodedDSN)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(time.Minute * 5)
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

// ValidateDatabaseVersion checks whether the database version in basyxsystem matches the expected service version.
// Returns an error if the version is missing, unreadable, or does not match.
func ValidateDatabaseVersion(db *sql.DB, expectedVersion string) error {
	if db == nil {
		return fmt.Errorf("DB-CHECKVER-NILDB database handle is nil")
	}
	trimmedExpected := strings.TrimSpace(expectedVersion)
	if trimmedExpected == "" {
		return fmt.Errorf("DB-CHECKVER-NOEXPECTED expected version is empty")
	}

	row := db.QueryRow(`
		SELECT database_version
		FROM basyxsystem
		ORDER BY identifier ASC
		LIMIT 1
	`)

	var actualVersion string
	err := row.Scan(&actualVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("DB-CHECKVER-NOVERSIONROW basyxsystem has no version row")
		}
		return fmt.Errorf("DB-CHECKVER-READFAIL failed to read database version: %w", err)
	}

	if strings.TrimSpace(actualVersion) != trimmedExpected {
		return fmt.Errorf(
			"DB-CHECKVER-MISMATCH expected database version %q but found %q",
			trimmedExpected,
			strings.TrimSpace(actualVersion),
		)
	}

	return nil
}

// ValidateDatabaseVersionByDSN opens a temporary database connection and validates the schema version.
func ValidateDatabaseVersionByDSN(dsn string, expectedVersion string) error {
	db, err := NewDatabaseConnection(dsn)
	if err != nil {
		return fmt.Errorf("DB-CHECKVER-CONNECTFAIL failed to connect while validating version: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	return ValidateDatabaseVersion(db, expectedVersion)
}

func StartTransaction(db *sql.DB) (*sql.Tx, func(*error), error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	cleanup := func(txErr *error) {
		if txErr != nil {
			_ = tx.Rollback()
		}
	}
	return tx, cleanup, nil
}
