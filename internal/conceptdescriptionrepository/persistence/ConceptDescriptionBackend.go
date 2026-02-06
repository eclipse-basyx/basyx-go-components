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
// Author: Jannik Fried (Fraunhofer IESE)

// Package persistence contains the implementation of the Concept Description Repository API service's persistence layer,
// which is responsible for storing and retrieving concept descriptions. It provides an interface for interacting with
// the underlying database and abstracts away the details of data storage from the rest of the application.
package persistence

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// ConceptDescriptionBackend is the struct that implements the persistence layer for the Concept Description Repository API service.
// It contains a reference to the database connection pool and provides methods for storing and retrieving concept descriptions.
type ConceptDescriptionBackend struct {
	db *sql.DB
}

// NewConceptDescriptionBackend creates a new instance of ConceptDescriptionBackend with the given database connection parameters.
// It establishes a connection to the database and returns an error if the connection fails.
//
// Parameters:
// - connectionString: The connection string for the PostgreSQL database.
// - maxOpenConnections: The maximum number of open connections to the database.
// - maxIdleConnections: The maximum number of idle connections in the connection pool.
// - connMaxLifetimeMinutes: The maximum lifetime of a connection in minutes.
//
// Returns:
// - A pointer to a ConceptDescriptionBackend instance if the connection is successful.
// - An error if the connection fails or if there is an issue with the database configuration.
func NewConceptDescriptionBackend(dsn string, maxOpenConnections int32, maxIdleConnections int, connMaxLifetimeMinutes int, databaseSchema string) (*ConceptDescriptionBackend, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}
	if maxOpenConnections > 0 {
		db.SetMaxOpenConns(int(maxOpenConnections))
	}
	if maxIdleConnections > 0 {
		db.SetMaxIdleConns(maxIdleConnections)
	}
	if connMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(connMaxLifetimeMinutes) * time.Minute)
	}

	healthy, err := testDBConnection(db)
	if !healthy {
		_, _ = fmt.Printf("CDREPO-TESTDBCON-FAIL Failed to connect to database: %v\n", err)
		return nil, err
	}

	return &ConceptDescriptionBackend{db: db}, nil
}

func testDBConnection(db *sql.DB) (bool, error) {
	err := db.Ping()
	if err != nil {
		return false, err
	}
	return true, nil
}

func (b *ConceptDescriptionBackend) CreateConceptDescription(cd types.IConceptDescription) error {
	return nil
}

func (b *ConceptDescriptionBackend) GetConceptDescriptions(idShort *string, isCaseOf *string, dataSpecificationRef *string, limit int, cursor *string) ([]types.IConceptDescription, error) {
	return nil, nil
}

func (b *ConceptDescriptionBackend) GetConceptDescriptionByID(id string) (types.IConceptDescription, error) {
	return nil, nil
}

func (b *ConceptDescriptionBackend) PutConceptDescription(id string, cd types.IConceptDescription) error {
	return nil
}

func (b *ConceptDescriptionBackend) DeleteConceptDescription(id string) error {
	return nil
}
