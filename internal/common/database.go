/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

//nolint:all
package common

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
)

const (
	CURRENT_DATABASE_VERSION = "v1.1.5"
	cleanSchemaState         = "clean"
)

// NewDatabaseConnection establishes a PostgreSQL database connection.
//
// This function creates a database connection pool with optimized settings for high-concurrency
// applications. Database schema initialization is handled by the BaSyx configuration service.
//
// Connection pool settings:
//   - MaxOpenConns: 50 (maximum concurrent connections)
//   - MaxIdleConns: 25 (maximum idle connections in pool)
//   - ConnMaxLifetime: 5 minutes (connection recycling interval)
//
// Parameters:
//   - dsn: PostgreSQL Data Source Name (connection string)
//     Format: "postgres://user:password@host:port/dbname?sslmode=disable"
//
// Returns:
//   - *sql.DB: Configured database connection pool
//   - error: Error if connection fails
//
// Example:
//
//	dsn := "postgres://admin:password@localhost:5432/basyx_db?sslmode=disable"
//	db, err := NewDatabaseConnection(dsn)
//	if err != nil {
//	    log.Fatal("Database connection failed:", err)
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
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// ValidateSchemaVersion checks whether basyxsystem is clean and matches the expected schema version.
// Returns an error if the state/version is missing, unreadable, dirty, or does not match.
func ValidateSchemaVersion(db *sql.DB, expectedVersion string) error {
	if db == nil {
		return fmt.Errorf("DB-CHECKVER-NILDB database handle is nil")
	}
	trimmedExpected := strings.TrimSpace(expectedVersion)
	if trimmedExpected == "" {
		return fmt.Errorf("DB-CHECKVER-NOEXPECTED expected version is empty")
	}

	query, _, err := goqu.Dialect("postgres").
		From(goqu.T("basyxsystem")).
		Select(goqu.C("schema_version"), goqu.C("state")).
		Order(goqu.C("identifier").Asc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return fmt.Errorf("DB-CHECKVER-BUILDQUERY failed to build version query: %w", err)
	}

	var actualVersion string
	var schemaState string
	err = db.QueryRow(query).Scan(&actualVersion, &schemaState)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("DB-CHECKVER-NOVERSIONROW basyxsystem has no version row")
		}
		_, _ = fmt.Println("[ERROR] It seems that the BaSyx Configuration Service is missing or was not started before. Please see the wiki (User Documentation) on how to integrate it into your setup")
		_, _ = fmt.Println("[ERROR] If the BaSyx Configuration Service was started before - check the database connection of the service and make sure it exited successfully")
		return fmt.Errorf("DB-CHECKVER-READFAIL failed to read schema version: %w", err)
	}

	if strings.TrimSpace(schemaState) != cleanSchemaState {
		return fmt.Errorf(
			"DB-CHECKVER-DIRTYSTATE expected schema state %q but found %q",
			cleanSchemaState,
			strings.TrimSpace(schemaState),
		)
	}

	if strings.TrimSpace(actualVersion) != trimmedExpected {
		return fmt.Errorf(
			"DB-CHECKVER-MISMATCH expected schema version %q but found %q",
			trimmedExpected,
			strings.TrimSpace(actualVersion),
		)
	}

	return nil
}

// ValidateSchemaVersionByDSN opens a temporary database connection and validates the schema version.
func ValidateSchemaVersionByDSN(dsn string, expectedVersion string) error {
	db, err := NewDatabaseConnection(dsn)
	if err != nil {
		return fmt.Errorf("DB-CHECKVER-CONNECTFAIL failed to connect while validating version: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	return ValidateSchemaVersion(db, expectedVersion)
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
