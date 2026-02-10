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
	"sort"

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
//   - The function resolves the internal AAS descriptor id, loads all submodel
//     descriptors via ReadSubmodelDescriptorsByAASDescriptorIDs (which performs
//     the necessary batched joins), and applies ordering/pagination in memory.
//   - This keeps the code compact and avoids duplicating SQL join logic. If the
//     number of submodels per AAS can be very large and DB-level pagination is
//     required, push ORDER/LIMIT/GTE into SQL over the submodel tables.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - aasID: AAS Id string owning the submodels
//   - limit: maximum number of items to return (<=0 uses a large default)
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
	if limit <= 0 {
		limit = 10000000
	}

	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.
		From(aas).
		Select(aas.Col(colDescriptorID)).
		Where(aas.Col(colAASID).Eq(aasID)).
		Limit(1)

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return nil, "", common.NewInternalServerError("Failed to build AAS lookup query. See server logs for details.")
	}

	var descID int64
	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []model.SubmodelDescriptor{}, "", nil
		}
		return nil, "", common.NewInternalServerError("Failed to query AAS descriptor id. See server logs for details.")
	}

	m, err := ReadSubmodelDescriptorsByAASDescriptorIDs(ctx, db, []int64{descID}, true)
	if err != nil {
		return nil, "", err
	}
	list := append([]model.SubmodelDescriptor{}, m[descID]...)

	sort.Slice(list, func(i, j int) bool {
		return list[i].Id < list[j].Id
	})

	if cursor != "" {
		lo, hi := 0, len(list)
		for lo < hi {
			mid := (lo + hi) / 2
			if list[mid].Id < cursor {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		list = list[lo:]
	}

	list, nextCursor := applyCursorLimit(list, limit, func(r model.SubmodelDescriptor) string {
		return r.Id
	})

	return list, nextCursor, nil
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

func insertSubmodelDescriptorForAASTx(
	ctx context.Context,
	tx *sql.Tx,
	aasID string,
	submodel model.SubmodelDescriptor,
) (model.SubmodelDescriptor, error) {
	// Lookup AAS descriptor id by AAS Id string
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.
		From(aas).
		Select(aas.Col(colDescriptorID)).
		Where(aas.Col(colAASID).Eq(aasID)).
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

	err = deleteSubmodelDescriptorForAASByIDTx(ctx, tx, aasID, submodel.Id)

	if err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}
	result, err := insertSubmodelDescriptorForAASTx(ctx, tx, aasID, submodel)
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
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")
	smd := goqu.T(tblSubmodelDescriptor).As("smd")

	ds := d.
		From(smd).
		InnerJoin(aas, goqu.On(smd.Col(colAASDescriptorID).Eq(aas.Col(colDescriptorID)))).
		Select(smd.Col(colDescriptorID)).
		Where(
			goqu.And(
				aas.Col(colAASID).Eq(aasID),
				smd.Col(colAASID).Eq(submodelID),
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

	delSQL, delArgs, delErr := d.Delete(tblDescriptor).Where(goqu.C(colID).Eq(descID)).ToSQL()
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
	d := goqu.Dialect(dialect)
	smd := goqu.T(tblSubmodelDescriptor).As("smd")
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.
		From(smd).
		InnerJoin(aas, goqu.On(smd.Col(colAASDescriptorID).Eq(aas.Col(colDescriptorID)))).
		Select(goqu.L("1")).
		Where(
			goqu.And(
				aas.Col(colAASID).Eq(aasID),
				smd.Col(colAASID).Eq(submodelID),
			),
		).
		Limit(1)

	sqlStr, args, err := ds.ToSQL()
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
) ([]model.SubmodelDescriptor, string, error) {
	if limit <= 0 {
		limit = 10000000
	}

	rows, nextCursor, err := listSubmodelDescriptorIDsWithoutAAS(ctx, db, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return []model.SubmodelDescriptor{}, nextCursor, nil
	}

	descIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		descIDs = append(descIDs, row.DescID)
	}

	byDesc, err := ReadSubmodelDescriptorsByDescriptorIDs(ctx, db, descIDs)
	if err != nil {
		return nil, "", err
	}

	list := make([]model.SubmodelDescriptor, 0, len(descIDs))
	for _, row := range rows {
		if smdRows, ok := byDesc[row.DescID]; ok {
			list = append(list, smdRows...)
		}
	}

	return list, nextCursor, nil
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

	if err = deleteSubmodelDescriptorByIDTx(ctx, tx, submodel.Id); err != nil {
		_ = tx.Rollback()
		return model.SubmodelDescriptor{}, err
	}
	result, err := insertSubmodelDescriptorTx(ctx, tx, submodel)
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

// ExistsSubmodelByID performs a lightweight existence check for a submodel
// descriptor without an AAS association.
func ExistsSubmodelByID(ctx context.Context, db *sql.DB, submodelID string) (bool, error) {
	d := goqu.Dialect(dialect)
	smd := goqu.T(tblSubmodelDescriptor).As("smd")

	ds := d.
		From(smd).
		Select(goqu.L("1")).
		Where(
			goqu.And(
				smd.Col(colAASID).Eq(submodelID),
				smd.Col(colAASDescriptorID).IsNull(),
			),
		).
		Limit(1)
	sqlStr, args, err := ds.ToSQL()
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

type submodelDescriptorPageRow struct {
	DescID     int64
	SubmodelID string
}

func listSubmodelDescriptorIDsWithoutAAS(
	ctx context.Context,
	db DBQueryer,
	limit int32,
	cursor string,
) ([]submodelDescriptorPageRow, string, error) {
	d := goqu.Dialect(dialect)
	smd := goqu.T(tblSubmodelDescriptor).As("smd")

	ds := d.
		From(smd).
		Select(
			smd.Col(colDescriptorID),
			smd.Col(colAASID),
		).
		Where(smd.Col(colAASDescriptorID).IsNull())

	if cursor != "" {
		ds = ds.Where(smd.Col(colAASID).Gte(cursor))
	}
	if limit <= 0 {
		return nil, "", common.NewErrBadRequest("Limit must be greater than 0")
	}
	peekLimit := int(limit) + 1

	if peekLimit <= 1 {
		return nil, "", common.NewErrBadRequest("Limit must be greater than 0")
	}
	ds = ds.Order(smd.Col(colAASID).Asc()).Limit(uint(peekLimit))

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return nil, "", common.NewInternalServerError("Failed to build submodel descriptor lookup query. See server logs for details.")
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("Failed to query submodel descriptor ids. See server logs for details.")
	}
	defer func() {
		_ = rows.Close()
	}()

	pageRows := make([]submodelDescriptorPageRow, 0, peekLimit)
	for rows.Next() {
		var row submodelDescriptorPageRow
		if scanErr := rows.Scan(&row.DescID, &row.SubmodelID); scanErr != nil {
			return nil, "", common.NewInternalServerError("Failed to scan submodel descriptor ids. See server logs for details.")
		}
		pageRows = append(pageRows, row)
	}
	if rows.Err() != nil {
		return nil, "", common.NewInternalServerError("Failed to iterate submodel descriptor ids. See server logs for details.")
	}

	pageRows, nextCursor := applyCursorLimit(pageRows, limit, func(r submodelDescriptorPageRow) string {
		return r.SubmodelID
	})

	return pageRows, nextCursor, nil
}

func lookupSubmodelDescriptorID(ctx context.Context, db DBQueryer, submodelID string) (int64, error) {
	d := goqu.Dialect(dialect)
	smd := goqu.T(tblSubmodelDescriptor).As("smd")

	ds := d.
		From(smd).
		Select(smd.Col(colDescriptorID)).
		Where(
			goqu.And(
				smd.Col(colAASID).Eq(submodelID),
				smd.Col(colAASDescriptorID).IsNull(),
			),
		).
		Limit(1)
	sqlStr, args, buildErr := ds.ToSQL()
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
	d := goqu.Dialect(dialect)
	smd := goqu.T(tblSubmodelDescriptor).As("smd")

	ds := d.
		From(smd).
		Select(smd.Col(colDescriptorID)).
		Where(
			goqu.And(
				smd.Col(colAASID).Eq(submodelID),
				smd.Col(colAASDescriptorID).IsNull(),
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

	delSQL, delArgs, delErr := d.Delete(tblDescriptor).Where(goqu.C(colID).Eq(descID)).ToSQL()
	if delErr != nil {
		return delErr
	}
	_, err := tx.Exec(delSQL, delArgs...)
	return err
}
