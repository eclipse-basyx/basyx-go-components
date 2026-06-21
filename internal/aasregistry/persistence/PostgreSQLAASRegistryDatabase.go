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

// Package aasregistrydatabase provides a PostgreSQL-backed persistence layer
// for the AAS Registry. It offers creation, retrieval, listing, replacement,
// and deletion of Asset Administration Shell (AAS) descriptors and their
// related entities (endpoints, specific asset IDs, extensions, and submodel
// descriptors). The package uses goqu to build SQL and database/sql for query
// execution, and applies cursor-based pagination where appropriate.
// Author: Martin Stemmer ( Fraunhofer IESE )
package aasregistrydatabase

import (
	"context"
	"database/sql"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/jackc/pgx/v5"
)

// PostgreSQLAASRegistryDatabase is a PostgreSQL-backed implementation of the AAS
// registry database. It is safe for concurrent use by multiple goroutines.
type PostgreSQLAASRegistryDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

// NewPostgreSQLAASRegistryDatabase creates a new PostgreSQL-backed AAS registry
// database handle. It initializes the database using the provided DSN and
// schema path (or the default bundled schema when empty), and configures the
// connection pool according to the supplied limits. The returned instance can
// be used concurrently by multiple goroutines.
func NewPostgreSQLAASRegistryDatabase(
	dsn string,
	maxOpenConns int32,
	maxIdleConns int,
	connMaxLifetimeMinutes int,
	cacheEnabled bool,
) (*PostgreSQLAASRegistryDatabase, error) {
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

	return NewPostgreSQLAASRegistryDatabaseFromDB(db, cacheEnabled)
}

// NewPostgreSQLAASRegistryDatabaseFromDB creates a new backend instance from an existing DB pool.
func NewPostgreSQLAASRegistryDatabaseFromDB(db *sql.DB, cacheEnabled bool) (*PostgreSQLAASRegistryDatabase, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("AASREG-NEWFROMDB-NILDB database handle must not be nil")
	}

	return &PostgreSQLAASRegistryDatabase{
		db:           db,
		cacheEnabled: cacheEnabled,
	}, nil
}

// ExecuteInTransaction executes fn within a single database transaction.
func (p *PostgreSQLAASRegistryDatabase) ExecuteInTransaction(
	startErrorCode string,
	commitErrorCode string,
	fn func(tx *sql.Tx) error,
) error {
	return common.ExecuteInTransaction(p.db, startErrorCode, commitErrorCode, fn)
}

func appendDescriptorHistoryTx(ctx context.Context, tx *sql.Tx, descriptor model.AssetAdministrationShellDescriptor, changeType string, deleted bool) error {
	snapshot, err := descriptor.ToJsonable()
	if err != nil {
		return common.NewInternalServerError("AASREG-HISTORY-TOJSONABLE " + err.Error())
	}

	return history.AppendVersionTx(ctx, tx, history.TableDescriptor, descriptor.Id, changeType, snapshot, deleted)
}

// InsertAdministrationShellDescriptor inserts the provided AAS descriptor
// and all related nested entities into the database.
func (p *PostgreSQLAASRegistryDatabase) InsertAdministrationShellDescriptor(
	ctx context.Context,
	aasd model.AssetAdministrationShellDescriptor,
) (model.AssetAdministrationShellDescriptor, error) {
	if common.SupportsPostgreSQLBatch(p.db) &&
		history.ActiveConfig().Mode == history.ModeOff &&
		descriptors.CanSkipPostInsertReadback(ctx) {
		return p.insertAdministrationShellDescriptorBatch(ctx, aasd)
	}

	batch, err := descriptors.BuildAdministrationShellDescriptorCreateBatch(ctx, aasd)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}

	var result model.AssetAdministrationShellDescriptor
	err = common.ExecuteInTransaction(p.db, "AASREG-INSERTAASDESC-STARTTX", "AASREG-INSERTAASDESC-COMMIT", func(tx *sql.Tx) error {
		if batchErr := common.ExecutePostgreSQLBatchInTransaction(ctx, tx, batch.Statements()); batchErr != nil {
			return batchErr
		}

		stored, getErr := descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id)
		if getErr != nil {
			if common.IsErrNotFound(getErr) {
				return common.NewErrDenied("AAS Descriptor access not allowed")
			}
			return getErr
		}
		if historyErr := appendDescriptorHistoryTx(ctx, tx, stored, history.ChangeCreated, false); historyErr != nil {
			return historyErr
		}

		result = stored
		return nil
	})
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	return result, nil
}

func (p *PostgreSQLAASRegistryDatabase) insertAdministrationShellDescriptorBatch(
	ctx context.Context,
	aasd model.AssetAdministrationShellDescriptor,
) (model.AssetAdministrationShellDescriptor, error) {
	batch, err := descriptors.BuildAdministrationShellDescriptorCreateBatch(ctx, aasd)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	createdAtQuery := goqu.
		From(common.TblAASDescriptor).
		Select(common.ColCreatedAt).
		Where(goqu.C(common.ColAASID).Eq(aasd.Id))
	if err = batch.AppendDataset(createdAtQuery); err != nil {
		return model.AssetAdministrationShellDescriptor{}, common.NewInternalServerError("AASREG-PGBATCH-BUILDCREATEDAT " + err.Error())
	}

	statements := batch.Statements()
	mutationCount := len(statements) - 1
	result := aasd
	err = common.ExecutePostgreSQLBatchTransaction(ctx, p.db, statements, func(batchResults pgx.BatchResults) error {
		for statementIndex := 0; statementIndex < mutationCount; statementIndex++ {
			if _, execErr := batchResults.Exec(); execErr != nil {
				return common.NewInternalServerError("AASREG-PGBATCH-EXEC " + execErr.Error())
			}
		}
		var createdAt time.Time
		if scanErr := batchResults.QueryRow().Scan(&createdAt); scanErr != nil {
			return common.NewInternalServerError("AASREG-PGBATCH-READCREATEDAT " + scanErr.Error())
		}
		result.CreatedAt = &createdAt
		return nil
	})
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	return result, nil
}

// InsertAdministrationShellDescriptorInTransaction inserts the provided AAS
// descriptor in the provided transaction.
func (p *PostgreSQLAASRegistryDatabase) InsertAdministrationShellDescriptorInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	aasd model.AssetAdministrationShellDescriptor,
) error {
	if tx == nil {
		return common.NewInternalServerError("AASREG-INSERTAASDESC-NILTX transaction must not be nil")
	}
	if err := descriptors.InsertAdministrationShellDescriptorTx(ctx, tx, aasd); err != nil {
		return err
	}
	stored, err := descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id)
	if err != nil {
		return err
	}
	return appendDescriptorHistoryTx(ctx, tx, stored, history.ChangeCreated, false)
}

// InsertAdministrationShellDescriptorsInTransaction inserts multiple AAS
// descriptors and their descriptor graph rows in the provided transaction. The
// method builds table-oriented multi-row statements, executes them as one
// batched block, then appends history entries for the stored descriptors. It
// returns the index of the descriptor that failed during post-insert readback or
// history processing; when the batched insert itself fails before an item can be
// identified, the index is 0.
func (p *PostgreSQLAASRegistryDatabase) InsertAdministrationShellDescriptorsInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	aasDescriptors []model.AssetAdministrationShellDescriptor,
) (int, error) {
	if tx == nil {
		return 0, common.NewInternalServerError("AASREG-BULKINSERT-NILTX transaction must not be nil")
	}
	batch, err := descriptors.BuildAdministrationShellDescriptorsCreateBatch(ctx, tx, aasDescriptors)
	if err != nil {
		return 0, err
	}
	if err = common.ExecutePostgreSQLBatchInTransaction(ctx, tx, batch.Statements()); err != nil {
		return 0, err
	}

	for index, descriptor := range aasDescriptors {
		stored, getErr := descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, descriptor.Id)
		if getErr != nil {
			if common.IsErrNotFound(getErr) {
				return index, common.NewErrDenied("AAS Descriptor access not allowed")
			}
			return index, getErr
		}
		if historyErr := appendDescriptorHistoryTx(ctx, tx, stored, history.ChangeCreated, false); historyErr != nil {
			return index, historyErr
		}
	}
	return -1, nil
}

// ExistingAASDescriptorIDsInTransaction returns the subset of identifiers that
// already exist in aas_descriptor using the caller's transaction. The result map
// is keyed by AAS identifier so bulk create validation can report the first
// conflicting input item without loading full descriptors.
func (p *PostgreSQLAASRegistryDatabase) ExistingAASDescriptorIDsInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	identifiers []string,
) (map[string]struct{}, error) {
	if tx == nil {
		return nil, common.NewInternalServerError("AASREG-BULKEXISTS-NILTX transaction must not be nil")
	}
	if len(identifiers) == 0 {
		return map[string]struct{}{}, nil
	}

	query, args, err := goqu.
		From(common.TblAASDescriptor).
		Select(common.ColAASID).
		Where(goqu.C(common.ColAASID).In(identifiers)).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("AASREG-BULKEXISTS-BUILDSQL " + err.Error())
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("AASREG-BULKEXISTS-EXECQUERY " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	existing := make(map[string]struct{})
	for rows.Next() {
		var identifier string
		if scanErr := rows.Scan(&identifier); scanErr != nil {
			return nil, common.NewInternalServerError("AASREG-BULKEXISTS-SCANID " + scanErr.Error())
		}
		existing[identifier] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("AASREG-BULKEXISTS-ITERATE " + err.Error())
	}
	return existing, nil
}

// GetAssetAdministrationShellDescriptorByID returns the AAS descriptor
// identified by the given AAS ID.
func (p *PostgreSQLAASRegistryDatabase) GetAssetAdministrationShellDescriptorByID(
	ctx context.Context,
	aasIdentifier string,
) (model.AssetAdministrationShellDescriptor, error) {
	return descriptors.GetAssetAdministrationShellDescriptorByID(ctx, p.db, aasIdentifier)
}

// GetAssetAdministrationShellDescriptorByIDInTransaction returns the AAS descriptor
// identified by the given AAS ID using the provided transaction.
func (p *PostgreSQLAASRegistryDatabase) GetAssetAdministrationShellDescriptorByIDInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	aasIdentifier string,
) (model.AssetAdministrationShellDescriptor, error) {
	if tx == nil {
		return model.AssetAdministrationShellDescriptor{}, common.NewInternalServerError("AASREG-GETAASDESC-NILTX transaction must not be nil")
	}

	return descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasIdentifier)
}

// DeleteAssetAdministrationShellDescriptorByID deletes the AAS descriptor
// identified by the given AAS ID.
func (p *PostgreSQLAASRegistryDatabase) DeleteAssetAdministrationShellDescriptorByID(
	ctx context.Context,
	aasIdentifier string,
) error {
	return common.ExecuteInTransaction(p.db, "AASREG-DELAASDESC-STARTTX", "AASREG-DELAASDESC-COMMIT", func(tx *sql.Tx) error {
		existing, err := descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasIdentifier)
		if err != nil {
			return err
		}
		if err := descriptors.DeleteAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasIdentifier); err != nil {
			return err
		}
		return appendDescriptorHistoryTx(ctx, tx, existing, history.ChangeDeleted, true)
	})
}

// ReplaceAdministrationShellDescriptor replaces an existing AAS descriptor
// with the given value and reports whether it existed.
func (p *PostgreSQLAASRegistryDatabase) ReplaceAdministrationShellDescriptor(
	ctx context.Context,
	aasd model.AssetAdministrationShellDescriptor,
) (model.AssetAdministrationShellDescriptor, error) {
	var result model.AssetAdministrationShellDescriptor
	err := common.ExecuteInTransaction(p.db, "AASREG-REPLACEAASDESC-STARTTX", "AASREG-REPLACEAASDESC-COMMIT", func(tx *sql.Tx) error {
		if _, err := descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id); err != nil {
			return err
		}
		if err := descriptors.DeleteAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id); err != nil {
			return err
		}
		if err := descriptors.InsertAdministrationShellDescriptorTx(ctx, tx, aasd); err != nil {
			return err
		}
		stored, err := descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id)
		if err != nil {
			return err
		}
		if err := appendDescriptorHistoryTx(ctx, tx, stored, history.ChangeUpdated, false); err != nil {
			return err
		}
		result = stored
		return nil
	})
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	return result, nil
}

// UpsertAdministrationShellDescriptorInTransaction replaces an existing AAS
// descriptor or inserts it when missing in the provided transaction.
func (p *PostgreSQLAASRegistryDatabase) UpsertAdministrationShellDescriptorInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	aasd model.AssetAdministrationShellDescriptor,
) error {
	if tx == nil {
		return common.NewInternalServerError("AASREG-UPSERTAASDESC-NILTX transaction must not be nil")
	}

	created, err := descriptors.UpsertAdministrationShellDescriptorTx(ctx, tx, aasd)
	if err != nil {
		return err
	}

	stored, err := descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id)
	if err != nil {
		return err
	}
	if created {
		return appendDescriptorHistoryTx(ctx, tx, stored, history.ChangeCreated, false)
	}
	return appendDescriptorHistoryTx(ctx, tx, stored, history.ChangeUpdated, false)
}

// DeleteAssetAdministrationShellDescriptorByIDInTransaction deletes an AAS
// descriptor by id in the provided transaction.
func (p *PostgreSQLAASRegistryDatabase) DeleteAssetAdministrationShellDescriptorByIDInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	aasIdentifier string,
) error {
	if tx == nil {
		return common.NewInternalServerError("AASREG-DELAASDESC-NILTX transaction must not be nil")
	}

	existing, err := descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasIdentifier)
	if err != nil {
		return err
	}
	if err := descriptors.DeleteAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasIdentifier); err != nil {
		return err
	}
	return appendDescriptorHistoryTx(ctx, tx, existing, history.ChangeDeleted, true)
}

// GetAssetAdministrationShellDescriptorRecentChanges returns descriptor history rows for recent-change APIs.
func (p *PostgreSQLAASRegistryDatabase) GetAssetAdministrationShellDescriptorRecentChanges(ctx context.Context, limit int32, cursor string, createdFrom time.Time, updatedFrom time.Time) ([]history.Row, string, error) {
	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return nil, "", common.NewInternalServerError("AASREG-RECENT-SHOULDENFORCE " + enforceErr.Error())
	}
	if !shouldEnforceFormula {
		return history.RecentRows(ctx, p.db, history.TableDescriptor, limit, cursor, createdFrom, updatedFrom)
	}

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREG-RECENT-BADCOLLECTOR " + err.Error())
	}
	visibilityDS := goqu.From(common.TDescriptor).
		InnerJoin(
			common.TAASDescriptor,
			goqu.On(common.TAASDescriptor.Col(common.ColDescriptorID).Eq(common.TDescriptor.Col(common.ColID))),
		).
		Select(goqu.V(1)).
		Where(common.TAASDescriptor.Col(common.ColAASID).Eq(goqu.I("history.identifier")))

	return history.RecentRowsForVisibleIdentifiables(ctx, p.db, history.TableDescriptor, limit, cursor, createdFrom, updatedFrom, visibilityDS, collector)
}

// ListAssetAdministrationShellDescriptors lists AAS descriptors with optional
// pagination and asset filtering, returning a next-page cursor when present.
func (p *PostgreSQLAASRegistryDatabase) ListAssetAdministrationShellDescriptors(
	ctx context.Context,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
) ([]model.AssetAdministrationShellDescriptor, string, error) {
	return descriptors.ListAssetAdministrationShellDescriptors(ctx, p.db, limit, cursor, assetKind, assetType, "")
}

// ListSubmodelDescriptorsForAAS lists submodel descriptors for a given AAS ID
// with optional pagination, returning a next-page cursor when present.
func (p *PostgreSQLAASRegistryDatabase) ListSubmodelDescriptorsForAAS(
	ctx context.Context,
	aasID string,
	limit int32,
	cursor string,
) ([]model.SubmodelDescriptor, string, error) {
	return descriptors.ListSubmodelDescriptorsForAAS(ctx, p.db, aasID, limit, cursor)
}

// InsertSubmodelDescriptorForAAS inserts a submodel descriptor and associates
// it with the specified AAS ID.
func (p *PostgreSQLAASRegistryDatabase) InsertSubmodelDescriptorForAAS(
	ctx context.Context,
	aasID string,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	var result model.SubmodelDescriptor
	err := common.ExecuteInTransaction(p.db, "AASREG-INSERTSMDESCFORAAS-STARTTX", "AASREG-INSERTSMDESCFORAAS-COMMIT", func(tx *sql.Tx) error {
		stored, err := descriptors.InsertSubmodelDescriptorForAASTx(ctx, tx, aasID, submodel)
		if err != nil {
			return err
		}
		if err := p.appendAddedSubmodelDescriptorHistoryTx(ctx, tx, aasID, stored); err != nil {
			return err
		}
		result = stored
		return nil
	})
	if err != nil {
		return model.SubmodelDescriptor{}, err
	}
	return result, nil
}

// ReplaceSubmodelDescriptorForAAS replaces a submodel descriptor for the given
// AAS ID and reports whether it existed.
func (p *PostgreSQLAASRegistryDatabase) ReplaceSubmodelDescriptorForAAS(
	ctx context.Context,
	aasID string,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	var result model.SubmodelDescriptor
	err := common.ExecuteInTransaction(p.db, "AASREG-REPLACESMDESCFORAAS-STARTTX", "AASREG-REPLACESMDESCFORAAS-COMMIT", func(tx *sql.Tx) error {
		if err := descriptors.DeleteSubmodelDescriptorForAASByIDTx(ctx, tx, aasID, submodel.Id); err != nil {
			return err
		}
		stored, err := descriptors.InsertSubmodelDescriptorForAASTx(ctx, tx, aasID, submodel)
		if err != nil {
			return err
		}
		if err := p.appendReplacedSubmodelDescriptorHistoryTx(ctx, tx, aasID, stored); err != nil {
			return err
		}
		result = stored
		return nil
	})
	if err != nil {
		return model.SubmodelDescriptor{}, err
	}
	return result, nil
}

// GetSubmodelDescriptorForAASByID returns the submodel descriptor identified
// by the submodel ID for the specified AAS ID.
func (p *PostgreSQLAASRegistryDatabase) GetSubmodelDescriptorForAASByID(
	ctx context.Context,
	aasID string,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	return descriptors.GetSubmodelDescriptorForAASByID(ctx, p.db, aasID, submodelID)
}

// DeleteSubmodelDescriptorForAASByID deletes the submodel descriptor identified
// by submodel ID for the specified AAS ID.
func (p *PostgreSQLAASRegistryDatabase) DeleteSubmodelDescriptorForAASByID(
	ctx context.Context,
	aasID string,
	submodelID string,
) error {
	return common.ExecuteInTransaction(p.db, "AASREG-DELSMDESCFORAAS-STARTTX", "AASREG-DELSMDESCFORAAS-COMMIT", func(tx *sql.Tx) error {
		if _, err := descriptors.GetSubmodelDescriptorForAASByID(ctx, tx, aasID, submodelID); err != nil {
			return err
		}
		if err := descriptors.DeleteSubmodelDescriptorForAASByIDTx(ctx, tx, aasID, submodelID); err != nil {
			return err
		}
		return p.appendRemovedSubmodelDescriptorHistoryTx(ctx, tx, aasID, submodelID)
	})
}

// ExistsAASByID reports whether an AAS with the given ID exists.
func (p *PostgreSQLAASRegistryDatabase) ExistsAASByID(
	ctx context.Context,
	aasID string,
) (bool, error) {
	return descriptors.ExistsAASByID(ctx, p.db, aasID)
}

// ExistsSubmodelForAAS reports whether the given submodel ID exists for the
// specified AAS ID.
func (p *PostgreSQLAASRegistryDatabase) ExistsSubmodelForAAS(
	ctx context.Context,
	aasID,
	submodelID string,
) (bool, error) {
	return descriptors.ExistsSubmodelForAAS(ctx, p.db, aasID, submodelID)
}
