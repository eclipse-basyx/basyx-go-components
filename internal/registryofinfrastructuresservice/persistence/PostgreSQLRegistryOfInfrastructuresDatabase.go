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

// Package registryofinfrastructurespostgresql provides PostgreSQL-based persistence implementation
// for the Eclipse BaSyx RegistryOfInfrastructures Service.
//
// This package implements the storage and retrieval of Infrastructure identifiers in a PostgreSQL database.
// It supports operations for creating, retrieving, searching, and deleting AAS RegistryOfInfrastructures
// information with cursor-based pagination for efficient querying of large datasets.
package registryofinfrastructurespostgresql

import (
	"context"
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// PostgreSQLRegistryOfInfrastructuresDatabase provides PostgreSQL-based persistence for the RegistryOfInfrastructures Service.
//
// It manages infrastructure identifiers in a PostgreSQL database, using connection pooling for efficient
// database access. The database schema is automatically initialized on startup from the
// RegistryOfInfrastructuresschema.sql file.
type PostgreSQLRegistryOfInfrastructuresDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

// NewPostgreSQLRegistryOfInfrastructuresBackend creates and initializes a new PostgreSQL RegistryOfInfrastructures database backend.
//
// This function establishes a connection pool to the PostgreSQL database using the provided DSN
// (Data Source Name), configures connection pool settings, and initializes the database schema
// by executing the RegistryOfInfrastructuresschema.sql file from the resources/sql directory.
//
// Parameters:
//   - dsn: PostgreSQL connection string (e.g., "postgres://user:pass@localhost:5432/dbname")
//   - maxConns: Maximum number of connections in the pool
//
// Returns:
//   - *PostgreSQLRegistryOfInfrastructuresDatabase: Initialized database instance
//   - error: Configuration, connection, or schema initialization error
//
// The connection pool is configured with:
//   - MaxConns: Set to the provided maxConns parameter
//   - MaxConnLifetime: 5 minutes to ensure connection freshness
//
// The function reads and executes RegistryOfInfrastructuresschema.sql from the current working directory's
// resources/sql subdirectory to set up the required database tables.
func NewPostgreSQLRegistryOfInfrastructuresBackend(dsn string, _ int32 /* maxOpenConns */, _ /* maxIdleConns */ int, _ /* connMaxLifetimeMinutes */ int, cacheEnabled bool, databaseSchema string) (*PostgreSQLRegistryOfInfrastructuresDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	return &PostgreSQLRegistryOfInfrastructuresDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

// InsertInfrastructureDescriptor inserts the provided infrastructure descriptor
// and all related nested entities into the database.
func (p *PostgreSQLRegistryOfInfrastructuresDatabase) InsertInfrastructureDescriptor(
	ctx context.Context,
	aasd model.InfrastructureDescriptor,
) error {
	return descriptors.InsertInfrastructureDescriptor(ctx, p.db, aasd)
}

// GetInfrastructureDescriptorByID returns the infrastructure descriptor
// identified by the given infrastructure descriptor ID.
func (p *PostgreSQLRegistryOfInfrastructuresDatabase) GetInfrastructureDescriptorByID(
	ctx context.Context,
	registryIdentifier string,
) (model.InfrastructureDescriptor, error) {
	return descriptors.GetInfrastructureDescriptorByID(ctx, p.db, registryIdentifier)
}

// DeleteInfrastructureDescriptorByID deletes the infrastructure descriptor
// identified by the given ID.
func (p *PostgreSQLRegistryOfInfrastructuresDatabase) DeleteInfrastructureDescriptorByID(
	ctx context.Context,
	infrastructureDescriptor string,
) error {
	return descriptors.DeleteInfrastructureDescriptorByID(ctx, p.db, infrastructureDescriptor)
}

// ReplaceInfrastructureDescriptor replaces an existing infrastructure descriptor
// with the given value and reports whether it existed.
func (p *PostgreSQLRegistryOfInfrastructuresDatabase) ReplaceInfrastructureDescriptor(
	ctx context.Context,
	infrastructureDescriptor model.InfrastructureDescriptor,
) (bool, error) {
	return descriptors.ReplaceInfrastructureDescriptor(ctx, p.db, infrastructureDescriptor)
}

// ListInfrastructureDescriptors lists infrastructure descriptors with optional
// pagination and asset filtering, returning a next-page cursor when present.
func (p *PostgreSQLRegistryOfInfrastructuresDatabase) ListInfrastructureDescriptors(
	ctx context.Context,
	limit int32,
	cursor string,
	company string,
	endpointInterface string,
) ([]model.InfrastructureDescriptor, string, error) {
	return descriptors.ListInfrastructureDescriptors(ctx, p.db, limit, cursor, company, endpointInterface)
}
