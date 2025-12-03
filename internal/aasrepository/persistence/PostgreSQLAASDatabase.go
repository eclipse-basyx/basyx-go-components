/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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
// Package persistencepostgresql provides PostgreSQL-based persistence implementation
// for the Eclipse BaSyx AAS Service.
//
// This package implements the storage and retrieval of Asset Administration Shell (AAS)
// identifiers and their associated asset links in a PostgreSQL database. It supports
// operations for creating, retrieving, searching, and deleting AAS discovery information
// with cursor-based pagination for efficient querying of large datasets.

// Package persistencepostgresql provides PostgreSQL-based persistence for the AAS repository.
package persistencepostgresql

import (
	"database/sql"
	"errors"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"

	// Import for PostgreSQL driver
	_ "github.com/lib/pq"
)

// PostgreSQLAASDatabase is the DB handler used by the AAS Repository.
type PostgreSQLAASDatabase struct {
	DB *sql.DB
}

// NewPostgreSQLAASDatabaseBackend initializes the database and applies schema.
func NewPostgreSQLAASDatabaseBackend(
	dsn string,
	maxOpenConns int,
	maxIdleConns int,
	_ int, // connMaxLifetimeMinutes is unused for now
	databaseSchema string,
) (*PostgreSQLAASDatabase, error) {

	// common.InitializeDatabase executes the SQL schema file automatically.
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	// (Optional) configure SQL connection pooling
	if maxOpenConns > 0 {
		db.SetMaxOpenConns(maxOpenConns)
	}
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}

	return &PostgreSQLAASDatabase{DB: db}, nil
}

// GetAllAAS retrieves all Asset Administration Shells from the database.
func (p *PostgreSQLAASDatabase) GetAllAAS() ([]model.AssetAdministrationShell, error) {
	dialect := goqu.New("postgres", p.DB)

	query, _, err := dialect.
		From("aas").
		Select("id", "id_short", "category", "model_type").
		Order(goqu.I("id").Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := p.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.AssetAdministrationShell

	for rows.Next() {
		var shell model.AssetAdministrationShell
		if err := rows.Scan(&shell.ID, &shell.IdShort, &shell.Category, &shell.ModelType); err != nil {
			return nil, err
		}

		// Add placeholders
		shell.DisplayName = []model.LangStringNameType{}
		shell.Description = []model.LangStringTextType{}
		shell.Extensions = []model.Extension{}
		shell.EmbeddedDataSpecifications = []model.EmbeddedDataSpecification{}
		shell.Submodels = []model.Reference{}
		shell.DerivedFrom = nil
		shell.Administration = model.AdministrativeInformation{}
		shell.AssetInformation = &model.AssetInformation{}

		result = append(result, shell)
	}

	return result, nil
}

// InsertAAS inserts a new Asset Administration Shell into the database.
func (p *PostgreSQLAASDatabase) InsertAAS(aas model.AssetAdministrationShell) error {
	dialect := goqu.New("postgres", p.DB)

	query, _, err := dialect.
		Insert("aas").
		Rows(goqu.Record{
			"id":         aas.ID,
			"id_short":   aas.IdShort,
			"category":   aas.Category,
			"model_type": aas.ModelType,
		}).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = p.DB.Exec(query)
	return err
}

// DeleteAASByID deletes an Asset Administration Shell by its ID.
func (p *PostgreSQLAASDatabase) DeleteAASByID(id string) error {
	dialect := goqu.New("postgres", p.DB)

	query, _, err := dialect.
		Delete("aas").
		Where(goqu.Ex{"id": id}).
		ToSQL()
	if err != nil {
		return err
	}

	result, err := p.DB.Exec(query)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// GetAASByID retrieves an Asset Administration Shell by its ID.
func (p *PostgreSQLAASDatabase) GetAASByID(id string) (*model.AssetAdministrationShell, error) {
	dialect := goqu.New("postgres", p.DB)
	query, _, err := dialect.
		From("aas").
		Select("id", "id_short", "category", "model_type").
		Where(goqu.Ex{"id": id}).
		Limit(1).
		ToSQL()
	if err != nil {
		return nil, err
	}
	row := p.DB.QueryRow(query)

	var shell model.AssetAdministrationShell
	if err := row.Scan(&shell.ID, &shell.IdShort, &shell.Category, &shell.ModelType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	// Placeholder values
	shell.DisplayName = []model.LangStringNameType{}
	shell.Description = []model.LangStringTextType{}
	shell.Extensions = []model.Extension{}
	shell.EmbeddedDataSpecifications = []model.EmbeddedDataSpecification{}
	shell.Submodels = []model.Reference{}
	shell.DerivedFrom = nil
	shell.Administration = model.AdministrativeInformation{}
	shell.AssetInformation = &model.AssetInformation{}

	return &shell, nil
}
