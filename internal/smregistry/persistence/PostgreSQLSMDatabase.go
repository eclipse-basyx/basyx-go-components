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

	"github.com/doug-martin/goqu/v9"
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
) (*PostgreSQLSMDatabase, error) {
	db, err := common.NewDatabaseConnection(dsn)
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

	return NewPostgreSQLSMBackendFromDB(db)
}

// NewPostgreSQLSMBackendFromDB creates a new backend instance from an existing DB pool.
func NewPostgreSQLSMBackendFromDB(db *sql.DB) (*PostgreSQLSMDatabase, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("SMREG-NEWFROMDB-NILDB database handle must not be nil")
	}

	return &PostgreSQLSMDatabase{db: db}, nil
}

// ExecuteInTransaction executes fn within a single database transaction.
func (p *PostgreSQLSMDatabase) ExecuteInTransaction(
	startErrorCode string,
	commitErrorCode string,
	fn func(tx *sql.Tx) error,
) error {
	return common.ExecuteInTransaction(p.db, startErrorCode, commitErrorCode, fn)
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

// InsertSubmodelDescriptorInTransaction inserts a global submodel descriptor
// in the provided transaction.
func (p *PostgreSQLSMDatabase) InsertSubmodelDescriptorInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	if tx == nil {
		return model.SubmodelDescriptor{}, common.NewInternalServerError("SMREG-INSERTSMDESC-NILTX transaction must not be nil")
	}
	return descriptors.InsertSubmodelDescriptorTx(ctx, tx, submodel)
}

// InsertSubmodelDescriptorsInTransaction inserts multiple global submodel descriptors.
//
// The method inserts descriptor graph rows in the provided transaction and
// performs readback when create authorization requires it.
//
// Parameters:
//   - ctx: Request context carrying configuration and security data.
//   - tx: Transaction used for the bulk insert.
//   - submodels: Global submodel descriptors to insert.
//
// Returns:
//   - int: Failed descriptor index, or -1 on success.
//   - error: Error when batch creation, insertion, or readback fails.
func (p *PostgreSQLSMDatabase) InsertSubmodelDescriptorsInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	submodels []model.SubmodelDescriptor,
) (int, error) {
	if tx == nil {
		return 0, common.NewInternalServerError("SMREG-BULKINSERT-NILTX transaction must not be nil")
	}
	if len(submodels) == 0 {
		return -1, nil
	}

	batch, err := descriptors.BuildSubmodelDescriptorsCreateBatch(ctx, tx, submodels)
	if err != nil {
		return 0, err
	}
	if err = common.ExecutePostgreSQLBatchInTransaction(ctx, tx, batch.Statements()); err != nil {
		return 0, err
	}

	if descriptors.CanSkipCreateReadback(ctx) {
		return -1, nil
	}

	for index, descriptor := range submodels {
		if _, getErr := descriptors.GetSubmodelDescriptorByID(ctx, tx, descriptor.Id); getErr != nil {
			if common.IsErrNotFound(getErr) {
				return index, common.NewErrDenied("Submodel Descriptor access not allowed")
			}
			return index, getErr
		}
	}
	return -1, nil
}

// ReplaceSubmodelDescriptor replaces a global Submodel Descriptor (no AAS association).
func (p *PostgreSQLSMDatabase) ReplaceSubmodelDescriptor(
	ctx context.Context,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	return descriptors.ReplaceSubmodelDescriptor(ctx, p.db, submodel)
}

// UpsertSubmodelDescriptorInTransaction replaces an existing global submodel
// descriptor or inserts it when missing in the provided transaction.
func (p *PostgreSQLSMDatabase) UpsertSubmodelDescriptorInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	submodel model.SubmodelDescriptor,
) error {
	if tx == nil {
		return common.NewInternalServerError("SMREG-UPSERTSMDESC-NILTX transaction must not be nil")
	}

	if err := lockSubmodelDescriptorUpsertTx(ctx, tx, submodel.Id); err != nil {
		return err
	}

	if err := descriptors.DeleteSubmodelDescriptorByIDTx(ctx, tx, submodel.Id); err != nil {
		if !common.IsErrNotFound(err) {
			return err
		}
		_, insertErr := descriptors.InsertSubmodelDescriptorTx(ctx, tx, submodel)
		return insertErr
	}

	_, err := descriptors.InsertSubmodelDescriptorTx(ctx, tx, submodel)
	return err
}

func lockSubmodelDescriptorUpsertTx(ctx context.Context, tx *sql.Tx, submodelID string) error {
	sqlStr, args, err := buildSubmodelDescriptorUpsertLockSQL(submodelID)
	if err != nil {
		return common.NewInternalServerError("SMREG-LOCKSMDESCUPSERT-BUILDSQL " + err.Error())
	}

	if _, err = tx.ExecContext(ctx, sqlStr, args...); err != nil {
		return common.NewInternalServerError("SMREG-LOCKSMDESCUPSERT-EXECSQL " + err.Error())
	}
	return nil
}

func buildSubmodelDescriptorUpsertLockSQL(submodelID string) (string, []any, error) {
	return goqu.
		Dialect(common.Dialect).
		Select(goqu.Func("pg_advisory_xact_lock", goqu.Func("hashtextextended", "submodel_descriptor:"+submodelID, int64(0)))).
		Prepared(true).
		ToSQL()
}

// GetSubmodelDescriptorByID returns a global Submodel Descriptor by its id.
func (p *PostgreSQLSMDatabase) GetSubmodelDescriptorByID(
	ctx context.Context,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	return descriptors.GetSubmodelDescriptorByID(ctx, p.db, submodelID)
}

// GetSubmodelDescriptorByIDInTransaction returns a global submodel descriptor
// by id using the provided transaction.
func (p *PostgreSQLSMDatabase) GetSubmodelDescriptorByIDInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	if tx == nil {
		return model.SubmodelDescriptor{}, common.NewInternalServerError("SMREG-GETSMDESC-NILTX transaction must not be nil")
	}
	return descriptors.GetSubmodelDescriptorByID(ctx, tx, submodelID)
}

// DeleteSubmodelDescriptorByID deletes a global Submodel Descriptor by its id.
func (p *PostgreSQLSMDatabase) DeleteSubmodelDescriptorByID(
	ctx context.Context,
	submodelID string,
) error {
	return descriptors.DeleteSubmodelDescriptorByID(ctx, p.db, submodelID)
}

// DeleteSubmodelDescriptorByIDInTransaction deletes a global submodel
// descriptor by id in the provided transaction.
func (p *PostgreSQLSMDatabase) DeleteSubmodelDescriptorByIDInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
) error {
	if tx == nil {
		return common.NewInternalServerError("SMREG-DELSMDESC-NILTX transaction must not be nil")
	}
	return descriptors.DeleteSubmodelDescriptorByIDTx(ctx, tx, submodelID)
}

// DeleteSubmodelDescriptorsByIDsInTransaction deletes multiple global submodel descriptors.
//
// The method deletes all supplied descriptors in the provided transaction.
//
// Parameters:
//   - ctx: Request context carrying configuration data.
//   - tx: Transaction used for deletion.
//   - submodelIDs: Global submodel descriptor identifiers to delete.
//
// Returns:
//   - error: Error when the transaction is missing or deletion fails.
func (p *PostgreSQLSMDatabase) DeleteSubmodelDescriptorsByIDsInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	submodelIDs []string,
) error {
	if tx == nil {
		return common.NewInternalServerError("SMREG-BULKDELETE-NILTX transaction must not be nil")
	}
	return descriptors.DeleteSubmodelDescriptorsByIDsTx(ctx, tx, submodelIDs)
}

// ExistsSubmodelByID reports whether a global Submodel Descriptor exists by its id.
func (p *PostgreSQLSMDatabase) ExistsSubmodelByID(
	ctx context.Context,
	submodelID string,
) (bool, error) {
	return descriptors.ExistsSubmodelByID(ctx, p.db, submodelID)
}

// ExistingSubmodelDescriptorIDsInTransaction returns existing global submodel descriptor ids.
//
// The result map contains only identifiers that already exist for global
// submodel descriptors.
//
// Parameters:
//   - ctx: Request context carrying configuration data.
//   - tx: Transaction used for the existence lookup.
//   - identifiers: Candidate submodel descriptor identifiers.
//
// Returns:
//   - map[string]struct{}: Set keyed by existing identifier.
//   - error: Error when SQL rendering or database reads fail.
func (p *PostgreSQLSMDatabase) ExistingSubmodelDescriptorIDsInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	identifiers []string,
) (map[string]struct{}, error) {
	if tx == nil {
		return nil, common.NewInternalServerError("SMREG-BULKEXISTS-NILTX transaction must not be nil")
	}
	existing := make(map[string]struct{})
	limit := common.BulkBatchLimitFromContext(ctx)
	for start := 0; start < len(identifiers); start += limit {
		end := min(start+limit, len(identifiers))
		query, args, err := goqu.
			From(common.TblSubmodelDescriptor).
			Select(common.ColAASID).
			Where(
				goqu.And(
					goqu.C(common.ColAASID).In(identifiers[start:end]),
					goqu.C(common.ColAASDescriptorID).IsNull(),
				),
			).
			ToSQL()
		if err != nil {
			return nil, common.NewInternalServerError("SMREG-BULKEXISTS-BUILDSQL " + err.Error())
		}
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, common.NewInternalServerError("SMREG-BULKEXISTS-EXECQUERY " + err.Error())
		}
		for rows.Next() {
			var identifier string
			if scanErr := rows.Scan(&identifier); scanErr != nil {
				_ = rows.Close()
				return nil, common.NewInternalServerError("SMREG-BULKEXISTS-SCANID " + scanErr.Error())
			}
			existing[identifier] = struct{}{}
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return nil, common.NewInternalServerError("SMREG-BULKEXISTS-ITERATE " + err.Error())
		}
		_ = rows.Close()
	}
	return existing, nil
}
