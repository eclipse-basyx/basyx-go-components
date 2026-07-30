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
// Author: Christian Koort ( Fraunhofer IESE )

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

// PostgreSQLCompanyLookupDatabase provides PostgreSQL-backed persistence for
// the Company Lookup Service. The database pool remains owned by the caller.
type PostgreSQLCompanyLookupDatabase struct {
	db           *sql.DB
	readerDB     *sql.DB
	cacheEnabled bool
}

// NewPostgreSQLCompanyLookupBackend creates a Company Lookup backend with its
// own PostgreSQL connection pool.
//
// Positive pool limits are applied to the new pool. Zero values use the common
// pool defaults and negative values are rejected. The backend retains the pool
// for its service lifetime.
//
// Parameters:
//   - dsn: PostgreSQL connection string.
//   - maxOpenConns: Maximum number of open connections.
//   - maxIdleConns: Maximum number of idle connections.
//   - connMaxLifetimeMinutes: Maximum connection lifetime in minutes.
//   - cacheEnabled: Whether descriptor lookup caching is enabled.
//
// Returns:
//   - *PostgreSQLCompanyLookupDatabase: Backend using the newly created pool.
//   - error: Connection or backend validation error.
func NewPostgreSQLCompanyLookupBackend(dsn string, maxOpenConns int32, maxIdleConns int, connMaxLifetimeMinutes int, cacheEnabled bool) (*PostgreSQLCompanyLookupDatabase, error) {
	db, err := common.NewDatabaseConnectionWithConfig(dsn, common.PostgresConfig{
		MaxOpenConnections:     int(maxOpenConns),
		MaxIdleConnections:     maxIdleConns,
		ConnMaxLifetimeMinutes: connMaxLifetimeMinutes,
	})
	if err != nil {
		return nil, err
	}

	return NewPostgreSQLCompanyLookupBackendFromDB(db, cacheEnabled)
}

// NewPostgreSQLCompanyLookupBackendFromDB creates a backend using an existing
// PostgreSQL pool. The caller retains ownership of db and must close it.
//
// Parameters:
//   - db: Shared PostgreSQL connection pool.
//   - cacheEnabled: Whether descriptor lookup caching is enabled.
//
// Returns:
//   - *PostgreSQLCompanyLookupDatabase: Backend sharing the caller-owned pool.
//   - error: Validation error when db is nil.
func NewPostgreSQLCompanyLookupBackendFromDB(db *sql.DB, cacheEnabled bool) (*PostgreSQLCompanyLookupDatabase, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("COMPANYLOOKUP-NEWFROMDB-NILDB database handle must not be nil")
	}
	return NewPostgreSQLCompanyLookupBackendFromPools(db, db, cacheEnabled)
}

// NewPostgreSQLCompanyLookupBackendFromPools creates a backend using
// caller-owned writer and reader pools.
func NewPostgreSQLCompanyLookupBackendFromPools(writer *sql.DB, reader *sql.DB, cacheEnabled bool) (*PostgreSQLCompanyLookupDatabase, error) {
	if writer == nil {
		return nil, common.NewErrBadRequest("COMPANYLOOKUP-NEWFROMPOOLS-NILWRITER writer database handle must not be nil")
	}
	if reader == nil {
		return nil, common.NewErrBadRequest("COMPANYLOOKUP-NEWFROMPOOLS-NILREADER reader database handle must not be nil")
	}
	return &PostgreSQLCompanyLookupDatabase{db: writer, readerDB: reader, cacheEnabled: cacheEnabled}, nil
}

func (p *PostgreSQLCompanyLookupDatabase) readDB(ctx context.Context) *sql.DB {
	return common.PostgresReadPool(ctx, p.db, p.readerDB)
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
	return descriptors.GetCompanyDescriptorByID(ctx, p.readDB(ctx), companyIdentifier)
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
	return descriptors.ListCompanyDescriptors(ctx, p.readDB(ctx), limit, cursor, name, assetID)
}

// ExistsCompanyDescriptorByID reports whether a company descriptor with the given ID exists.
func (p *PostgreSQLCompanyLookupDatabase) ExistsCompanyDescriptorByID(
	ctx context.Context,
	companyIdentifier string,
) (bool, error) {
	return descriptors.ExistsCompanyDescriptorByID(ctx, p.db, companyIdentifier)
}
