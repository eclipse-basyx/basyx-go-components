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
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // Postgres Driver for Goqu
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
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

	return NewConceptDescriptionBackendFromDB(db)
}

// NewConceptDescriptionBackendFromDB creates a new backend instance from an existing DB pool.
func NewConceptDescriptionBackendFromDB(db *sql.DB) (*ConceptDescriptionBackend, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("CDREPO-NEWFROMDB-NILDB database handle must not be nil")
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

func conceptDescriptionToJSONString(cd types.IConceptDescription) (string, error) {
	jsonable, err := jsonization.ToJsonable(cd)
	if err != nil {
		return "", common.NewErrBadRequest("CDREPO-CDJSON-TOJSONABLE failed to convert concept description to jsonable")
	}

	bytes, err := json.Marshal(jsonable)
	if err != nil {
		return "", common.NewErrBadRequest("CDREPO-CDJSON-MARSHAL failed to marshal concept description")
	}

	return string(bytes), nil
}

func (b *ConceptDescriptionBackend) createConceptDescriptionInTx(ctx context.Context, tx *sql.Tx, cd types.IConceptDescription) error {
	conceptDescriptionString, err := conceptDescriptionToJSONString(cd)
	if err != nil {
		return err
	}

	insertQuery, args, err := goqu.Insert("concept_description").Rows(
		goqu.Record{
			"id":       cd.ID(),
			"id_short": cd.IDShort(),
			"data":     conceptDescriptionString,
		},
	).ToSQL()
	if err != nil {
		return common.NewInternalServerError("CDREPO-CRTCD-BUILDSQL " + err.Error())
	}

	if _, err = tx.ExecContext(ctx, insertQuery, args...); err != nil {
		return common.NewInternalServerError("CDREPO-CRTCD-EXECSQL " + err.Error())
	}

	return nil
}

func (b *ConceptDescriptionBackend) deleteConceptDescriptionInTx(ctx context.Context, tx *sql.Tx, id string) error {
	delQuery, args, err := goqu.Delete("concept_description").Where(goqu.Ex{"id": id}).ToSQL()
	if err != nil {
		return common.NewInternalServerError("CDREPO-DELCD-BUILDSQL " + err.Error())
	}

	if _, err = tx.ExecContext(ctx, delQuery, args...); err != nil {
		return common.NewInternalServerError("CDREPO-DELCD-EXECSQL " + err.Error())
	}

	return nil
}

func conceptDescriptionExistsInTx(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	query, args, err := goqu.From("concept_description").
		Select(goqu.V(1)).
		Where(goqu.Ex{"id": id}).
		Limit(1).
		ToSQL()
	if err != nil {
		return false, common.NewInternalServerError("CDREPO-CDEXIST-BUILDSQL " + err.Error())
	}

	var existsMarker int
	scanErr := tx.QueryRowContext(ctx, query, args...).Scan(&existsMarker)
	if scanErr == nil {
		return true, nil
	}
	if errors.Is(scanErr, sql.ErrNoRows) {
		return false, nil
	}

	return false, common.NewInternalServerError("CDREPO-CDEXIST-EXECSQL " + scanErr.Error())
}

func (b *ConceptDescriptionBackend) checkConceptDescriptionVisibilityInTx(ctx context.Context, tx *sql.Tx, id string) (bool, bool, error) {
	exists, err := conceptDescriptionExistsInTx(ctx, tx, id)
	if err != nil {
		return false, false, err
	}
	if !exists {
		return false, false, nil
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return false, false, common.NewInternalServerError("CDREPO-ABACCHKCD-SHOULDENFORCE " + enforceErr.Error())
	}
	if !shouldEnforceFormula {
		return true, true, nil
	}

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootCD)
	if collectorErr != nil {
		return false, false, common.NewInternalServerError("CDREPO-ABACCHKCD-BADCOLLECTOR " + collectorErr.Error())
	}

	query := goqu.From("concept_description").
		Select(goqu.C("id")).
		Where(goqu.C("id").Eq(id)).
		Limit(1)

	query, addFormulaErr := auth.AddFormulaQueryFromContext(ctx, query, collector)
	if addFormulaErr != nil {
		return false, false, common.NewInternalServerError("CDREPO-ABACCHKCD-ADDFORMULA " + addFormulaErr.Error())
	}

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return false, false, common.NewInternalServerError("CDREPO-ABACCHKCD-BUILDSQL " + toSQLErr.Error())
	}

	var visibleID string
	scanErr := tx.QueryRowContext(ctx, sqlQuery, args...).Scan(&visibleID)
	if scanErr == nil {
		return true, true, nil
	}
	if errors.Is(scanErr, sql.ErrNoRows) {
		return true, false, nil
	}

	return false, false, common.NewInternalServerError("CDREPO-ABACCHKCD-EXECSQL " + scanErr.Error())
}

// CreateConceptDescription inserts a new concept description into the database.
func (b *ConceptDescriptionBackend) CreateConceptDescription(ctx context.Context, cd types.IConceptDescription) (err error) {
	tx, cleanup, err := common.StartTransaction(b.db)
	if err != nil {
		return common.NewInternalServerError("CDREPO-CRTCD-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	exists, err := conceptDescriptionExistsInTx(ctx, tx, cd.ID())
	if err != nil {
		return err
	}
	if exists {
		return common.NewErrConflict("Concept description with the given ID already exists - use PUT for Replacement")
	}

	if err = b.createConceptDescriptionInTx(ctx, tx, cd); err != nil {
		return err
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return common.NewInternalServerError("CDREPO-CRTCD-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		exists, visible, visErr := b.checkConceptDescriptionVisibilityInTx(ctx, tx, cd.ID())
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewInternalServerError("CDREPO-CRTCD-ABACCHECKMISSING created concept description not found before commit")
		}
		if !visible {
			return common.NewErrDenied("CDREPO-CRTCD-ABACDENIED created concept description is not accessible under ABAC constraints")
		}
	}

	if err = tx.Commit(); err != nil {
		return common.NewInternalServerError("CDREPO-CRTCD-COMMIT " + err.Error())
	}

	return nil
}

// GetConceptDescriptions retrieves a paginated list of concept descriptions with optional filters.
func (b *ConceptDescriptionBackend) GetConceptDescriptions(ctx context.Context, idShort *string, isCaseOf *string, dataSpecificationRef *string, limit uint, cursor *string) ([]types.IConceptDescription, string, error) {
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

	if idShort != nil && strings.TrimSpace(*idShort) != "" {
		query = query.Where(goqu.Ex{"id_short": strings.TrimSpace(*idShort)})
	}

	if isCaseOf != nil && strings.TrimSpace(*isCaseOf) != "" {
		query = query.Where(goqu.L(`EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(data->'isCaseOf', '[]'::jsonb)) AS is_case_of,
				 jsonb_array_elements(COALESCE(is_case_of->'keys', '[]'::jsonb)) AS key_item
			WHERE key_item->>'value' = ?
		)`, strings.TrimSpace(*isCaseOf)))
	}

	if dataSpecificationRef != nil && strings.TrimSpace(*dataSpecificationRef) != "" {
		query = query.Where(goqu.L(`EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(data->'embeddedDataSpecifications', '[]'::jsonb)) AS eds,
				 jsonb_array_elements(COALESCE(eds->'dataSpecification'->'keys', '[]'::jsonb)) AS key_item
			WHERE key_item->>'value' = ?
		)`, strings.TrimSpace(*dataSpecificationRef)))
	}

	if cursor != nil && strings.TrimSpace(*cursor) != "" {
		query = query.Where(goqu.C("id").Gte(strings.TrimSpace(*cursor)))
	}

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootCD)
	if collectorErr != nil {
		return nil, "", common.NewInternalServerError("CDREPO-GCDS-BADCOLLECTOR " + collectorErr.Error())
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return nil, "", common.NewInternalServerError("CDREPO-GCDS-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		var addFormulaErr error
		query, addFormulaErr = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if addFormulaErr != nil {
			return nil, "", common.NewInternalServerError("CDREPO-GCDS-ABACFORMULA " + addFormulaErr.Error())
		}
	}

	sqlQuery, args, err := query.ToSQL()
	if err != nil {
		return nil, "", fmt.Errorf("CDREPO-GCDS-BUILDSQL failed to build SQL query: %w", err)
	}

	rows, err := b.db.QueryContext(ctx, sqlQuery, args...)
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
		var identifier string
		var idShortValue sql.NullString
		var data string
		if scanErr := rows.Scan(&identifier, &idShortValue, &data); scanErr != nil {
			return nil, "", fmt.Errorf("CDREPO-GCDS-SCANROW failed to scan row: %w", scanErr)
		}

		if readCount == limit {
			nextCursor = identifier
			break
		}

		var jsonable map[string]any
		if unmarshalErr := json.Unmarshal([]byte(data), &jsonable); unmarshalErr != nil {
			return nil, "", fmt.Errorf("CDREPO-GCDS-UNMARSHAL failed to unmarshal JSON data: %w", unmarshalErr)
		}

		cd, fromJSONErr := jsonization.ConceptDescriptionFromJsonable(jsonable)
		if fromJSONErr != nil {
			return nil, "", fmt.Errorf("CDREPO-GCDS-FROMJSON failed to convert jsonable to concept description: %w", fromJSONErr)
		}

		conceptDescriptions = append(conceptDescriptions, cd)
		readCount++
	}

	if err = rows.Err(); err != nil {
		return nil, "", fmt.Errorf("CDREPO-GCDS-ROWSERR error iterating over rows: %w", err)
	}

	return conceptDescriptions, nextCursor, nil
}

// GetConceptDescriptionByID retrieves a concept description by its identifier.
func (b *ConceptDescriptionBackend) GetConceptDescriptionByID(ctx context.Context, id string) (types.IConceptDescription, error) {
	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootCD)
	if collectorErr != nil {
		return nil, common.NewInternalServerError("CDREPO-GCDBYID-BADCOLLECTOR " + collectorErr.Error())
	}

	query := goqu.From("concept_description").
		Select(goqu.C("data")).
		Where(goqu.C("id").Eq(id)).
		Limit(1)

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return nil, common.NewInternalServerError("CDREPO-GCDBYID-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		var addFormulaErr error
		query, addFormulaErr = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if addFormulaErr != nil {
			return nil, common.NewInternalServerError("CDREPO-GCDBYID-ABACFORMULA " + addFormulaErr.Error())
		}
	}

	sqlQuery, args, err := query.ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("CDREPO-GCDBYID-BUILDSQL " + err.Error())
	}

	var data string
	scanErr := b.db.QueryRowContext(ctx, sqlQuery, args...).Scan(&data)
	if scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("Concept description with the given ID does not exist")
		}
		return nil, common.NewInternalServerError("CDREPO-GCDBYID-EXECSQL " + scanErr.Error())
	}

	var jsonable map[string]any
	if err = json.Unmarshal([]byte(data), &jsonable); err != nil {
		return nil, common.NewInternalServerError("CDREPO-GCDBYID-UNMARSHAL " + err.Error())
	}

	cd, err := jsonization.ConceptDescriptionFromJsonable(jsonable)
	if err != nil {
		return nil, common.NewInternalServerError("CDREPO-GCDBYID-FROMJSON " + err.Error())
	}

	return cd, nil
}

// PutConceptDescription updates or replaces the concept description with the given identifier.
func (b *ConceptDescriptionBackend) PutConceptDescription(ctx context.Context, id string, cd types.IConceptDescription) (err error) {
	tx, cleanup, err := common.StartTransaction(b.db)
	if err != nil {
		return common.NewInternalServerError("CDREPO-PUTCD-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	existingExists, existsErr := conceptDescriptionExistsInTx(ctx, tx, id)
	if existsErr != nil {
		return existsErr
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return common.NewInternalServerError("CDREPO-PUTCD-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		ctx = auth.SelectPutFormulaByExistence(ctx, existingExists)
		if existingExists {
			_, visible, visErr := b.checkConceptDescriptionVisibilityInTx(ctx, tx, id)
			if visErr != nil {
				return visErr
			}
			if !visible {
				return common.NewErrDenied("CDREPO-PUTCD-ABACDENIED existing concept description is not accessible under ABAC constraints")
			}
		}
	}

	if existingExists {
		if err = b.deleteConceptDescriptionInTx(ctx, tx, id); err != nil {
			return err
		}
	}

	if err = b.createConceptDescriptionInTx(ctx, tx, cd); err != nil {
		return err
	}

	if shouldEnforceFormula {
		exists, visible, visErr := b.checkConceptDescriptionVisibilityInTx(ctx, tx, cd.ID())
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewInternalServerError("CDREPO-PUTCD-ABACCHECKMISSING written concept description not found before commit")
		}
		if !visible {
			return common.NewErrDenied("CDREPO-PUTCD-ABACDENIED written concept description is not accessible under ABAC constraints")
		}
	}

	if err = tx.Commit(); err != nil {
		return common.NewInternalServerError("CDREPO-PUTCD-COMMIT " + err.Error())
	}

	return nil
}

// DeleteConceptDescription removes a concept description by its identifier.
func (b *ConceptDescriptionBackend) DeleteConceptDescription(ctx context.Context, id string) (err error) {
	tx, cleanup, err := common.StartTransaction(b.db)
	if err != nil {
		return common.NewInternalServerError("CDREPO-DELCD-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return common.NewInternalServerError("CDREPO-DELCD-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		exists, visible, visErr := b.checkConceptDescriptionVisibilityInTx(ctx, tx, id)
		if visErr != nil {
			return visErr
		}
		if exists && !visible {
			return common.NewErrDenied("CDREPO-DELCD-ABACDENIED deleting this concept description is not allowed")
		}
	}

	if err = b.deleteConceptDescriptionInTx(ctx, tx, id); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return common.NewInternalServerError("CDREPO-DELCD-COMMIT " + err.Error())
	}

	return nil
}
