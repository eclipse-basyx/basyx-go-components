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
// Author: Martin Stemmer ( Fraunhofer IESE )

// Package smregistrypostgresql provides PostgreSQL-based persistence implementation
package smregistrypostgresql

import (
	"context"
	"database/sql"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// PostgreSQLSMDatabase provides PostgreSQL-based persistence for the Submodel Registry Service.
type PostgreSQLSMDatabase struct {
	db *sql.DB
}

// NewPostgreSQLSMBackend creates and initializes a new PostgreSQL Submodel Registry database backend.
func NewPostgreSQLSMBackend(
	dsn string,
	maxOpenConns int32,
	maxIdleConns int,
	connMaxLifetimeMinutes int,
	_ bool,
	databaseSchema string,
) (*PostgreSQLSMDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	if maxOpenConns > 0 {
		db.SetMaxOpenConns(int(maxOpenConns))
	}
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}
	if connMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(connMaxLifetimeMinutes) * time.Minute)
	}

	return &PostgreSQLSMDatabase{db: db}, nil
}

// ListSubmodelDescriptors lists global Submodel Descriptors (no AAS association).
func (p *PostgreSQLSMDatabase) ListSubmodelDescriptors(
	ctx context.Context,
	limit int32,
	cursor string,
) ([]model.SubmodelDescriptor, string, error) {
	return descriptors.ListSubmodelDescriptors(ctx, p.db, limit, cursor)
}

// InsertSubmodelDescriptor inserts a global Submodel Descriptor (no AAS association).
func (p *PostgreSQLSMDatabase) InsertSubmodelDescriptor(
	ctx context.Context,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	return descriptors.InsertSubmodelDescriptor(ctx, p.db, submodel)
}

// ReplaceSubmodelDescriptor replaces a global Submodel Descriptor (no AAS association).
func (p *PostgreSQLSMDatabase) ReplaceSubmodelDescriptor(
	ctx context.Context,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	return descriptors.ReplaceSubmodelDescriptor(ctx, p.db, submodel)
}

// GetSubmodelDescriptorByID returns a global Submodel Descriptor by its id.
func (p *PostgreSQLSMDatabase) GetSubmodelDescriptorByID(
	ctx context.Context,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	return descriptors.GetSubmodelDescriptorByID(ctx, p.db, submodelID)
}

// DeleteSubmodelDescriptorByID deletes a global Submodel Descriptor by its id.
func (p *PostgreSQLSMDatabase) DeleteSubmodelDescriptorByID(
	ctx context.Context,
	submodelID string,
) error {
	return descriptors.DeleteSubmodelDescriptorByID(ctx, p.db, submodelID)
}

// ExistsSubmodelByID reports whether a global Submodel Descriptor exists by its id.
func (p *PostgreSQLSMDatabase) ExistsSubmodelByID(
	ctx context.Context,
	submodelID string,
) (bool, error) {
	return descriptors.ExistsSubmodelByID(ctx, p.db, submodelID)
}
