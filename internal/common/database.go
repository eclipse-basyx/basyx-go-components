//nolint:all
package common

import (
	"database/sql"
	"fmt"
	"os"
	"time"
)

// InitializeDatabase establishes a PostgreSQL database connection with optional schema initialization.
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
//	db, err := InitializeDatabase(dsn, "schema/basyx_schema.sql")
//	if err != nil {
//	    log.Fatal("Database initialization failed:", err)
//	}
//	defer db.Close()
func InitializeDatabase(dsn string, schemaFilePath string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	// Set Max Connection
	db.SetMaxOpenConns(500)
	db.SetMaxIdleConns(500)
	db.SetConnMaxLifetime(time.Minute * 5)

	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if schemaFilePath == "" {
		fmt.Println("No SQL Schema passed - skipping schema loading.")
		return db, nil
	}
	queryString, fileError := os.ReadFile(schemaFilePath)

	if fileError != nil {
		return nil, fileError
	}

	_, dbError := db.Exec(string(queryString))

	if dbError != nil {
		return nil, dbError
	}
	return db, nil
}
