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

// Package descriptors contains the data‑access helpers that read and write
// Asset Administration Shell (AAS) and Submodel descriptor data to a
// PostgreSQL database.
//
// The package focuses on:
//   - Clear transaction boundaries for write operations (insert, replace, delete)
//   - Efficient batched reads that assemble fully materialized descriptors
//     (including semantic references, administrative information, display names,
//     descriptions, endpoints, specific asset IDs, extensions and supplemental
//     semantic IDs)
//   - Concurrent fan‑out of dependent lookups using errgroup to reduce latency
//   - Cursor‑based pagination for list operations where applicable
//
// Queries are built with goqu and executed via database/sql. Most read helpers
// return plain model types from internal/common/model so callers can use the
// results directly without further mapping.
// Author: Martin Stemmer ( Fraunhofer IESE )
package descriptors

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// InsertAssetAdministrationShellDescriptor creates a new AssetAdministrationShellDescriptor
// and all its related entities (display name, description, administration,
// endpoints, specific asset IDs, extensions and submodel descriptors).
//
// The operation runs in its own database transaction. If any part of the write
// fails, the transaction is rolled back and no partial data is left behind.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - aasd: descriptor to persist
//
// Returns an error when SQL building/execution fails or when writing any of the
// dependent rows fails. Errors are wrapped into common errors where relevant.
func InsertAssetAdministrationShellDescriptor(ctx context.Context, db *sql.DB, aasd model.AssetAdministrationShellDescriptor) (model.AssetAdministrationShellDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()
	if err = InsertAdministrationShellDescriptorTx(ctx, tx, aasd); err != nil {
		_ = tx.Rollback()
		return model.AssetAdministrationShellDescriptor{}, err
	}
	if CanSkipPostInsertReadback(ctx) {
		return aasd, tx.Commit()
	}
	result, err := GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id)
	if err != nil {
		_ = tx.Rollback()
		return model.AssetAdministrationShellDescriptor{}, err
	}
	return result, tx.Commit()
}

// CanSkipCreateReadback reports whether create readback can be skipped.
//
// Route-level authorization has already run, so readback is only required when
// the request context contains ABAC filters or restricted create formulas.
//
// Parameters:
//   - ctx: Request context carrying the ABAC query filter.
//
// Returns:
//   - bool: True when callers can trust the created descriptor without readback.
func CanSkipCreateReadback(ctx context.Context) bool {
	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter == nil {
		return true
	}
	if len(queryFilter.Filters) > 0 {
		return false
	}
	if len(queryFilter.FormulasByRight) > 0 {
		return auth.HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumCREATE)
	}
	if queryFilter.Formula == nil {
		return true
	}
	return auth.HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumCREATE)
}

// CanSkipPostInsertReadback reports whether post-insert readback can be skipped.
//
// This helper keeps the AAS descriptor-specific name for callers that return
// newly inserted descriptors.
//
// Parameters:
//   - ctx: Request context carrying the ABAC query filter.
//
// Returns:
//   - bool: True when post-insert readback can be skipped.
func CanSkipPostInsertReadback(ctx context.Context) bool {
	return CanSkipCreateReadback(ctx)
}

// CanSkipDeleteReadback reports whether deletion needs no descriptor read for
// authorization. History callers must check mutation recording separately.
func CanSkipDeleteReadback(ctx context.Context) bool {
	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter == nil {
		return true
	}
	if len(queryFilter.Filters) > 0 {
		return false
	}
	if len(queryFilter.FormulasByRight) > 0 {
		return auth.HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumDELETE)
	}
	if queryFilter.Formula == nil {
		return true
	}
	return auth.HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumDELETE)
}

// CanSkipUpdateReadback reports whether the request context has no restrictive
// update formula or fragment filter that requires post-update authorization.
func CanSkipUpdateReadback(ctx context.Context) bool {
	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter == nil {
		return true
	}
	if len(queryFilter.Filters) > 0 {
		return false
	}
	if len(queryFilter.FormulasByRight) > 0 {
		return auth.HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumUPDATE)
	}
	if queryFilter.Formula == nil {
		return true
	}
	return auth.HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumUPDATE)
}

// InsertAdministrationShellDescriptorTx performs the same insert as
// InsertAdministrationShellDescriptor but uses the provided transaction. This allows
// callers to compose multiple writes into a single atomic unit.
//
// The function inserts the base descriptor row first and then creates related
// entities (display name/description/admin info/endpoints/specific IDs/extensions
// and submodel descriptors). If any step fails, the error is returned and the
// caller is responsible for rolling back the transaction.
func InsertAdministrationShellDescriptorTx(ctx context.Context, tx *sql.Tx, aasd model.AssetAdministrationShellDescriptor) error {
	return insertAdministrationShellDescriptorTx(ctx, tx, aasd)
}

func insertAdministrationShellDescriptorTx(ctx context.Context, tx *sql.Tx, aasd model.AssetAdministrationShellDescriptor) error {
	d := goqu.Dialect(common.Dialect)

	descTbl := goqu.T(common.TblDescriptor)

	sqlStr, args, buildErr := d.
		Insert(common.TblDescriptor).
		Returning(descTbl.Col(common.ColID)).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	var descriptorID int64
	if err := tx.QueryRow(sqlStr, args...).Scan(&descriptorID); err != nil {
		return err
	}

	return insertAdministrationShellDescriptorDetailsTx(ctx, tx, descriptorID, aasd, true)
}

func insertAdministrationShellDescriptorDetailsTx(ctx context.Context, tx *sql.Tx, descriptorID int64, aasd model.AssetAdministrationShellDescriptor, insertAASDescriptor bool) error {
	d := goqu.Dialect(common.Dialect)

	descriptionPayload, err := buildLangStringTextPayload(aasd.Description)
	if err != nil {
		return common.NewInternalServerError("AASDESC-INSERT-DESCRIPTIONPAYLOAD")
	}
	displayNamePayload, err := buildLangStringNamePayload(aasd.DisplayName)
	if err != nil {
		return common.NewInternalServerError("AASDESC-INSERT-DISPLAYNAMEPAYLOAD")
	}
	administrationPayload, err := buildAdministrativeInfoPayload(aasd.Administration)
	if err != nil {
		return common.NewInternalServerError("AASDESC-INSERT-ADMINPAYLOAD")
	}
	extensionsPayload, err := buildExtensionsPayload(aasd.Extensions)
	if err != nil {
		return common.NewInternalServerError("AASDESC-INSERT-EXTENSIONPAYLOAD")
	}

	sqlStr, args, buildErr := d.
		Insert(common.TblDescriptorPayload).
		Rows(goqu.Record{
			common.ColDescriptorID:              descriptorID,
			common.ColDescriptionPayload:        goqu.L("?::jsonb", string(descriptionPayload)),
			common.ColDisplayNamePayload:        goqu.L("?::jsonb", string(displayNamePayload)),
			common.ColAdministrativeInfoPayload: goqu.L("?::jsonb", string(administrationPayload)),
			common.ColExtensionsPayload:         goqu.L("?::jsonb", string(extensionsPayload)),
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	if insertAASDescriptor {
		sqlStr, args, buildErr = d.
			Insert(common.TblAASDescriptor).
			Rows(buildAASDescriptorInsertRecord(ctx, descriptorID, aasd)).
			ToSQL()
		if buildErr != nil {
			return buildErr
		}
		if _, err = tx.Exec(sqlStr, args...); err != nil {
			return err
		}
	}

	if err = CreateEndpoints(tx, descriptorID, aasd.Endpoints); err != nil {
		return err
	}

	var aasRef sql.NullInt64
	if cfg, ok := common.ConfigFromContext(ctx); ok && cfg.General.DiscoveryIntegration {
		ref, err := ensureAASIdentifierTx(ctx, tx, aasd.Id)
		if err != nil {
			return err
		}
		aasRef = sql.NullInt64{Int64: ref, Valid: true}
	}

	specificAssetIds := specificAssetIDsWithGlobalAssetID(ctx, aasd)

	if err = common.CreateSpecificAssetIDDescriptor(tx, descriptorID, aasRef, specificAssetIds); err != nil {
		return err
	}

	return createSubModelDescriptors(tx, sql.NullInt64{Int64: descriptorID, Valid: true}, aasd.SubmodelDescriptors)
}

// UpsertAdministrationShellDescriptorTx upserts an AssetAdministrationShellDescriptor
// within the provided transaction. It first acquires an advisory lock scoped to
// the AAS Id to prevent concurrent upserts for the same AAS. Then it attempts
// to locate the internal descriptor id for the given AAS Id using a SELECT
// ... FOR UPDATE to lock the row. If a descriptor is found, the function
// replaces the descriptor details; otherwise it inserts a new descriptor and
// related rows.
//
// The function returns a boolean indicating whether a new descriptor was
// created (true) or an existing descriptor was replaced (false), and an error
// when the operation fails. The caller is responsible for committing or
// rolling back the supplied transaction `tx`.
//
// Parameters:
//   - ctx: context for cancellation and query filtering.
//   - tx: database transaction to use for the upsert (must be non-nil).
//   - aasd: the AssetAdministrationShellDescriptor to insert or replace.
//
// Note: This function relies on advisory locks and FOR UPDATE row locking to
// avoid race conditions; it must be invoked inside a transaction.
func UpsertAdministrationShellDescriptorTx(ctx context.Context, tx *sql.Tx, aasd model.AssetAdministrationShellDescriptor) (bool, error) {
	if err := lockAASDescriptorUpsertTx(ctx, tx, aasd.Id); err != nil {
		return false, err
	}

	descriptorID, found, err := selectAASDescriptorIDForUpdateTx(ctx, tx, aasd.Id)
	if err != nil {
		return false, err
	}

	if found {
		if err = replaceAdministrationShellDescriptorDetailsTx(ctx, tx, descriptorID, aasd); err != nil {
			return false, err
		}
		return false, nil
	}

	return true, InsertAdministrationShellDescriptorTx(ctx, tx, aasd)
}

// GetAASDescriptorCreatedAtByIDTx returns and locks the persisted AAS descriptor
// creation timestamp so replace operations can preserve it across delete/insert.
func GetAASDescriptorCreatedAtByIDTx(ctx context.Context, tx *sql.Tx, aasID string) (time.Time, error) {
	d := goqu.Dialect(common.Dialect)
	aasTbl := goqu.T(common.TblAASDescriptor)

	sqlStr, args, buildErr := d.
		From(aasTbl).
		Select(aasTbl.Col(common.ColCreatedAt)).
		Where(aasTbl.Col(common.ColAASID).Eq(aasID)).
		ForUpdate(goqu.Wait).
		ToSQL()
	if buildErr != nil {
		return time.Time{}, buildErr
	}

	var createdAt time.Time
	if err := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, common.NewErrNotFound("AAS Descriptor not found")
		}
		return time.Time{}, err
	}

	return createdAt, nil
}

func lockAASDescriptorUpsertTx(ctx context.Context, tx *sql.Tx, aasID string) error {
	sqlStr, args, err := buildAASDescriptorUpsertLockSQL(aasID)
	if err != nil {
		return common.NewInternalServerError("AASDESC-LOCKAASUPSERT-BUILDSQL " + err.Error())
	}

	if _, err = tx.ExecContext(ctx, sqlStr, args...); err != nil {
		return common.NewInternalServerError("AASDESC-LOCKAASUPSERT-EXECSQL " + err.Error())
	}
	return nil
}

func buildAASDescriptorUpsertLockSQL(aasID string) (string, []any, error) {
	return goqu.
		Dialect(common.Dialect).
		Select(goqu.Func("pg_advisory_xact_lock", goqu.Func("hashtextextended", "aas_descriptor:"+aasID, int64(0)))).
		Prepared(true).
		ToSQL()
}

func selectAASDescriptorIDForUpdateTx(ctx context.Context, tx *sql.Tx, aasID string) (int64, bool, error) {
	d := goqu.Dialect(common.Dialect)
	aasTbl := goqu.T(common.TblAASDescriptor)

	sqlStr, args, buildErr := d.
		From(aasTbl).
		Select(aasTbl.Col(common.ColDescriptorID)).
		Where(aasTbl.Col(common.ColAASID).Eq(aasID)).
		ForUpdate(goqu.Wait).
		ToSQL()
	if buildErr != nil {
		return 0, false, buildErr
	}

	var descriptorID int64
	if err := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descriptorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}

	return descriptorID, true, nil
}

func replaceAdministrationShellDescriptorDetailsTx(ctx context.Context, tx *sql.Tx, descriptorID int64, aasd model.AssetAdministrationShellDescriptor) error {
	if err := updateAASDescriptorRowTx(ctx, tx, descriptorID, aasd); err != nil {
		return err
	}
	if err := deleteAdministrationShellDescriptorDetailsTx(ctx, tx, descriptorID); err != nil {
		return err
	}
	return insertAdministrationShellDescriptorDetailsTx(ctx, tx, descriptorID, aasd, false)
}

func updateAASDescriptorRowTx(ctx context.Context, tx *sql.Tx, descriptorID int64, aasd model.AssetAdministrationShellDescriptor) error {
	d := goqu.Dialect(common.Dialect)
	record := buildAASDescriptorUpdateRecord(ctx, descriptorID, aasd)

	sqlStr, args, buildErr := d.
		Update(common.TblAASDescriptor).
		Set(record).
		Where(goqu.C(common.ColDescriptorID).Eq(descriptorID)).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}

	_, err := tx.ExecContext(ctx, sqlStr, args...)
	return err
}

func deleteAdministrationShellDescriptorDetailsTx(ctx context.Context, tx *sql.Tx, descriptorID int64) error {
	d := goqu.Dialect(common.Dialect)

	childDescriptorIDs := d.
		From(common.TblSubmodelDescriptor).
		Select(common.ColDescriptorID).
		Where(goqu.C(common.ColAASDescriptorID).Eq(descriptorID))
	if err := deleteDescriptorRowsBySelectTx(ctx, tx, childDescriptorIDs); err != nil {
		return err
	}

	for _, tableName := range []string{
		common.TblAASDescriptorEndpoint,
		common.TblSpecificAssetID,
		common.TblDescriptorPayload,
	} {
		sqlStr, args, buildErr := d.
			Delete(tableName).
			Where(goqu.C(common.ColDescriptorID).Eq(descriptorID)).
			ToSQL()
		if buildErr != nil {
			return buildErr
		}
		if _, err := tx.ExecContext(ctx, sqlStr, args...); err != nil {
			return err
		}
	}

	return nil
}

func deleteDescriptorRowsBySelectTx(ctx context.Context, tx *sql.Tx, descriptorIDs *goqu.SelectDataset) error {
	d := goqu.Dialect(common.Dialect)
	sqlStr, args, buildErr := d.
		Delete(common.TblDescriptor).
		Where(goqu.C(common.ColID).In(descriptorIDs)).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	_, err := tx.ExecContext(ctx, sqlStr, args...)
	return err
}

// buildAASDescriptorInsertRecord accepts either a concrete descriptor id or a
// Goqu sequence expression, because batch inserts reference ids before they are
// scanned back as int64 values.
func buildAASDescriptorInsertRecord(
	ctx context.Context,
	descriptorID any,
	aasd model.AssetAdministrationShellDescriptor,
) goqu.Record {
	record := goqu.Record{
		common.ColDescriptorID:  descriptorID,
		common.ColAssetKind:     aasd.AssetKind,
		common.ColAssetType:     aasd.AssetType,
		common.ColGlobalAssetID: aasd.GlobalAssetId,
		common.ColIDShort:       aasd.IdShort,
		common.ColAASID:         aasd.Id,
	}

	if allowAASDescriptorCreatedAtOverrideFromContext(ctx) && aasd.CreatedAt != nil {
		record[common.ColCreatedAt] = *aasd.CreatedAt
	}

	return record
}

func buildAASDescriptorUpdateRecord(
	ctx context.Context,
	descriptorID any,
	aasd model.AssetAdministrationShellDescriptor,
) goqu.Record {
	record := buildAASDescriptorInsertRecord(ctx, descriptorID, aasd)
	delete(record, common.ColDescriptorID)
	delete(record, common.ColCreatedAt)
	return record
}

// GetAssetAdministrationShellDescriptorByID returns a fully materialized
// AssetAdministrationShellDescriptor by its AAS Id string.
//
// The function loads optional related entities (administration, display name,
// description, endpoints, specific asset IDs, extensions and submodel
// descriptors) concurrently to minimize latency. If the AAS does not exist, a
// NotFound error is returned.
func GetAssetAdministrationShellDescriptorByID(
	ctx context.Context, db *sql.DB, aasIdentifier string,
) (model.AssetAdministrationShellDescriptor, error) {
	result, _, err := listAssetAdministrationShellDescriptorsSingleStatement(ctx, db, 1, "", "", "", aasIdentifier, time.Time{}, time.Time{})
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	if len(result) != 1 {
		return model.AssetAdministrationShellDescriptor{}, common.NewErrNotFound("AAS Descriptor not found")
	}
	return result[0], nil
}

// GetAssetAdministrationShellDescriptorByIDTx returns a fully materialized
// AssetAdministrationShellDescriptor by its AAS Id string using the provided
// transaction. It avoids concurrent queries, which are unsafe on *sql.Tx.
func GetAssetAdministrationShellDescriptorByIDTx(
	ctx context.Context, tx *sql.Tx, aasIdentifier string,
) (model.AssetAdministrationShellDescriptor, error) {
	result, _, err := listAssetAdministrationShellDescriptorsSingleStatement(ctx, tx, 1, "", "", "", aasIdentifier, time.Time{}, time.Time{})
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	if len(result) != 1 {
		return model.AssetAdministrationShellDescriptor{}, common.NewErrNotFound("AAS Descriptor not found")
	}
	return result[0], nil
}

// DeleteAssetAdministrationShellDescriptorByID deletes the descriptor for the
// given AAS Id string. Deletion happens on the base descriptor row with ON
// DELETE CASCADE removing dependent rows.
//
// The delete runs in its own transaction.
func DeleteAssetAdministrationShellDescriptorByID(ctx context.Context, db *sql.DB, aasIdentifier string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasIdentifier)
	if err != nil {
		return err
	}

	err = deleteAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasIdentifier)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// DeleteAssetAdministrationShellDescriptorByIDTx deletes a descriptor by AAS id
// using the provided transaction.
func DeleteAssetAdministrationShellDescriptorByIDTx(ctx context.Context, tx *sql.Tx, aasIdentifier string) error {
	return deleteAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasIdentifier)
}

// DeleteAssetAdministrationShellDescriptorsByIDsTx deletes AAS descriptors by id.
//
// The function deletes AAS descriptor base rows and embedded submodel descriptor
// rows in bounded chunks.
//
// Parameters:
//   - ctx: Request context carrying configuration data.
//   - tx: Transaction used for deletion.
//   - aasIdentifiers: AAS descriptor identifiers to delete.
//
// Returns:
//   - error: Error when SQL rendering or deletion fails.
func DeleteAssetAdministrationShellDescriptorsByIDsTx(ctx context.Context, tx *sql.Tx, aasIdentifiers []string) error {
	if len(aasIdentifiers) == 0 {
		return nil
	}
	d := goqu.Dialect(common.Dialect)
	batch := &common.PostgreSQLBatch{}
	limit := common.BulkBatchLimitFromContext(ctx)
	for start := 0; start < len(aasIdentifiers); start += limit {
		end := min(start+limit, len(aasIdentifiers))
		descriptorIDs := d.
			From(common.TblAASDescriptor).
			Select(common.ColDescriptorID).
			Where(goqu.C(common.ColAASID).In(aasIdentifiers[start:end]))
		childDescriptorIDs := d.
			From(common.TblSubmodelDescriptor).
			Select(common.ColDescriptorID).
			Where(goqu.C(common.ColAASDescriptorID).In(descriptorIDs))
		if err := batch.AppendDataset(
			d.Delete(common.TblDescriptor).Where(goqu.C(common.ColID).In(childDescriptorIDs)),
		); err != nil {
			return common.NewInternalServerError("AASDESC-BULKDELETE-BUILDCHILDSQL " + err.Error())
		}
		if err := batch.AppendDataset(
			d.Delete(common.TblDescriptor).Where(goqu.C(common.ColID).In(descriptorIDs)),
		); err != nil {
			return common.NewInternalServerError("AASDESC-BULKDELETE-BUILDPARENTSQL " + err.Error())
		}
	}
	return common.ExecutePostgreSQLBatchInTransaction(ctx, tx, batch.Statements())
}

// DeleteAssetAdministrationShellDescriptorByIDTx deletes using the provided
// transaction. It resolves the internal descriptor id and removes the base
// descriptor row plus descriptor rows of linked submodel descriptors.
// Dependent rows are removed via ON DELETE CASCADE.
func deleteAssetAdministrationShellDescriptorByIDTx(ctx context.Context, tx *sql.Tx, aasIdentifier string) error {
	d := goqu.Dialect(common.Dialect)
	aas := goqu.T(common.TblAASDescriptor).As("aas")

	sqlStr, args, buildErr := d.
		From(aas).
		Select(aas.Col(common.ColDescriptorID)).
		Where(aas.Col(common.ColAASID).Eq(aasIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}

	var descID int64
	if scanErr := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return common.NewErrNotFound("AAS Descriptor not found")
		}
		return scanErr
	}

	childDescriptorIDs := d.
		From(common.TblSubmodelDescriptor).
		Select(common.ColDescriptorID).
		Where(goqu.C(common.ColAASDescriptorID).Eq(descID))

	batch := &common.PostgreSQLBatch{}
	if err := batch.AppendDataset(d.Delete(common.TblDescriptor).Where(goqu.C(common.ColID).In(childDescriptorIDs))); err != nil {
		return common.NewInternalServerError("AASDESC-DELETE-BUILDCHILDSQL " + err.Error())
	}
	if err := batch.AppendDataset(d.Delete(common.TblDescriptor).Where(goqu.C(common.ColID).Eq(descID))); err != nil {
		return common.NewInternalServerError("AASDESC-DELETE-BUILDPARENTSQL " + err.Error())
	}
	return common.ExecutePostgreSQLBatchInTransaction(ctx, tx, batch.Statements())
}

// ReplaceAdministrationShellDescriptor atomically replaces the descriptor with
// the same AAS Id: if a descriptor exists it is deleted (base descriptor row),
// then the provided descriptor is inserted. Related rows are recreated from the
// input. The returned descriptor is the stored AssetAdministrationShellDescriptor
// after replacement.
func ReplaceAdministrationShellDescriptor(ctx context.Context, db *sql.DB, aasd model.AssetAdministrationShellDescriptor) (model.AssetAdministrationShellDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()

	// first check if user is allowed to replace
	if _, err = GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id); err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	createdAt, err := GetAASDescriptorCreatedAtByIDTx(ctx, tx, aasd.Id)
	if err != nil {
		_ = tx.Rollback()
		return model.AssetAdministrationShellDescriptor{}, err
	}
	aasd.CreatedAt = &createdAt
	// delete existing descriptor
	if err = deleteAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id); err != nil {
		_ = tx.Rollback()
		return model.AssetAdministrationShellDescriptor{}, err
	}
	// insert new descriptor
	if err = InsertAdministrationShellDescriptorTx(WithAllowAASDescriptorCreatedAtOverride(ctx), tx, aasd); err != nil {
		_ = tx.Rollback()
		return model.AssetAdministrationShellDescriptor{}, err
	}
	// check if user is allowed to write the new descriptor
	result, err := GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id)
	if err != nil {
		_ = tx.Rollback()
		return model.AssetAdministrationShellDescriptor{}, err
	}
	return result, tx.Commit()
}

func buildListAASDescriptorPageQuery(
	ctx context.Context,
	peekLimit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	identifiable string,
	createdFrom time.Time,
	updatedFrom time.Time,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	if peekLimit < 0 {
		return nil, common.NewErrBadRequest("Limit must not be negative")
	}

	d := goqu.Dialect(common.Dialect)
	ds := d.From(common.TDescriptor).
		InnerJoin(
			common.TAASDescriptor,
			goqu.On(common.TAASDescriptor.Col(common.ColDescriptorID).Eq(common.TDescriptor.Col(common.ColID))),
		).
		Select(
			common.TDescriptor.Col(common.ColID).As("descriptor_id"),
			common.TAASDescriptor.Col(common.ColAASID).As("sort_aas_id"),
		)

	ds, err := auth.AddFormulaQueryFromContext(ctx, ds, collector)
	if err != nil {
		return nil, err
	}

	if cursor != "" {
		ds = ds.Where(common.TAASDescriptor.Col(common.ColAASID).Gte(cursor))
	}

	if assetType != "" {
		ds = ds.Where(common.TAASDescriptor.Col(common.ColAssetType).Eq(assetType))
	}

	if assetKind != "" {
		assetKindAsString := model.GetAssetKindString(assetKind)
		convertedAssetKind, ok := stringification.AssetKindFromString(assetKindAsString)
		if !ok {
			return nil, errors.New("Invalid asset kind: " + assetKindAsString)
		}
		ds = ds.Where(common.TAASDescriptor.Col(common.ColAssetKind).Eq(convertedAssetKind))
	}

	if identifiable != "" {
		ds = ds.Where(common.TAASDescriptor.Col(common.ColID).Eq(identifiable))
	}
	switch {
	case !createdFrom.IsZero() && !updatedFrom.IsZero():
		ds = ds.Where(goqu.Or(
			common.TAASDescriptor.Col("administration_created_at").Gte(createdFrom.UTC()),
			common.TAASDescriptor.Col("administration_updated_at").Gte(updatedFrom.UTC()),
		))
	case !createdFrom.IsZero():
		ds = ds.Where(common.TAASDescriptor.Col("administration_created_at").Gte(createdFrom.UTC()))
	case !updatedFrom.IsZero():
		ds = ds.Where(common.TAASDescriptor.Col("administration_updated_at").Gte(updatedFrom.UTC()))
	}

	ds = ds.
		Order(common.TAASDescriptor.Col(common.ColAASID).Asc()).
		Limit(uint(peekLimit))

	return ds, nil
}

// ListAssetAdministrationShellDescriptors lists AAS descriptors with optional
// filtering by AssetKind and AssetType. Results are ordered by AAS Id
// ascending and support cursor‑based pagination where the cursor is the AAS Id
// of the first element to include (i.e. Id >= cursor).
//
// It returns the page of fully assembled descriptors and, when more results are
// available, a next cursor value (the Id immediately after the page). When
// limit <= 0, a default page size of 100 is applied.
//
//nolint:revive // Its only 31 instead of 30 - 1 is okay
func ListAssetAdministrationShellDescriptors(
	ctx context.Context,
	db DBQueryer,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	identifiable string,
	createdFrom time.Time,
	updatedFrom time.Time,
) ([]model.AssetAdministrationShellDescriptor, string, error) {
	if debugEnabled(ctx) {
		defer func(start time.Time) {
			slog.DebugContext(ctx, "AAS descriptor list completed", "duration", time.Since(start))
		}(time.Now())
	}
	if !hasAASDescriptorFilters(ctx) {
		return listAssetAdministrationShellDescriptorsBatched(
			ctx,
			db,
			limit,
			cursor,
			assetKind,
			assetType,
			identifiable,
			createdFrom,
			updatedFrom,
		)
	}
	return listAssetAdministrationShellDescriptorsSingleStatement(
		ctx,
		db,
		limit,
		cursor,
		assetKind,
		assetType,
		identifiable,
		createdFrom,
		updatedFrom,
	)
}

// ExistsAASByID performs a lightweight existence check for an AAS by its Id
// string. It returns true when a descriptor exists, false when it does not.
func ExistsAASByID(ctx context.Context, db *sql.DB, aasID string) (bool, error) {
	return existsAASByID(ctx, db, aasID)
}

func existsAASByID(ctx context.Context, db DBQueryer, aasID string) (bool, error) {
	d := goqu.Dialect(common.Dialect)
	aas := goqu.T(common.TblAASDescriptor).As("aas")

	ds := d.From(aas).Select(goqu.L("1")).Where(aas.Col(common.ColAASID).Eq(aasID)).Limit(1)
	sqlStr, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		return false, err
	}

	var one int
	if scanErr := db.QueryRowContext(ctx, sqlStr, args...).Scan(&one); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return false, nil
		}
		return false, scanErr
	}
	return true, nil
}
