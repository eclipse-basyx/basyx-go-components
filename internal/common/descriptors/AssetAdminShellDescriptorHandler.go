/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	"golang.org/x/sync/errgroup"
)

// InsertAdministrationShellDescriptor creates a new AssetAdministrationShellDescriptor
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
func InsertAdministrationShellDescriptor(ctx context.Context, db *sql.DB, aasd model.AssetAdministrationShellDescriptor) error {
	return WithTx(ctx, db, func(tx *sql.Tx) error {
		return InsertAdministrationShellDescriptorTx(ctx, tx, aasd)
	})
}

// InsertAdministrationShellDescriptorTx performs the same insert as
// InsertAdministrationShellDescriptor but uses the provided transaction. This allows
// callers to compose multiple writes into a single atomic unit.
//
// The function inserts the base descriptor row first and then creates related
// entities (display name/description/admin info/endpoints/specific IDs/extensions
// and submodel descriptors). If any step fails, the error is returned and the
// caller is responsible for rolling back the transaction.
func InsertAdministrationShellDescriptorTx(_ context.Context, tx *sql.Tx, aasd model.AssetAdministrationShellDescriptor) error {
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

	var displayNameID, descriptionID, administrationID sql.NullInt64

	dnID, err := persistence_utils.CreateLangStringNameTypes(tx, aasd.DisplayName)
	if err != nil {
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}
	displayNameID = dnID
	var convertedDescription []model.LangStringText
	for _, desc := range aasd.Description {
		convertedDescription = append(convertedDescription, desc)
	}
	descID, err := persistence_utils.CreateLangStringTextTypes(tx, convertedDescription)
	if err != nil {
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}
	descriptionID = descID

	adminID, err := persistence_utils.CreateAdministrativeInformation(tx, aasd.Administration)
	if err != nil {
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}
	administrationID = adminID

	sqlStr, args, buildErr = d.
		Insert(tblAASDescriptor).
		Rows(goqu.Record{
			colDescriptorID:  descriptorID,
			colDescriptionID: descriptionID,
			colDisplayNameID: displayNameID,
			colAdminInfoID:   administrationID,
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

	if err = createEndpoints(tx, descriptorID, aasd.Endpoints); err != nil {
		return err
	}

	if err = createSpecificAssetID(tx, descriptorID, aasd.SpecificAssetIds); err != nil {
		return err
	}

	if err = createExtensions(tx, descriptorID, aasd.Extensions); err != nil {
		return err
	}

	if err = createSubModelDescriptors(tx, descriptorID, aasd.SubmodelDescriptors); err != nil {
		return err
	}

	return nil
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
	d := goqu.Dialect(dialect)

	aas := goqu.T(tblAASDescriptor).As("aas")

	sqlStr, args, buildErr := d.
		From(aas).
		Select(
			aas.Col(colDescriptorID),
			aas.Col(colAssetKind),
			aas.Col(colAssetType),
			aas.Col(colGlobalAssetID),
			aas.Col(colIDShort),
			aas.Col(colAASID),
			aas.Col(colAdminInfoID),
			aas.Col(colDisplayNameID),
			aas.Col(colDescriptionID),
		).
		Where(aas.Col(colAASID).Eq(aasIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return model.AssetAdministrationShellDescriptor{}, buildErr
	}

	var (
		descID                            int64
		assetKindStr                      sql.NullString
		assetType, globalAssetID, idShort sql.NullString
		idStr                             string
		adminInfoID                       sql.NullInt64
		displayNameID                     sql.NullInt64
		descriptionID                     sql.NullInt64
	)

	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(
		&descID,
		&assetKindStr,
		&assetType,
		&globalAssetID,
		&idShort,
		&idStr,
		&adminInfoID,
		&displayNameID,
		&descriptionID,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.AssetAdministrationShellDescriptor{}, common.NewErrNotFound("AAS Descriptor not found")
		}
		return model.AssetAdministrationShellDescriptor{}, err
	}

	var ak *model.AssetKind
	if assetKindStr.Valid && assetKindStr.String != "" {
		v, err := model.NewAssetKindFromValue(assetKindStr.String)
		if err != nil {
			return model.AssetAdministrationShellDescriptor{}, fmt.Errorf("invalid AssetKind %q", assetKindStr.String)
		}
		ak = &v
	}
	g, ctx := errgroup.WithContext(ctx)

	var (
		adminInfo        *model.AdministrativeInformation
		displayName      []model.LangStringNameType
		description      []model.LangStringTextType
		endpoints        []model.Endpoint
		specificAssetIDs []model.SpecificAssetID
		extensions       []model.Extension
		smds             []model.SubmodelDescriptor
	)

	g.Go(func() error {
		if adminInfoID.Valid {
			ai, err := ReadAdministrativeInformationByID(ctx, db, tblAASDescriptor, adminInfoID)
			if err != nil {
				return err
			}
			adminInfo = ai
		}
		return nil
	})
	GoAssign(g, func() ([]model.LangStringNameType, error) {
		return persistence_utils.GetLangStringNameTypes(db, displayNameID)
	}, &displayName)

	GoAssign(g, func() ([]model.LangStringTextType, error) {
		return persistence_utils.GetLangStringTextTypes(db, descriptionID)
	}, &description)

	GoAssign(g, func() ([]model.Endpoint, error) { return ReadEndpointsByDescriptorID(ctx, db, descID) }, &endpoints)

	GoAssign(g, func() ([]model.SpecificAssetID, error) { return ReadSpecificAssetIDsByDescriptorID(ctx, db, descID) }, &specificAssetIDs)

	GoAssign(g, func() ([]model.Extension, error) { return ReadExtensionsByDescriptorID(ctx, db, descID) }, &extensions)

	GoAssign(g, func() ([]model.SubmodelDescriptor, error) {
		return ReadSubmodelDescriptorsByAASDescriptorID(ctx, db, descID)
	}, &smds)

	if err := g.Wait(); err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}

	return model.AssetAdministrationShellDescriptor{
		AssetKind:           ak,
		AssetType:           assetType.String,
		GlobalAssetId:       globalAssetID.String,
		IdShort:             idShort.String,
		Id:                  idStr,
		Administration:      adminInfo,
		DisplayName:         displayName,
		Description:         description,
		Endpoints:           endpoints,
		SpecificAssetIds:    specificAssetIDs,
		Extensions:          extensions,
		SubmodelDescriptors: smds,
	}, nil
}

// DeleteAssetAdministrationShellDescriptorByID deletes the descriptor for the
// given AAS Id string. Deletion happens on the base descriptor row with ON
// DELETE CASCADE removing dependent rows.
//
// The delete runs in its own transaction.
func DeleteAssetAdministrationShellDescriptorByID(ctx context.Context, db *sql.DB, aasIdentifier string) error {
	return WithTx(ctx, db, func(tx *sql.Tx) error {
		return deleteAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasIdentifier)
	})
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
// input. The returned boolean indicates whether a descriptor existed before the
// replace.
func ReplaceAdministrationShellDescriptor(ctx context.Context, db *sql.DB, aasd model.AssetAdministrationShellDescriptor) (bool, error) {
	existed := false
	err := WithTx(ctx, db, func(tx *sql.Tx) error {
		d := goqu.Dialect(dialect)
		aas := goqu.T(tblAASDescriptor).As("aas")

		sqlStr, args, buildErr := d.
			From(aas).
			Select(aas.Col(colDescriptorID)).
			Where(aas.Col(colAASID).Eq(aasd.Id)).
			Limit(1).
			ToSQL()
		if buildErr != nil {
			return buildErr
		}
		var descID int64
		scanErr := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descID)
		if scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
			return scanErr
		}
		if scanErr == nil {
			existed = true
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
		}

		return InsertAdministrationShellDescriptorTx(ctx, tx, aasd)
	})
	return existed, err
}

// ListAssetAdministrationShellDescriptors lists AAS descriptors with optional
// filtering by AssetKind and AssetType. Results are ordered by AAS Id
// ascending and support cursor‑based pagination where the cursor is the AAS Id
// of the first element to include (i.e. Id >= cursor).
//
// It returns the page of fully assembled descriptors and, when more results are
// available, a next cursor value (the Id immediately after the page). When
// limit <= 0, a conservative large default is applied.
func ListAssetAdministrationShellDescriptors(
	ctx context.Context,
	db *sql.DB,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
) ([]model.AssetAdministrationShellDescriptor, string, error) {
	start := time.Now()
	defer func() {
		fmt.Printf("ListAssetAdministrationShellDescriptors took %s\n", time.Since(start))
	}()

	if limit <= 0 {
		limit = 1000000
	}
	peekLimit := int(limit) + 1

	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.
		From(aas).
		Select(
			aas.Col(colDescriptorID),
			aas.Col(colAssetKind),
			aas.Col(colAssetType),
			aas.Col(colGlobalAssetID),
			aas.Col(colIDShort),
			aas.Col(colAASID),
			aas.Col(colAdminInfoID),
			aas.Col(colDisplayNameID),
			aas.Col(colDescriptionID),
		)

	ds, err := getFilterQueryFromContext(ctx, d, ds, aas)
	if err != nil {
		return nil, "", err
	}

	if cursor != "" {
		ds = ds.Where(aas.Col(colAASID).Gte(cursor))
	}

	if assetType != "" {
		ds = ds.Where(aas.Col(colAssetType).Eq(assetType))
	}

	if assetKind != "" {
		ds = ds.Where(aas.Col(colAssetKind).Eq(assetKind))
	}

	ds = ds.
		Order(aas.Col(colAASID).Asc()).
		Limit(uint(peekLimit))

	sqlStr, args, err := ds.ToSQL()

	fmt.Println(sqlStr)
	fmt.Println(args)
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
		fmt.Println("a")
		var r model.AssetAdministrationShellDescriptorRow
		if err := rows.Scan(
			&r.DescID,
			&r.AssetKindStr,
			&r.AssetType,
			&r.GlobalAssetID,
			&r.IDShort,
			&r.IDStr,
			&r.AdminInfoID,
			&r.DisplayNameID,
			&r.DescriptionID,
		); err != nil {
			return nil, "", common.NewInternalServerError("Failed to scan AAS descriptor row. See server logs for details.")
		}
		descRows = append(descRows, r)
	}
	if rows.Err() != nil {
		return nil, "", common.NewInternalServerError("Failed to iterate AAS descriptors. See server logs for details.")
	}

	var nextCursor string
	if len(descRows) > int(limit) {
		nextCursor = descRows[limit].IDStr
		descRows = descRows[:limit]
	}

	if len(descRows) == 0 {
		return []model.AssetAdministrationShellDescriptor{}, nextCursor, nil
	}

	descIDs := make([]int64, 0, len(descRows))
	adminInfoIDs := make([]int64, 0, len(descRows))
	displayNameIDs := make([]int64, 0, len(descRows))
	descriptionIDs := make([]int64, 0, len(descRows))

	seenDesc := make(map[int64]struct{}, len(descRows))
	seenAI := map[int64]struct{}{}
	seenDN := map[int64]struct{}{}
	seenDE := map[int64]struct{}{}

	for _, r := range descRows {

		if _, ok := seenDesc[r.DescID]; !ok {
			seenDesc[r.DescID] = struct{}{}
			descIDs = append(descIDs, r.DescID)
		}

		if r.AdminInfoID.Valid {
			id := r.AdminInfoID.Int64
			if _, ok := seenAI[id]; !ok {
				seenAI[id] = struct{}{}
				adminInfoIDs = append(adminInfoIDs, id)
			}
		}
		if r.DisplayNameID.Valid {
			id := r.DisplayNameID.Int64
			if _, ok := seenDN[id]; !ok {
				seenDN[id] = struct{}{}
				displayNameIDs = append(displayNameIDs, id)
			}
		}

		if r.DescriptionID.Valid {
			id := r.DescriptionID.Int64
			if _, ok := seenDE[id]; !ok {
				seenDE[id] = struct{}{}
				descriptionIDs = append(descriptionIDs, id)
			}
		}
	}

	admByID := map[int64]*model.AdministrativeInformation{}
	dnByID := map[int64][]model.LangStringNameType{}
	descByID := map[int64][]model.LangStringTextType{}
	endpointsByDesc := map[int64][]model.Endpoint{}
	specificByDesc := map[int64][]model.SpecificAssetID{}
	extByDesc := map[int64][]model.Extension{}
	smdByDesc := map[int64][]model.SubmodelDescriptor{}

	g, gctx := errgroup.WithContext(ctx)

	if len(adminInfoIDs) > 0 {
		ids := append([]int64(nil), adminInfoIDs...)
		GoAssign(g, func() (map[int64]*model.AdministrativeInformation, error) {
			return ReadAdministrativeInformationByIDs(gctx, db, tblAASDescriptor, ids)
		}, &admByID)
	}
	if len(displayNameIDs) > 0 {
		ids := append([]int64(nil), displayNameIDs...)
		GoAssign(g, func() (map[int64][]model.LangStringNameType, error) {
			return GetLangStringNameTypesByIDs(db, ids)
		}, &dnByID)
	}

	if len(descriptionIDs) > 0 {
		ids := append([]int64(nil), descriptionIDs...)
		GoAssign(g, func() (map[int64][]model.LangStringTextType, error) {
			return GetLangStringTextTypesByIDs(db, ids)
		}, &descByID)
	}

	if len(descIDs) > 0 {
		ids := append([]int64(nil), descIDs...)
		GoAssign(g, func() (map[int64][]model.Endpoint, error) {
			return ReadEndpointsByDescriptorIDs(gctx, db, ids)
		}, &endpointsByDesc)
		GoAssign(g, func() (map[int64][]model.SpecificAssetID, error) {
			return ReadSpecificAssetIDsByDescriptorIDs(gctx, db, ids)
		}, &specificByDesc)
		GoAssign(g, func() (map[int64][]model.Extension, error) {
			return ReadExtensionsByDescriptorIDs(gctx, db, ids)
		}, &extByDesc)
		GoAssign(g, func() (map[int64][]model.SubmodelDescriptor, error) {
			return ReadSubmodelDescriptorsByAASDescriptorIDs(gctx, db, ids)
		}, &smdByDesc)
	}

	if err := g.Wait(); err != nil {
		return nil, "", err
	}

	out := make([]model.AssetAdministrationShellDescriptor, 0, len(descRows))
	for _, r := range descRows {
		var ak *model.AssetKind
		if r.AssetKindStr.Valid && r.AssetKindStr.String != "" {
			v, convErr := model.NewAssetKindFromValue(r.AssetKindStr.String)
			if convErr != nil {
				return nil, "", fmt.Errorf("invalid AssetKind %q for AAS %s", r.AssetKindStr.String, r.IDStr)
			}
			ak = &v
		}

		var adminInfo *model.AdministrativeInformation
		if r.AdminInfoID.Valid {
			if v, ok := admByID[r.AdminInfoID.Int64]; ok {
				tmp := v
				adminInfo = tmp
			}
		}

		var displayName []model.LangStringNameType
		if r.DisplayNameID.Valid {
			displayName = dnByID[r.DisplayNameID.Int64]
		}

		var description []model.LangStringTextType
		if r.DescriptionID.Valid {
			description = descByID[r.DescriptionID.Int64]
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
