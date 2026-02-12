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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // Postgres Driver for Goqu
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

// CreateConceptDescription inserts a new concept description into the database.
func (b *ConceptDescriptionBackend) CreateConceptDescription(cd types.IConceptDescription) error {
	var jsonable map[string]any
	var err error

	exists, err := doesConceptDescriptionExist(b.db, cd.ID())
	if err != nil {
		return common.NewInternalServerError("Failed to check of CD Existence CDREPO-CCD-ERREXIST")
	}
	if exists {
		return common.NewErrConflict("Concept description with the given ID already exists - use PUT for Replacement")
	}

	jsonable, err = jsonization.ToJsonable(cd)
	if err != nil {
		return common.NewErrBadRequest("Failed to convert concept description to jsonable CDREPO-CCD-TOJSONABLE")
	}

	var conceptDescriptionString string
	bytes, err := json.Marshal(jsonable)
	if err != nil {
		return common.NewErrBadRequest("Failed to jsonify concept description CDREPO-CCD-TOJSONSTRING")
	}

	conceptDescriptionString = string(bytes)

	// Insert the concept description into the database with goqu
	goquQuery, args, err := goqu.Insert("concept_description").Rows(
		goqu.Record{
			"id":       cd.ID(),
			"id_short": cd.IDShort(),
			"data":     conceptDescriptionString,
		},
	).ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build SQL query: %w", err)
	}

	_, err = b.db.Exec(goquQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to execute SQL query: %w", err)
	}

	return nil
}

// GetConceptDescriptions retrieves a paginated list of concept descriptions with optional filters.
func (b *ConceptDescriptionBackend) GetConceptDescriptions(idShort *string, _ /* isCaseOf */ *string, _ /* dataSpecificationRef */ *string, limit uint, cursor *string) ([]types.IConceptDescription, string, error) {
	if limit == 0 {
		limit = 100
	}

	peekLimit := limit + 1
	var conceptDescriptions []types.IConceptDescription
	nextCursor := ""

	query := goqu.From("concept_description").
		Select(goqu.C("id"), goqu.C("id_short"), goqu.C("data")).
		Order(goqu.I("id").Asc()).
		Limit(peekLimit)

	// if idShort != nil {
	// 	query = query.Where(goqu.Ex{"id_short": *idShort})
	// }
	// if isCaseOf != nil {
	// 	query = query.Where(goqu.Ex{"data->>'isCaseOf'": *isCaseOf})
	// }
	// if dataSpecificationRef != nil {
	// 	query = query.Where(goqu.Ex{"data->>'dataSpecificationRef'": *dataSpecificationRef})
	// }

	if idShort != nil && strings.TrimSpace(*idShort) != "" {
		query = query.Where(goqu.Ex{"id_short": strings.TrimSpace(*idShort)})
	}

	if cursor != nil && strings.TrimSpace(*cursor) != "" {
		query = query.Where(goqu.C("id").Gte(strings.TrimSpace(*cursor)))
	}

	sqlQuery, args, err := query.ToSQL()
	if err != nil {
		return nil, "", fmt.Errorf("CDREPO-GCDS-BUILDSQL failed to build SQL query: %w", err)
	}

	rows, err := b.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, "", fmt.Errorf("CDREPO-GCDS-EXECQUERY failed to execute SQL query: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_, _ = fmt.Printf("CDREPO-GCDS-CLOSEROWS failed to close rows: %v\n", closeErr)
		}
	}()

	readCount := uint(0)

	for rows.Next() {
		var id string
		var idShort string
		var data string
		if err := rows.Scan(&id, &idShort, &data); err != nil {
			return nil, "", fmt.Errorf("CDREPO-GCDS-SCANROW failed to scan row: %w", err)
		}

		if readCount == limit {
			nextCursor = id
			break
		}

		var jsonable map[string]any
		if err := json.Unmarshal([]byte(data), &jsonable); err != nil {
			return nil, "", fmt.Errorf("CDREPO-GCDS-UNMARSHAL failed to unmarshal JSON data: %w", err)
		}

		cd, err := jsonization.ConceptDescriptionFromJsonable(jsonable)
		if err != nil {
			return nil, "", fmt.Errorf("CDREPO-GCDS-FROMJSON failed to convert jsonable to concept description: %w", err)
		}

		conceptDescriptions = append(conceptDescriptions, cd)
		readCount++
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("CDREPO-GCDS-ROWSERR error iterating over rows: %w", err)
	}

	return conceptDescriptions, nextCursor, nil
}

// GetConceptDescriptionByID retrieves a concept description by its identifier.
func (b *ConceptDescriptionBackend) GetConceptDescriptionByID(id string) (types.IConceptDescription, error) {
	exists, err := doesConceptDescriptionExist(b.db, id)
	if err != nil {
		return nil, common.NewInternalServerError("Failed to check of CD Existence CDREPO-GCDBID-ERREXIST")
	}
	if !exists {
		return nil, common.NewErrNotFound("Concept description with the given ID does not exist")
	}

	var data string
	query, args, err := goqu.From("concept_description").
		Select("data").
		Where(goqu.Ex{"id": id}).
		Limit(1).
		ToSQL()
	if err != nil {
		return nil, fmt.Errorf("failed to build SQL query: %w", err)
	}

	err = b.db.QueryRow(query, args...).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to execute SQL query: %w", err)
	}

	var jsonable map[string]any
	if err := json.Unmarshal([]byte(data), &jsonable); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON data: %w", err)
	}

	cd, err := jsonization.ConceptDescriptionFromJsonable(jsonable)
	if err != nil {
		return nil, fmt.Errorf("failed to convert jsonable to concept description: %w", err)
	}

	return cd, nil
}

// PutConceptDescription updates or replaces the concept description with the given identifier.
func (b *ConceptDescriptionBackend) PutConceptDescription(id string, cd types.IConceptDescription) error {
	exists, err := doesConceptDescriptionExist(b.db, id)
	if err != nil {
		return err
	}

	if exists {
		err = b.DeleteConceptDescription(id)
		if err != nil {
			return err
		}
	}

	err = b.CreateConceptDescription(cd)
	return err
}

// DeleteConceptDescription removes a concept description by its identifier.
func (b *ConceptDescriptionBackend) DeleteConceptDescription(id string) error {
	delQuery, args, err := goqu.Delete("concept_description").Where(
		goqu.Ex{"id": id},
	).ToSQL()
	if err != nil {
		return err
	}
	_, err = b.db.Exec(delQuery, args...)
	return err
}

func doesConceptDescriptionExist(db *sql.DB, id string) (bool, error) {
	query, args, err := goqu.From("concept_description").
		Select(goqu.L("1")).
		Where(goqu.Ex{"id": id}).
		Limit(1).
		ToSQL()
	if err != nil {
		return false, fmt.Errorf("CDREPO-CDEXIST-BUILDSQL failed to build SQL query: %w", err)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return false, fmt.Errorf("CDREPO-CDEXIST-EXEC failed to execute SQL query: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_, _ = fmt.Printf("CDREPO-CDEXIST-CLOSEROWS failed to close rows: %v\n", closeErr)
		}
	}()

	return rows.Next(), nil
}
