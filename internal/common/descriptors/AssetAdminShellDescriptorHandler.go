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
	"fmt"
	"time"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"golang.org/x/sync/errgroup"
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
	result, err := GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id)
	if err != nil {
		_ = tx.Rollback()
		return model.AssetAdministrationShellDescriptor{}, err
	}
	return result, tx.Commit()
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
	d := goqu.Dialect(dialect)

	descTbl := goqu.T(tblDescriptor)

	sqlStr, args, buildErr := d.
		Insert(tblDescriptor).
		Returning(descTbl.Col(colID)).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	var descriptorID int64
	if err := tx.QueryRow(sqlStr, args...).Scan(&descriptorID); err != nil {
		return err
	}

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

	sqlStr, args, buildErr = d.
		Insert(tblDescriptorPayload).
		Rows(goqu.Record{
			colDescriptorID:              descriptorID,
			colDescriptionPayload:        goqu.L("?::jsonb", string(descriptionPayload)),
			colDisplayNamePayload:        goqu.L("?::jsonb", string(displayNamePayload)),
			colAdministrativeInfoPayload: goqu.L("?::jsonb", string(administrationPayload)),
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	sqlStr, args, buildErr = d.
		Insert(tblAASDescriptor).
		Rows(goqu.Record{
			colDescriptorID:  descriptorID,
			colAssetKind:     aasd.AssetKind,
			colAssetType:     aasd.AssetType,
			colGlobalAssetID: aasd.GlobalAssetId,
			colIDShort:       aasd.IdShort,
			colAASID:         aasd.Id,
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
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

	if err = createSpecificAssetID(tx, descriptorID, aasRef, aasd.SpecificAssetIds); err != nil {
		return err
	}

	if err = createExtensions(tx, descriptorID, aasd.Extensions); err != nil {
		return err
	}

	return createSubModelDescriptors(tx, sql.NullInt64{Int64: descriptorID, Valid: true}, aasd.SubmodelDescriptors)
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
	result, _, err := listAssetAdministrationShellDescriptors(ctx, db, 1, "", "", "", aasIdentifier, true)
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
	result, _, err := listAssetAdministrationShellDescriptors(ctx, tx, 1, "", "", "", aasIdentifier, false)
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

// DeleteAssetAdministrationShellDescriptorByIDTx deletes using the provided
// transaction. It resolves the internal descriptor id and removes the base
// descriptor row. Dependent rows are removed via ON DELETE CASCADE.
func deleteAssetAdministrationShellDescriptorByIDTx(ctx context.Context, tx *sql.Tx, aasIdentifier string) error {
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	sqlStr, args, buildErr := d.
		From(aas).
		Select(aas.Col(colDescriptorID)).
		Where(aas.Col(colAASID).Eq(aasIdentifier)).
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

	delStr, delArgs, buildDelErr := d.
		Delete(tblDescriptor).
		Where(goqu.C(colID).Eq(descID)).
		ToSQL()
	if buildDelErr != nil {
		return buildDelErr
	}
	if _, execErr := tx.Exec(delStr, delArgs...); execErr != nil {
		return execErr
	}
	return nil
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
	_, err = GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	// delete existing descriptor
	if err = deleteAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasd.Id); err != nil {
		_ = tx.Rollback()
		return model.AssetAdministrationShellDescriptor{}, err
	}
	// insert new descriptor
	if err = InsertAdministrationShellDescriptorTx(ctx, tx, aasd); err != nil {
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

func buildListAssetAdministrationShellDescriptorsQuery(
	ctx context.Context,
	peekLimit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	identifiable string,
) (*goqu.SelectDataset, error) {
	d := goqu.Dialect(dialect)
	var mapper = []auth.ExpressionIdentifiableMapper{
		{
			Exp: tAASDescriptor.Col(colDescriptorID),
		},
		{
			Exp:      tAASDescriptor.Col(colAssetKind),
			Fragment: fragPtr("$aasdesc#assetKind"),
		},
		{
			Exp:      tAASDescriptor.Col(colAssetType),
			Fragment: fragPtr("$aasdesc#assetType"),
		},
		{
			Exp:      tAASDescriptor.Col(colGlobalAssetID),
			Fragment: fragPtr("$aasdesc#globalAssetId"),
		},
		{
			Exp:      tAASDescriptor.Col(colIDShort),
			Fragment: fragPtr("$aasdesc#idShort"),
		},
		{
			Exp: tAASDescriptor.Col(colAASID),
		},
		{
			Exp:      tDescriptorPayload.Col(colAdministrativeInfoPayload),
			Fragment: fragPtr("$aasdesc#administration"),
		},
		{
			Exp:      tDescriptorPayload.Col(colDisplayNamePayload),
			Fragment: fragPtr("$aasdesc#displayName"),
		},
		{
			Exp:      tDescriptorPayload.Col(colDescriptionPayload),
			Fragment: fragPtr("$aasdesc#description"),
		},
	}

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		return nil, err
	}
	expressions, err := auth.GetColumnSelectStatement(ctx, mapper, collector)
	if err != nil {
		return nil, err
	}

	ds := d.From(tDescriptor).
		InnerJoin(
			tAASDescriptor,
			goqu.On(tAASDescriptor.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		).
		LeftJoin(
			tDescriptorPayload,
			goqu.On(tDescriptorPayload.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		).
		Select(
			expressions[0],
			expressions[1],
			expressions[2],
			expressions[3],
			expressions[4],
			expressions[5],
			expressions[6],
			expressions[7],
			expressions[8],
		)
	if auth.NeedsGroupBy(ctx, mapper) {
		ds = ds.GroupBy(
			expressions[0], // descriptor_id
		)
	}
	ds, err = auth.AddFormulaQueryFromContext(ctx, ds, collector)
	if err != nil {
		return nil, err
	}

	if cursor != "" {
		ds = ds.Where(tAASDescriptor.Col(colAASID).Gte(cursor))
	}

	if assetType != "" {
		ds = ds.Where(tAASDescriptor.Col(colAssetType).Eq(assetType))
	}

	if assetKind != "" {
		assetKindAsString := model.GetAssetKindString(assetKind)
		convertedAssetKind, ok := stringification.AssetKindFromString(assetKindAsString)
		if !ok {
			return nil, errors.New("Invalid asset kind: " + assetKindAsString)
		}
		ds = ds.Where(tAASDescriptor.Col(colAssetKind).Eq(convertedAssetKind))
	}

	if identifiable != "" {
		ds = ds.Where(tAASDescriptor.Col(colID).Eq(identifiable))
	}

	if peekLimit < 0 {
		return nil, common.NewErrBadRequest("Limit has to be higher than 0")
	}
	ds = ds.
		Order(tAASDescriptor.Col(colAASID).Asc()).
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
// limit <= 0, a conservative large default is applied.
//
//nolint:revive // Its only 31 instead of 30 - 1 is okay
func ListAssetAdministrationShellDescriptors(
	ctx context.Context,
	db *sql.DB,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	identifiable string,
) ([]model.AssetAdministrationShellDescriptor, string, error) {
	if debugEnabled(ctx) {
		defer func(start time.Time) {
			_, _ = fmt.Printf("ListAssetAdministrationShellDescriptors took %s\n", time.Since(start))
		}(time.Now())
	}
	return listAssetAdministrationShellDescriptors(ctx, db, limit, cursor, assetKind, assetType, identifiable, true)
}

//nolint:revive // has to be refactored later. i have no time
func listAssetAdministrationShellDescriptors(
	ctx context.Context,
	db DBQueryer,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	identifiable string,
	allowParallel bool,
) ([]model.AssetAdministrationShellDescriptor, string, error) {
	if limit <= 0 {
		limit = 1000000
	}
	peekLimit := limit + 1
	ds, err := buildListAssetAdministrationShellDescriptorsQuery(ctx, peekLimit, cursor, assetKind, assetType, identifiable)
	if err != nil {
		return nil, "", err
	}
	sqlStr, args, err := ds.ToSQL()
	if debugEnabled(ctx) {
		_, _ = fmt.Println(sqlStr)
	}

	if err != nil {
		return nil, "", err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = rows.Close()
	}()

	descRows := make([]model.AssetAdministrationShellDescriptorRow, 0, peekLimit)
	for rows.Next() {
		var r model.AssetAdministrationShellDescriptorRow
		if err := rows.Scan(
			&r.DescID,
			&r.AssetKind,
			&r.AssetType,
			&r.GlobalAssetID,
			&r.IDShort,
			&r.IDStr,
			&r.AdministrativeInfoPayload,
			&r.DisplayNamePayload,
			&r.DescriptionPayload,
		); err != nil {
			return nil, "", common.NewInternalServerError("Failed to scan AAS descriptor row. See server logs for details.")
		}
		descRows = append(descRows, r)
	}
	if rows.Err() != nil {
		return nil, "", common.NewInternalServerError("Failed to iterate AAS descriptors. See server logs for details.")
	}

	descRows, nextCursor := applyCursorLimit(descRows, limit, func(r model.AssetAdministrationShellDescriptorRow) string {
		return r.IDStr
	})

	if len(descRows) == 0 {
		return []model.AssetAdministrationShellDescriptor{}, nextCursor, nil
	}

	descIDs := make([]int64, 0, len(descRows))

	seenDesc := make(map[int64]struct{}, len(descRows))

	for _, r := range descRows {
		if _, ok := seenDesc[r.DescID]; !ok {
			seenDesc[r.DescID] = struct{}{}
			descIDs = append(descIDs, r.DescID)
		}
	}

	endpointsByDesc := map[int64][]model.Endpoint{}
	specificByDesc := map[int64][]types.ISpecificAssetID{}
	extByDesc := map[int64][]types.Extension{}
	smdByDesc := map[int64][]model.SubmodelDescriptor{}

	if allowParallel {
		g, gctx := errgroup.WithContext(ctx)

		if len(descIDs) > 0 {
			ids := append([]int64(nil), descIDs...)
			GoAssign(g, func() (map[int64][]model.Endpoint, error) {
				return ReadEndpointsByDescriptorIDs(gctx, db, ids, "aas")
			}, &endpointsByDesc)
			GoAssign(g, func() (map[int64][]types.ISpecificAssetID, error) {
				return ReadSpecificAssetIDsByDescriptorIDs(gctx, db, ids)
			}, &specificByDesc)
			GoAssign(g, func() (map[int64][]types.Extension, error) {
				return ReadExtensionsByDescriptorIDs(gctx, db, ids)
			}, &extByDesc)
			GoAssign(g, func() (map[int64][]model.SubmodelDescriptor, error) {
				return ReadSubmodelDescriptorsByAASDescriptorIDs(gctx, db, ids, false)
			}, &smdByDesc)
		}

		if err := g.Wait(); err != nil {
			return nil, "", err
		}
	} else {
		var err error
		if len(descIDs) > 0 {
			endpointsByDesc, err = ReadEndpointsByDescriptorIDs(ctx, db, descIDs, "aas")
			if err != nil {
				return nil, "", err
			}
			specificByDesc, err = ReadSpecificAssetIDsByDescriptorIDs(ctx, db, descIDs)
			if err != nil {
				return nil, "", err
			}
			extByDesc, err = ReadExtensionsByDescriptorIDs(ctx, db, descIDs)
			if err != nil {
				return nil, "", err
			}
			smdByDesc, err = ReadSubmodelDescriptorsByAASDescriptorIDs(ctx, db, descIDs, false)
			if err != nil {
				return nil, "", err
			}
		}
	}

	out := make([]model.AssetAdministrationShellDescriptor, 0, len(descRows))
	for _, r := range descRows {
		var ak *types.AssetKind
		if r.AssetKind.Valid {
			localAk := types.AssetKind(r.AssetKind.Int64)
			ak = &localAk
		}

		adminInfo, err := parseAdministrativeInfoPayload(r.AdministrativeInfoPayload)
		if err != nil {
			return nil, "", common.NewInternalServerError("AASDESC-LIST-ADMINPAYLOAD")
		}
		displayName, err := parseLangStringNamePayload(r.DisplayNamePayload)
		if err != nil {
			return nil, "", common.NewInternalServerError("AASDESC-LIST-DISPLAYNAMEPAYLOAD")
		}
		description, err := parseLangStringTextPayload(r.DescriptionPayload)
		if err != nil {
			return nil, "", common.NewInternalServerError("AASDESC-LIST-DESCRIPTIONPAYLOAD")
		}

		out = append(out, model.AssetAdministrationShellDescriptor{
			AssetKind:           ak,
			AssetType:           r.AssetType.String,
			GlobalAssetId:       r.GlobalAssetID.String,
			IdShort:             r.IDShort.String,
			Id:                  r.IDStr,
			Administration:      adminInfo,
			DisplayName:         displayName,
			Description:         description,
			Endpoints:           endpointsByDesc[r.DescID],
			SpecificAssetIds:    specificByDesc[r.DescID],
			Extensions:          extByDesc[r.DescID],
			SubmodelDescriptors: smdByDesc[r.DescID],
		})
	}

	return out, nextCursor, nil
}

// ExistsAASByID performs a lightweight existence check for an AAS by its Id
// string. It returns true when a descriptor exists, false when it does not.
func ExistsAASByID(ctx context.Context, db *sql.DB, aasID string) (bool, error) {
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.From(aas).Select(goqu.L("1")).Where(aas.Col(colAASID).Eq(aasID)).Limit(1)
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
