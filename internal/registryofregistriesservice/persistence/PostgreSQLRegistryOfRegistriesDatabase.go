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

// Package registryofregistriespostgresql provides PostgreSQL-based persistence implementation
// for the Eclipse BaSyx RegistryOfRegistries Service.
//
// This package implements the storage and retrieval of Registry identifiers in a PostgreSQL database.
// It supports operations for creating, retrieving, searching, and deleting AAS RegistryOfRegistries
// information with cursor-based pagination for efficient querying of large datasets.
package registryofregistriespostgresql

import (
	"context"
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// PostgreSQLRegistryOfRegistriesDatabase provides PostgreSQL-based persistence for the RegistryOfRegistries Service.
//
// It manages registry identifiers in a PostgreSQL database, using connection pooling for efficient
// database access. The database schema is automatically initialized on startup from the
// RegistryOfRegistriesschema.sql file.
type PostgreSQLRegistryOfRegistriesDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

// NewPostgreSQLRegistryOfRegistriesBackend creates and initializes a new PostgreSQL RegistryOfRegistries database backend.
//
// This function establishes a connection pool to the PostgreSQL database using the provided DSN
// (Data Source Name), configures connection pool settings, and initializes the database schema
// by executing the RegistryOfRegistriesschema.sql file from the resources/sql directory.
//
// Parameters:
//   - dsn: PostgreSQL connection string (e.g., "postgres://user:pass@localhost:5432/dbname")
//   - maxConns: Maximum number of connections in the pool
//
// Returns:
//   - *PostgreSQLRegistryOfRegistriesDatabase: Initialized database instance
//   - error: Configuration, connection, or schema initialization error
//
// The connection pool is configured with:
//   - MaxConns: Set to the provided maxConns parameter
//   - MaxConnLifetime: 5 minutes to ensure connection freshness
//
// The function reads and executes RegistryOfRegistriesschema.sql from the current working directory's
// resources/sql subdirectory to set up the required database tables.
func NewPostgreSQLRegistryOfRegistriesBackend(dsn string, _ int32 /* maxOpenConns */, _ /* maxIdleConns */ int, _ /* connMaxLifetimeMinutes */ int, cacheEnabled bool, databaseSchema string) (*PostgreSQLRegistryOfRegistriesDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	return &PostgreSQLRegistryOfRegistriesDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

// InsertRegistryDescriptor inserts the provided registry descriptor
// and all related nested entities into the database.
func (p *PostgreSQLRegistryOfRegistriesDatabase) InsertRegistryDescriptor(
	ctx context.Context,
	aasd model.RegistryDescriptor,
) error {
	return descriptors.InsertRegistryDescriptor(ctx, p.db, aasd)
}

// GetRegistryDescriptorByID returns the registry descriptor
// identified by the given registry descriptor ID.
func (p *PostgreSQLRegistryOfRegistriesDatabase) GetRegistryDescriptorByID(
	ctx context.Context,
	registryIdentifier string,
) (model.RegistryDescriptor, error) {
	return descriptors.GetRegistryDescriptorByID(ctx, p.db, registryIdentifier)
}

// DeleteRegistryDescriptorByID deletes the registry descriptor
// identified by the given ID.
func (p *PostgreSQLRegistryOfRegistriesDatabase) DeleteRegistryDescriptorByID(
	ctx context.Context,
	registryDescriptor string,
) error {
	return descriptors.DeleteRegistryDescriptorByID(ctx, p.db, registryDescriptor)
}

// ReplaceRegistryDescriptor replaces an existing registry descriptor
// with the given value and reports whether it existed.
func (p *PostgreSQLRegistryOfRegistriesDatabase) ReplaceRegistryDescriptor(
	ctx context.Context,
	registryDescriptor model.RegistryDescriptor,
) (bool, error) {
	return descriptors.ReplaceRegistryDescriptor(ctx, p.db, registryDescriptor)
}

// ListRegistryDescriptors lists registry descriptors with optional
// pagination and asset filtering, returning a next-page cursor when present.
func (p *PostgreSQLRegistryOfRegistriesDatabase) ListRegistryDescriptors(
	ctx context.Context,
	limit int32,
	cursor string,
	registryType string,
	company string,
	endpointInterface string,
) ([]model.RegistryDescriptor, string, error) {
	return descriptors.ListRegistryDescriptors(ctx, p.db, limit, cursor, registryType, company, endpointInterface)
}
