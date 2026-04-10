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

// Package companylookuppostgresql provides PostgreSQL-based persistence implementation
// for the Eclipse BaSyx Company Lookup Service.
//
// This package implements the storage and retrieval of company descriptors in a PostgreSQL database.
// It supports operations for creating, retrieving, searching, and deleting company descriptors
// information with cursor-based pagination for efficient querying of large datasets.
package companylookuppostgresql

import (
	"context"
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// PostgreSQLCompanyLookupDatabase provides PostgreSQL-based persistence for the Company Lookup Service.
//
// It manages company descriptors in a PostgreSQL database, using connection pooling for efficient
// database access. The database schema is automatically initialized on startup from the
// CompanyLookupSchema.sql file.
type PostgreSQLCompanyLookupDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

// NewPostgreSQLCompanyLookupBackend creates and initializes a new PostgreSQL company lookup database backend.
//
// This function establishes a connection pool to the PostgreSQL database using the provided DSN
// (Data Source Name), configures connection pool settings, and initializes the database schema
// by executing the schema file from the resources/sql directory.
//
// Parameters:
//   - dsn: PostgreSQL connection string (e.g., "postgres://user:pass@localhost:5432/dbname")
//   - maxConns: Maximum number of connections in the pool
//
// Returns:
//   - *PostgreSQLCompanyLookupDatabase: Initialized database instance
//   - error: Configuration, connection, or schema initialization error
//
// The connection pool is configured with:
//   - MaxConns: Set to the provided maxConns parameter
//   - MaxConnLifetime: 5 minutes to ensure connection freshness
//
// The function reads and executes the schema file from the current working directory's
// resources/sql subdirectory to set up the required database tables.
func NewPostgreSQLCompanyLookupBackend(dsn string, _ int32 /* maxOpenConns */, _ /* maxIdleConns */ int, _ /* connMaxLifetimeMinutes */ int, cacheEnabled bool, databaseSchema string) (*PostgreSQLCompanyLookupDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	return &PostgreSQLCompanyLookupDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

// InsertCompanyDescriptor inserts the provided company descriptor
// and all related nested entities into the database.
func (p *PostgreSQLCompanyLookupDatabase) InsertCompanyDescriptor(
	ctx context.Context,
	companyDescriptor model.CompanyDescriptor,
) (model.CompanyDescriptor, error) {
	return descriptors.InsertCompanyDescriptor(ctx, p.db, companyDescriptor)
}

// GetCompanyDescriptorByID returns the company descriptor
// identified by the given company descriptor ID.
func (p *PostgreSQLCompanyLookupDatabase) GetCompanyDescriptorByID(
	ctx context.Context,
	companyIdentifier string,
) (model.CompanyDescriptor, error) {
	return descriptors.GetCompanyDescriptorByID(ctx, p.db, companyIdentifier)
}

// DeleteCompanyDescriptorByID deletes the company descriptor
// identified by the given ID.
func (p *PostgreSQLCompanyLookupDatabase) DeleteCompanyDescriptorByID(
	ctx context.Context,
	companyIdentifier string,
) error {
	return descriptors.DeleteCompanyDescriptorByID(ctx, p.db, companyIdentifier)
}

// ReplaceCompanyDescriptor replaces an existing company descriptor
// with the given value and reports whether it existed.
func (p *PostgreSQLCompanyLookupDatabase) ReplaceCompanyDescriptor(
	ctx context.Context,
	companyDescriptor model.CompanyDescriptor,
) (model.CompanyDescriptor, error) {
	return descriptors.ReplaceCompanyDescriptor(ctx, p.db, companyDescriptor)
}

// ListCompanyDescriptors lists company descriptors with optional
// pagination and asset filtering, returning a next-page cursor when present.
func (p *PostgreSQLCompanyLookupDatabase) ListCompanyDescriptors(
	ctx context.Context,
	limit int32,
	cursor string,
	name string,
	assetID string,
) ([]model.CompanyDescriptor, string, error) {
	return descriptors.ListCompanyDescriptors(ctx, p.db, limit, cursor, name, assetID)
}

// ExistsCompanyDescriptorByID reports whether a company descriptor with the given ID exists.
func (p *PostgreSQLCompanyLookupDatabase) ExistsCompanyDescriptorByID(
	ctx context.Context,
	companyIdentifier string,
) (bool, error) {
	return descriptors.ExistsCompanyDescriptorByID(ctx, p.db, companyIdentifier)
}
