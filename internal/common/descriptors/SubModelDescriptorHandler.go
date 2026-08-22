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

package descriptors

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// ListSubmodelDescriptorsForAAS lists the SubmodelDescriptors that belong to a
// single AAS (identified by its AAS Id string). The result is ordered by
// Submodel Id ascending and supports cursor-based pagination using the
// Submodel Id as the cursor.
//
// Cursor semantics:
//   - When cursor != "", only submodels with Id >= cursor are included.
//   - nextCursor, when non-empty, is the Id of the first element after the
//     returned page.
//
// Implementation details:
//   - The function resolves the internal AAS descriptor id and then selects the
//     authorized child page before correlated child materialization.
//   - Parent lookup and page materialization are executed by the caller in one
//     read-only repeatable-read transaction.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - aasID: AAS Id string owning the submodels
//   - limit: maximum number of items to return (<=0 uses default page size 100)
//   - cursor: optional Submodel Id to start from (inclusive)
//
// Returns the page of submodel descriptors and an optional next cursor when
// additional items are available.
func ListSubmodelDescriptorsForAAS(
	ctx context.Context,
	db DBQueryer,
	aasID string,
	limit int32,
	cursor string,
) ([]model.SubmodelDescriptor, string, error) {
	d := goqu.Dialect(common.Dialect)
	aas := goqu.T(common.TblAASDescriptor).As("aas")

	ds := d.
		From(aas).
		Select(aas.Col(common.ColDescriptorID)).
		Where(aas.Col(common.ColAASID).Eq(aasID)).
		Limit(1)

	sqlStr, args, buildErr := ds.Prepared(true).ToSQL()
	if buildErr != nil {
		return nil, "", common.NewInternalServerError("AASREG-LISTSMDESC-BUILDPARENT " + buildErr.Error())
	}

	var descID int64
	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", common.NewErrNotFound("AAS Descriptor not found")
		}
		return nil, "", common.NewInternalServerError("AASREG-LISTSMDESC-QUERYPARENT " + err.Error())
	}
	return listSubmodelDescriptorsForAASSingleStatement(ctx, db, descID, limit, cursor)
}

// InsertSubmodelDescriptorForAAS inserts a single SubmodelDescriptor under the
// AAS identified by aasID (the AAS Id string).
//
// The function first resolves the internal AAS descriptor id. If the AAS does
// not exist, a NotFound error is returned. The insert runs inside a database
// transaction and uses the same creation helpers as other write paths. On any
// failure, the transaction is rolled back.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - aasID: AAS Id string owning the submodel
//   - submodel: descriptor to insert
func InsertSubmodelDescriptorForAAS(
	ctx context.Context,
	db *sql.DB,
	aasID string,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.SubmodelDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()
	result, err := insertSubmodelDescriptorForAASTx(ctx, tx, aasID, submodel)
	if err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}
	return result, tx.Commit()
}

// InsertSubmodelDescriptorForAASTx inserts a submodel descriptor for an AAS
// using the provided transaction.
func InsertSubmodelDescriptorForAASTx(
	ctx context.Context,
	tx *sql.Tx,
	aasID string,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	return insertSubmodelDescriptorForAASTx(ctx, tx, aasID, submodel)
}

func insertSubmodelDescriptorForAASTx(
	ctx context.Context,
	tx *sql.Tx,
	aasID string,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	// Lookup AAS descriptor id by AAS Id string
	d := goqu.Dialect(common.Dialect)
	aas := goqu.T(common.TblAASDescriptor).As("aas")

	ds := d.
		From(aas).
		Select(aas.Col(common.ColDescriptorID)).
		Where(aas.Col(common.ColAASID).Eq(aasID)).
		Limit(1)

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return model.SubmodelDescriptor{}, common.NewInternalServerError("Failed to build AAS lookup query. See server logs for details.")
	}

	var aasDescID int64
	if err := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&aasDescID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.SubmodelDescriptor{}, common.NewErrNotFound("AAS Descriptor not found")
		}
		return model.SubmodelDescriptor{}, common.NewInternalServerError("Failed to query AAS descriptor id. See server logs for details.")
	}

	err := createSubModelDescriptors(tx, sql.NullInt64{Int64: aasDescID, Valid: true}, []model.SubmodelDescriptor{submodel})

	if err != nil {
		return model.SubmodelDescriptor{}, err
	}

	return getSubmodelDescriptorForAASByIDOrDenied(ctx, tx, aasID, submodel.Id)
}

// ReplaceSubmodelDescriptorForAAS atomically replaces the submodel descriptor
// with the same Id under the given AAS. If a descriptor exists, the base
// descriptor row is deleted (cascade removes related rows), then the provided
// descriptor is inserted. The operation occurs within a single transaction.
//
// Returns a boolean indicating whether a descriptor existed before the replace.
// If the AAS does not exist, a NotFound error is returned.
func ReplaceSubmodelDescriptorForAAS(
	ctx context.Context,
	db *sql.DB,
	aasID string,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.SubmodelDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}

	if _, err = GetSubmodelDescriptorForAASByID(ctx, tx, aasID, submodel.Id); err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}

	err = deleteSubmodelDescriptorForAASByIDTx(ctx, tx, aasID, submodel.Id)

	if err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}
	result, err := InsertSubmodelDescriptorForAASTx(ctx, tx, aasID, submodel)
	if err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}

	return result, tx.Commit()
}

// GetSubmodelDescriptorForAASByID returns a single SubmodelDescriptor for a
// given AAS (by AAS Id string) and Submodel Id. The function resolves the
// internal AAS descriptor id, loads all submodels via
// ReadSubmodelDescriptorsByAASDescriptorIDs, and selects the one matching the
// provided submodelID. If either the AAS or the submodel under that AAS does
// not exist, NotFound is returned.
func GetSubmodelDescriptorForAASByID(
	ctx context.Context,
	db DBQueryer,
	aasID string,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	smdescs, _, err := ListSubmodelDescriptorsForAAS(ctx, db, aasID, 0, "")
	if err != nil {
		return model.SubmodelDescriptor{}, err
	}
	// TODO: do that in sql not in memory
	for _, smd := range smdescs {
		if smd.Id == submodelID {
			return smd, nil
		}
	}
	return model.SubmodelDescriptor{}, common.NewErrNotFound("Submodel Descriptor not found")
}

// getSubmodelDescriptorForAASByIDSecurity return a 403 instead of 404 for security reasons
func getSubmodelDescriptorForAASByIDOrDenied(
	ctx context.Context,
	db DBQueryer,
	aasID string,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	smdescs, _, err := ListSubmodelDescriptorsForAAS(ctx, db, aasID, 0, "")
	if err != nil {
		return model.SubmodelDescriptor{}, err
	}
	// TODO: do that in sql not in memory
	for _, smd := range smdescs {
		if smd.Id == submodelID {
			return smd, nil
		}
	}
	return model.SubmodelDescriptor{}, common.NewErrDenied("Submodel Descriptor access not allowed")
}

// DeleteSubmodelDescriptorForAASByID deletes the submodel descriptor under the
// given AAS. The function locates the base descriptor id by joining the AAS and
// submodel tables and then deletes the row from the base descriptor table. ON
// DELETE CASCADE in the schema cleans up related rows. The delete runs in a
// transaction.
func DeleteSubmodelDescriptorForAASByID(
	ctx context.Context,
	db *sql.DB,
	aasID string,
	submodelID string,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = GetSubmodelDescriptorForAASByID(ctx, db, aasID, submodelID)
	if err != nil {
		return err
	}
	err = deleteSubmodelDescriptorForAASByIDTx(ctx, tx, aasID, submodelID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// DeleteSubmodelDescriptorForAASByIDTx deletes a submodel descriptor for an AAS
// using the provided transaction.
func DeleteSubmodelDescriptorForAASByIDTx(
	ctx context.Context,
	tx *sql.Tx,
	aasID string,
	submodelID string,
) error {
	return deleteSubmodelDescriptorForAASByIDTx(ctx, tx, aasID, submodelID)
}

// deleteSubmodelDescriptorForAASByIDTx deletes the submodel descriptor under the
// given AAS within an existing transaction. The function locates the base
// descriptor id by joining the AAS and submodel tables and then deletes the row
// from the base descriptor table. ON DELETE CASCADE in the schema cleans up
// related rows.
func deleteSubmodelDescriptorForAASByIDTx(
	ctx context.Context,
	tx *sql.Tx,
	aasID string,
	submodelID string,
) error {
	d := goqu.Dialect(common.Dialect)
	aas := goqu.T(common.TblAASDescriptor).As("aas")
	smd := goqu.T(common.TblSubmodelDescriptor).As("smd")

	ds := d.
		From(smd).
		InnerJoin(aas, goqu.On(smd.Col(common.ColAASDescriptorID).Eq(aas.Col(common.ColDescriptorID)))).
		Select(smd.Col(common.ColDescriptorID)).
		Where(
			goqu.And(
				aas.Col(common.ColAASID).Eq(aasID),
				smd.Col(common.ColAASID).Eq(submodelID),
			),
		).
		Limit(1)

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return common.NewInternalServerError("Failed to build submodel lookup query. See server logs for details.")
	}
	var descID int64
	if err := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrNotFound("Submodel Descriptor not found")
		}
		return common.NewInternalServerError("Failed to query submodel descriptor id. See server logs for details.")
	}

	delSQL, delArgs, delErr := d.Delete(common.TblDescriptor).Where(goqu.C(common.ColID).Eq(descID)).ToSQL()
	if delErr != nil {
		return delErr
	}
	_, err := tx.Exec(delSQL, delArgs...)
	return err
}

// ExistsSubmodelForAAS performs a lightweight existence check for a submodel
// under a given AAS using an inner join and LIMIT 1. Returns true when present,
// false when absent.
func ExistsSubmodelForAAS(ctx context.Context, db *sql.DB, aasID, submodelID string) (bool, error) {
	d := goqu.Dialect(common.Dialect)
	smd := goqu.T(common.TblSubmodelDescriptor).As("smd")
	aas := goqu.T(common.TblAASDescriptor).As("aas")

	ds := d.
		From(smd).
		InnerJoin(aas, goqu.On(smd.Col(common.ColAASDescriptorID).Eq(aas.Col(common.ColDescriptorID)))).
		Select(goqu.L("1")).
		Where(
			goqu.And(
				aas.Col(common.ColAASID).Eq(aasID),
				smd.Col(common.ColAASID).Eq(submodelID),
			),
		).
		Limit(1)

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

// ListSubmodelDescriptors lists SubmodelDescriptors that are not associated
// with any AAS (aas_descriptor_id IS NULL). Results are ordered by Submodel Id
// ascending and support cursor-based pagination.
func ListSubmodelDescriptors(
	ctx context.Context,
	db DBQueryer,
	limit int32,
	cursor string,
	createdFrom time.Time,
	updatedFrom time.Time,
) ([]model.SubmodelDescriptor, string, error) {
	return listSubmodelDescriptorsSingleStatement(ctx, db, limit, cursor, createdFrom, updatedFrom)
}

// InsertSubmodelDescriptor inserts a single SubmodelDescriptor that is not
// associated with an AAS (aas_descriptor_id IS NULL).
func InsertSubmodelDescriptor(
	ctx context.Context,
	db *sql.DB,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.SubmodelDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()
	result, err := insertSubmodelDescriptorTx(ctx, tx, submodel)
	if err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}
	return result, tx.Commit()
}

// InsertSubmodelDescriptorTx inserts a global submodel descriptor using the
// provided transaction.
func InsertSubmodelDescriptorTx(
	ctx context.Context,
	tx *sql.Tx,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	return insertSubmodelDescriptorTx(ctx, tx, submodel)
}

// ReplaceSubmodelDescriptor atomically replaces a SubmodelDescriptor (global,
// non-AAS) by deleting the existing descriptor and inserting the new one.
func ReplaceSubmodelDescriptor(
	ctx context.Context,
	db *sql.DB,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.SubmodelDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = GetSubmodelDescriptorByID(ctx, tx, submodel.Id); err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}

	if err = deleteSubmodelDescriptorByIDTx(ctx, tx, submodel.Id); err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}
	result, err := InsertSubmodelDescriptorTx(ctx, tx, submodel)
	if err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}

	return result, tx.Commit()
}

// GetSubmodelDescriptorByID returns a single SubmodelDescriptor that is not
// associated with any AAS (aas_descriptor_id IS NULL).
func GetSubmodelDescriptorByID(
	ctx context.Context,
	db DBQueryer,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	descID, err := lookupSubmodelDescriptorID(ctx, db, submodelID)
	if err != nil {
		return model.SubmodelDescriptor{}, err
	}

	smdescs, err := ReadSubmodelDescriptorsByDescriptorIDs(ctx, db, []int64{descID})
	if err != nil {
		return model.SubmodelDescriptor{}, err
	}

	list := smdescs[descID]
	if len(list) == 0 {
		return model.SubmodelDescriptor{}, common.NewErrNotFound("Submodel Descriptor not found")
	}
	return list[0], nil
}

// getSubmodelDescriptorByIDOrDenied returns 403 when the descriptor exists but
// is not accessible under the current policy.
func getSubmodelDescriptorByIDOrDenied(
	ctx context.Context,
	db DBQueryer,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	smd, err := GetSubmodelDescriptorByID(ctx, db, submodelID)
	if err != nil {
		if common.IsErrNotFound(err) {
			return model.SubmodelDescriptor{}, common.NewErrDenied("Submodel Descriptor access not allowed")
		}
		return model.SubmodelDescriptor{}, err
	}
	return smd, nil
}

// DeleteSubmodelDescriptorByID deletes the submodel descriptor that is not
// associated with any AAS (aas_descriptor_id IS NULL).
func DeleteSubmodelDescriptorByID(
	ctx context.Context,
	db *sql.DB,
	submodelID string,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = GetSubmodelDescriptorByID(ctx, db, submodelID)
	if err != nil {
		return err
	}
	if err = deleteSubmodelDescriptorByIDTx(ctx, tx, submodelID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// DeleteSubmodelDescriptorByIDTx deletes a global submodel descriptor by id
// using the provided transaction.
func DeleteSubmodelDescriptorByIDTx(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
) error {
	return deleteSubmodelDescriptorByIDTx(ctx, tx, submodelID)
}

// DeleteSubmodelDescriptorsByIDsTx deletes global submodel descriptors by id.
//
// The function deletes descriptor rows in bounded chunks. Callers remain
// responsible for item-level existence checks when they need item-level errors.
//
// Parameters:
//   - ctx: Request context carrying configuration data.
//   - tx: Transaction used for deletion.
//   - submodelIDs: Global submodel descriptor identifiers to delete.
//
// Returns:
//   - error: Error when SQL rendering or deletion fails.
func DeleteSubmodelDescriptorsByIDsTx(
	ctx context.Context,
	tx *sql.Tx,
	submodelIDs []string,
) error {
	if len(submodelIDs) == 0 {
		return nil
	}
	d := goqu.Dialect(common.Dialect)
	batch := &common.PostgreSQLBatch{}
	limit := common.BulkBatchLimitFromContext(ctx)
	for start := 0; start < len(submodelIDs); start += limit {
		end := min(start+limit, len(submodelIDs))
		descriptorIDs := d.
			From(common.TblSubmodelDescriptor).
			Select(common.ColDescriptorID).
			Where(
				goqu.And(
					goqu.C(common.ColAASID).In(submodelIDs[start:end]),
					goqu.C(common.ColAASDescriptorID).IsNull(),
				),
			)
		if err := batch.AppendDataset(
			d.Delete(common.TblDescriptor).Where(goqu.C(common.ColID).In(descriptorIDs)),
		); err != nil {
			return common.NewInternalServerError("SMDESC-BULKDELETE-BUILDSQL " + err.Error())
		}
	}
	return common.ExecutePostgreSQLBatchInTransaction(ctx, tx, batch.Statements())
}

// ExistsSubmodelByID performs a lightweight existence check for a submodel
// descriptor without an AAS association.
func ExistsSubmodelByID(ctx context.Context, db *sql.DB, submodelID string) (bool, error) {
	return existsSubmodelByID(ctx, db, submodelID)
}

func existsSubmodelByID(ctx context.Context, db DBQueryer, submodelID string) (bool, error) {
	d := goqu.Dialect(common.Dialect)
	smd := goqu.T(common.TblSubmodelDescriptor).As("smd")

	ds := d.
		From(smd).
		Select(goqu.L("1")).
		Where(
			goqu.And(
				smd.Col(common.ColAASID).Eq(submodelID),
				smd.Col(common.ColAASDescriptorID).IsNull(),
			),
		).
		Limit(1)
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

func lookupSubmodelDescriptorID(ctx context.Context, db DBQueryer, submodelID string) (int64, error) {
	d := goqu.Dialect(common.Dialect)
	smd := goqu.T(common.TblSubmodelDescriptor).As("smd")

	ds := d.
		From(smd).
		Select(smd.Col(common.ColDescriptorID)).
		Where(
			goqu.And(
				smd.Col(common.ColAASID).Eq(submodelID),
				smd.Col(common.ColAASDescriptorID).IsNull(),
			),
		).
		Limit(1)
	sqlStr, args, buildErr := ds.Prepared(true).ToSQL()
	if buildErr != nil {
		return 0, common.NewInternalServerError("Failed to build submodel lookup query. See server logs for details.")
	}

	var descID int64
	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewErrNotFound("Submodel Descriptor not found")
		}
		return 0, common.NewInternalServerError("Failed to query submodel descriptor id. See server logs for details.")
	}
	return descID, nil
}

func insertSubmodelDescriptorTx(
	ctx context.Context,
	tx *sql.Tx,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	err := createSubModelDescriptors(tx, sql.NullInt64{}, []model.SubmodelDescriptor{submodel})
	if err != nil {
		return model.SubmodelDescriptor{}, err
	}

	return getSubmodelDescriptorByIDOrDenied(ctx, tx, submodel.Id)
}

func deleteSubmodelDescriptorByIDTx(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
) error {
	d := goqu.Dialect(common.Dialect)
	smd := goqu.T(common.TblSubmodelDescriptor).As("smd")

	ds := d.
		From(smd).
		Select(smd.Col(common.ColDescriptorID)).
		Where(
			goqu.And(
				smd.Col(common.ColAASID).Eq(submodelID),
				smd.Col(common.ColAASDescriptorID).IsNull(),
			),
		).
		Limit(1)

	delSQL, delArgs, buildErr := d.Delete(common.TblDescriptor).
		Where(goqu.C(common.ColID).In(ds)).
		ToSQL()
	if buildErr != nil {
		return common.NewInternalServerError("SMDESC-DELETE-BUILDSQL " + buildErr.Error())
	}
	result, err := tx.ExecContext(ctx, delSQL, delArgs...)
	if err != nil {
		return common.NewInternalServerError("SMDESC-DELETE-EXECSQL " + err.Error())
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return common.NewInternalServerError("SMDESC-DELETE-ROWSAFFECTED " + err.Error())
	}
	if deleted == 0 {
		return common.NewErrNotFound("Submodel Descriptor not found")
	}
	return nil
}
