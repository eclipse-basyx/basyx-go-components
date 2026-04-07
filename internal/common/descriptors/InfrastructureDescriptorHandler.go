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

package descriptors

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// InsertInfrastructureDescriptor creates a new InfrastructureDescriptor
// and all its related entities (display name, description,
// administration, and endpoints).
//
// The operation runs in its own database transaction. If any part of the write
// fails, the transaction is rolled back and no partial data is left behind.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - infrastructureDescriptor: descriptor to persist
//
// Returns an error when SQL building/execution fails or when writing any of the
// dependent rows fails. Errors are wrapped into common errors where relevant.
func InsertInfrastructureDescriptor(ctx context.Context, db *sql.DB, infrastructureDescriptor model.InfrastructureDescriptor) (model.InfrastructureDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.InfrastructureDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()
	if err = InsertInfrastructureDescriptorTx(ctx, tx, infrastructureDescriptor); err != nil {
		_ = tx.Rollback()
		return model.InfrastructureDescriptor{}, err
	}
	result, err := GetInfrastructureDescriptorByIDTx(ctx, tx, infrastructureDescriptor.Domain)
	if err != nil {
		_ = tx.Rollback()
		return model.InfrastructureDescriptor{}, err
	}
	return result, tx.Commit()
}

// InsertInfrastructureDescriptorTx performs the same insert as
// InsertInfrastructureDescriptor but uses the provided transaction. This allows
// callers to compose multiple writes into a single atomic unit.
//
// The function inserts the base descriptor row first and then creates related
// entities (display name/description/admin info/endpoints). If any step fails,
// the error is returned and the caller is responsible for rolling back the transaction.
func InsertInfrastructureDescriptorTx(_ context.Context, tx *sql.Tx, infdesc model.InfrastructureDescriptor) error {
	if err := model.AssertInfrastructureDescriptorConstraints(infdesc); err != nil {
		return common.NewErrBadRequest(err.Error())
	}

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

	descriptionPayload, err := buildLangStringTextPayload(infdesc.Description)
	if err != nil {
		return common.NewInternalServerError("INFDESC-INSERT-DESCRIPTIONPAYLOAD")
	}
	displayNamePayload, err := buildLangStringNamePayload(infdesc.DisplayName)
	if err != nil {
		return common.NewInternalServerError("INFDESC-INSERT-DISPLAYNAMEPAYLOAD")
	}
	administrationPayload, err := buildAdministrativeInfoPayload(infdesc.Administration)
	if err != nil {
		return common.NewInternalServerError("INFDESC-INSERT-ADMINPAYLOAD")
	}

	sqlStr, args, buildErr = d.
		Insert(common.TblDescriptorPayload).
		Rows(goqu.Record{
			common.ColDescriptorID:              descriptorID,
			common.ColDescriptionPayload:        goqu.L("?::jsonb", string(descriptionPayload)),
			common.ColDisplayNamePayload:        goqu.L("?::jsonb", string(displayNamePayload)),
			common.ColAdministrativeInfoPayload: goqu.L("?::jsonb", string(administrationPayload)),
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	sqlStr, args, buildErr = d.
		Insert(common.TblInfrastructureDescriptor).
		Rows(goqu.Record{
			common.ColDescriptorID:  descriptorID,
			common.ColGlobalAssetID: infdesc.GlobalAssetId,
			common.ColIDShort:       infdesc.IdShort,
			common.ColCompanyName:   infdesc.Name,
			common.ColCompanyDomain: infdesc.Domain,
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	if err = CreateEndpoints(tx, descriptorID, infdesc.Endpoints); err != nil {
		return common.NewInternalServerError("Failed to create Endpoints - no changes applied - see console for details")
	}

	if err = createInfrastructureNameOptions(tx, descriptorID, infdesc.NameOptions); err != nil {
		return common.NewInternalServerError("Failed to create Infrastructure Name Options - no changes applied - see console for details")
	}

	if err = createInfrastructureAssetIDRegexPatterns(tx, descriptorID, infdesc.AssetIdRegexPatterns); err != nil {
		return common.NewInternalServerError("Failed to create Infrastructure AssetID Regex Patterns - no changes applied - see console for details")
	}

	if err = createInfrastructureIDLinkRegexPatterns(tx, descriptorID, infdesc.IdLinkRegexPatterns); err != nil {
		return common.NewInternalServerError("Failed to create Infrastructure IDLink Regex Patterns - no changes applied - see console for details")
	}

	return nil
}

// GetInfrastructureDescriptorByID returns a fully materialized
// InfrastructureDescriptor by its Infrastructure Id string.
// The function loads optional related entities (administration, display name,
// description, and endpoints) concurrently to minimize latency. If the
// Infrastructure does not exist, a NotFound error is returned.
func GetInfrastructureDescriptorByID(ctx context.Context, db *sql.DB, infrastructureIdentifier string) (model.InfrastructureDescriptor, error) {
	d := goqu.Dialect(common.Dialect)

	inf := goqu.T(common.TblInfrastructureDescriptor).As("inf")
	payload := common.TDescriptorPayload.As("inf_payload")

	sqlStr, args, buildErr := d.
		From(inf).
		LeftJoin(
			payload,
			goqu.On(payload.Col(common.ColDescriptorID).Eq(inf.Col(common.ColDescriptorID))),
		).
		Select(
			inf.Col(common.ColDescriptorID),
			inf.Col(common.ColGlobalAssetID),
			inf.Col(common.ColIDShort),
			inf.Col(common.ColCompanyName),
			inf.Col(common.ColCompanyDomain),
			payload.Col(common.ColAdministrativeInfoPayload),
			payload.Col(common.ColDisplayNamePayload),
			payload.Col(common.ColDescriptionPayload),
		).
		Where(inf.Col(common.ColCompanyDomain).Eq(infrastructureIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return model.InfrastructureDescriptor{}, buildErr
	}

	var (
		descID                    int64
		globalAssetID, idShort    sql.NullString
		name, domain              sql.NullString
		administrativeInfoPayload []byte
		displayNamePayload        []byte
		descriptionPayload        []byte
	)

	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(
		&descID,
		&globalAssetID,
		&idShort,
		&name,
		&domain,
		&administrativeInfoPayload,
		&displayNamePayload,
		&descriptionPayload,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.InfrastructureDescriptor{}, common.NewErrNotFound("Infrastructure Descriptor not found")
		}
		return model.InfrastructureDescriptor{}, err
	}

	adminInfo, err := parseAdministrativeInfoPayload(administrativeInfoPayload)
	if err != nil {
		return model.InfrastructureDescriptor{}, common.NewInternalServerError("INFDESC-READ-ADMINPAYLOAD")
	}
	displayName, err := parseLangStringNamePayload(displayNamePayload)
	if err != nil {
		return model.InfrastructureDescriptor{}, common.NewInternalServerError("INFDESC-READ-DISPLAYNAMEPAYLOAD")
	}
	description, err := parseLangStringTextPayload(descriptionPayload)
	if err != nil {
		return model.InfrastructureDescriptor{}, common.NewInternalServerError("INFDESC-READ-DESCRIPTIONPAYLOAD")
	}
	endpoints, err := ReadEndpointsByDescriptorID(ctx, db, descID, "infrastructure")
	if err != nil {
		return model.InfrastructureDescriptor{}, err
	}
	nameOptions, err := readInfrastructureNameOptionsByDescriptorID(ctx, db, descID)
	if err != nil {
		return model.InfrastructureDescriptor{}, err
	}
	assetIDRegexPatterns, err := readInfrastructureAssetIDRegexPatternsByDescriptorID(ctx, db, descID)
	if err != nil {
		return model.InfrastructureDescriptor{}, err
	}
	idLinkRegexPatterns, err := readInfrastructureIDLinkRegexPatternsByDescriptorID(ctx, db, descID)
	if err != nil {
		return model.InfrastructureDescriptor{}, err
	}

	return model.InfrastructureDescriptor{
		GlobalAssetId:        globalAssetID.String,
		IdShort:              idShort.String,
		Name:                 name.String,
		Domain:               domain.String,
		NameOptions:          nameOptions,
		AssetIdRegexPatterns: assetIDRegexPatterns,
		IdLinkRegexPatterns:  idLinkRegexPatterns,
		Administration:       adminInfo,
		DisplayName:          displayName,
		Description:          description,
		Endpoints:            endpoints,
	}, nil
}

// GetInfrastructureDescriptorByIDTx returns a fully materialized
// InfrastructureDescriptor by its Infrastructure Id string using the provided
// transaction. It avoids concurrent queries, which are unsafe on *sql.Tx.
func GetInfrastructureDescriptorByIDTx(ctx context.Context, tx *sql.Tx, infrastructureIdentifier string) (model.InfrastructureDescriptor, error) {
	d := goqu.Dialect(common.Dialect)

	inf := goqu.T(common.TblInfrastructureDescriptor).As("inf")
	payload := common.TDescriptorPayload.As("inf_payload")

	sqlStr, args, buildErr := d.
		From(inf).
		LeftJoin(
			payload,
			goqu.On(payload.Col(common.ColDescriptorID).Eq(inf.Col(common.ColDescriptorID))),
		).
		Select(
			inf.Col(common.ColDescriptorID),
			inf.Col(common.ColGlobalAssetID),
			inf.Col(common.ColIDShort),
			inf.Col(common.ColCompanyName),
			inf.Col(common.ColCompanyDomain),
			payload.Col(common.ColAdministrativeInfoPayload),
			payload.Col(common.ColDisplayNamePayload),
			payload.Col(common.ColDescriptionPayload),
		).
		Where(inf.Col(common.ColCompanyDomain).Eq(infrastructureIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return model.InfrastructureDescriptor{}, buildErr
	}
	var (
		descID                    int64
		globalAssetID, idShort    sql.NullString
		name, domain              sql.NullString
		administrativeInfoPayload []byte
		displayNamePayload        []byte
		descriptionPayload        []byte
	)

	if err := tx.QueryRowContext(ctx, sqlStr, args...).Scan(
		&descID,
		&globalAssetID,
		&idShort,
		&name,
		&domain,
		&administrativeInfoPayload,
		&displayNamePayload,
		&descriptionPayload,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.InfrastructureDescriptor{}, common.NewErrNotFound("Infrastructure Descriptor not found")
		}
		return model.InfrastructureDescriptor{}, err
	}
	adminInfo, err := parseAdministrativeInfoPayload(administrativeInfoPayload)
	if err != nil {
		return model.InfrastructureDescriptor{}, common.NewInternalServerError("INFDESC-READ-ADMINPAYLOAD")
	}
	displayName, err := parseLangStringNamePayload(displayNamePayload)
	if err != nil {
		return model.InfrastructureDescriptor{}, common.NewInternalServerError("INFDESC-READ-DISPLAYNAMEPAYLOAD")
	}
	description, err := parseLangStringTextPayload(descriptionPayload)
	if err != nil {
		return model.InfrastructureDescriptor{}, common.NewInternalServerError("INFDESC-READ-DESCRIPTIONPAYLOAD")
	}
	endpoints, err := ReadEndpointsByDescriptorID(ctx, tx, descID, "infrastructure")
	if err != nil {
		return model.InfrastructureDescriptor{}, err
	}
	nameOptions, err := readInfrastructureNameOptionsByDescriptorID(ctx, tx, descID)
	if err != nil {
		return model.InfrastructureDescriptor{}, err
	}
	assetIDRegexPatterns, err := readInfrastructureAssetIDRegexPatternsByDescriptorID(ctx, tx, descID)
	if err != nil {
		return model.InfrastructureDescriptor{}, err
	}
	idLinkRegexPatterns, err := readInfrastructureIDLinkRegexPatternsByDescriptorID(ctx, tx, descID)
	if err != nil {
		return model.InfrastructureDescriptor{}, err
	}

	return model.InfrastructureDescriptor{
		GlobalAssetId:        globalAssetID.String,
		IdShort:              idShort.String,
		Name:                 name.String,
		Domain:               domain.String,
		NameOptions:          nameOptions,
		AssetIdRegexPatterns: assetIDRegexPatterns,
		IdLinkRegexPatterns:  idLinkRegexPatterns,
		Administration:       adminInfo,
		DisplayName:          displayName,
		Description:          description,
		Endpoints:            endpoints,
	}, nil
}

// DeleteInfrastructureDescriptorByID deletes the descriptor for the
// given Infrastructure Descriptor Id string. Deletion happens on the base descriptor row with ON
// DELETE CASCADE removing dependent rows.
// The delete runs in its own transaction.
func DeleteInfrastructureDescriptorByID(ctx context.Context, db *sql.DB, infrastructureIdentifier string) error {
	return WithTx(ctx, db, func(tx *sql.Tx) error {
		return DeleteInfrastructureDescriptorByIDTx(ctx, tx, infrastructureIdentifier)
	})
}

// DeleteInfrastructureDescriptorByIDTx deletes using the provided
// transaction. It resolves the internal descriptor id and removes the base
// descriptor row. Dependent rows are removed via ON DELETE CASCADE.
func DeleteInfrastructureDescriptorByIDTx(ctx context.Context, tx *sql.Tx, infrastructureIdentifier string) error {
	d := goqu.Dialect("postgres")
	inf := goqu.T(common.TblInfrastructureDescriptor).As("inf")

	sqlStr, args, buildErr := d.
		From(inf).
		Select(inf.Col(common.ColDescriptorID)).
		Where(inf.Col(common.ColCompanyDomain).Eq(infrastructureIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}

	var descID int64
	if scanErr := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return common.NewErrNotFound("Infrastructure Descriptor not found")
		}
		return scanErr
	}

	delStr, delArgs, buildDelErr := d.
		Delete(common.TblDescriptor).
		Where(goqu.C(common.ColID).Eq(descID)).
		ToSQL()
	if buildDelErr != nil {
		return buildDelErr
	}
	if _, execErr := tx.Exec(delStr, delArgs...); execErr != nil {
		return execErr
	}
	return nil
}

// ReplaceInfrastructureDescriptor atomically replaces the descriptor with the same
// Infrastructure Id: if a descriptor exists it is deleted (base descriptor row), then
// the provided descriptor is inserted. Related rows are recreated from the input.
// The returned descriptor is the stored Infrastructure Descriptor after replacement.
func ReplaceInfrastructureDescriptor(ctx context.Context, db *sql.DB, infrastructureDescriptor model.InfrastructureDescriptor) (model.InfrastructureDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.InfrastructureDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()

	// delete existing descriptor
	if err = DeleteInfrastructureDescriptorByIDTx(ctx, tx, infrastructureDescriptor.Domain); err != nil {
		_ = tx.Rollback()
		return model.InfrastructureDescriptor{}, err
	}
	// insert new descriptor
	if err = InsertInfrastructureDescriptorTx(ctx, tx, infrastructureDescriptor); err != nil {
		_ = tx.Rollback()
		return model.InfrastructureDescriptor{}, err
	}

	result, err := GetInfrastructureDescriptorByIDTx(ctx, tx, infrastructureDescriptor.Domain)
	if err != nil {
		_ = tx.Rollback()
		return model.InfrastructureDescriptor{}, err
	}
	return result, tx.Commit()
}

// ListInfrastructureDescriptors lists Infrastructure Descriptors with optional
// filtering by name/domain and endpoint interface. Results are ordered by Infrastructure Id
// ascending and support cursor‑based pagination where the cursor is the Infrastructure Id
// of the first element to include (i.e. Id >= cursor).
//
// It returns the page of fully assembled descriptors and, when more results are
// available, a next cursor value (the Id immediately after the page). When
// limit <= 0, a conservative large default is applied.
//
// nolint:revive // complexity is 31 which is +1 above the allowed threshold of 30
func ListInfrastructureDescriptors(
	ctx context.Context,
	db *sql.DB,
	limit int32,
	cursor string,
	name string,
	domain string,
	endpointInterface string,
	assetID string,
) ([]model.InfrastructureDescriptor, string, error) {
	if limit <= 0 {
		limit = 100
	}
	peekLimit := int(limit) + 1

	d := goqu.Dialect(common.Dialect)
	inf := goqu.T(common.TblInfrastructureDescriptor).As("inf")
	payload := common.TDescriptorPayload.As("inf_payload")
	aasdescendp := goqu.T(common.TblAASDescriptorEndpoint).As("aasdescendp")
	infNameOpt := goqu.T(common.TblInfrastructureDescriptorNameOption).As("inf_name_opt")
	assetIdPattern := goqu.T(common.TblInfrastructureDescriptorAssetIDRegex).As("inf_asset_id_pattern")
	idLinkPattern := goqu.T(common.TblInfrastructureDescriptorIDLinkRegex).As("inf_id_link_pattern")

	ds := d.
		From(inf).
		LeftJoin(
			payload,
			goqu.On(payload.Col(common.ColDescriptorID).Eq(inf.Col(common.ColDescriptorID))),
		).
		Select(
			inf.Col(common.ColDescriptorID),
			inf.Col(common.ColGlobalAssetID),
			inf.Col(common.ColIDShort),
			inf.Col(common.ColCompanyName),
			inf.Col(common.ColCompanyDomain),
			inf.Col(common.ColCompanyDomain),
			payload.Col(common.ColAdministrativeInfoPayload),
			payload.Col(common.ColDisplayNamePayload),
			payload.Col(common.ColDescriptionPayload),
		)

	if cursor != "" {
		ds = ds.Where(inf.Col(common.ColCompanyDomain).Gte(cursor))
	}

	if strings.TrimSpace(name) != "" {
		nameLower := strings.ToLower(name)
		ds = ds.
			LeftJoin(
				infNameOpt,
				goqu.On(
					inf.Col(common.ColDescriptorID).Eq(infNameOpt.Col(common.ColDescriptorID)),
					goqu.Func("LOWER", infNameOpt.Col(common.ColNameOption)).Eq(nameLower),
				),
			).
			Where(
				goqu.Or(
					goqu.Func("LOWER", inf.Col(common.ColCompanyName)).Eq(nameLower),
					infNameOpt.Col(common.ColDescriptorID).IsNotNull(),
				),
			)
	}

	if strings.TrimSpace(domain) != "" {
		ds = ds.Where(goqu.Func("LOWER", inf.Col(common.ColCompanyDomain)).Eq(strings.ToLower(domain)))
	}

	if endpointInterface != "" {
		ds = ds.
			LeftJoin(
				aasdescendp,
				goqu.On(
					inf.Col(common.ColDescriptorID).Eq(aasdescendp.Col(common.ColDescriptorID)),
				),
			).
			Where(aasdescendp.Col(common.ColInterface).Eq(endpointInterface))
	}

	if strings.TrimSpace(assetID) != "" {
		assetIdRegexExists := d.
			From(assetIdPattern).
			Select(goqu.V(true)).
			Where(
				assetIdPattern.Col(common.ColDescriptorID).Eq(inf.Col(common.ColDescriptorID)),
				goqu.L("? ~ ?", assetID, assetIdPattern.Col(common.ColRegexPattern)),
			)

		idLinkRegexExists := d.
			From(idLinkPattern).
			Select(goqu.V(true)).
			Where(
				idLinkPattern.Col(common.ColDescriptorID).Eq(inf.Col(common.ColDescriptorID)),
				goqu.L("? ~ ?", assetID, idLinkPattern.Col(common.ColRegexPattern)),
			)

		ds = ds.Where(
			goqu.Or(
				goqu.L("EXISTS ?", assetIdRegexExists),
				goqu.L("EXISTS ?", idLinkRegexExists),
			),
		)
	}

	if peekLimit < 0 {
		return nil, "", common.NewErrBadRequest("Limit is too high.")
	}

	ds = ds.
		Order(inf.Col(common.ColCompanyDomain).Asc()).
		Limit(uint(peekLimit))

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return nil, "", common.NewInternalServerError("Failed to build Infrastructure Descriptor query. See server logs for details.")
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("Failed to query Infrastructure Descriptors. See server logs for details.")
	}
	defer func() {
		_ = rows.Close()
	}()

	descRows := make([]model.InfrastructureDescriptorRow, 0, peekLimit)
	for rows.Next() {
		var r model.InfrastructureDescriptorRow
		if err := rows.Scan(
			&r.DescID,
			&r.GlobalAssetID,
			&r.IDShort,
			&r.Name,
			&r.Domain,
			&r.IDStr,
			&r.AdministrativeInfoPayload,
			&r.DisplayNamePayload,
			&r.DescriptionPayload,
		); err != nil {
			return nil, "", common.NewInternalServerError("Failed to scan Infrastructure Descriptor row. See server logs for details.")
		}
		descRows = append(descRows, r)
	}
	if rows.Err() != nil {
		return nil, "", common.NewInternalServerError("Failed to iterate Infrastructure Descriptors. See server logs for details.")
	}

	var nextCursor string
	if len(descRows) > int(limit) {
		nextCursor = descRows[limit].IDStr
		descRows = descRows[:limit]
	}

	if len(descRows) == 0 {
		return []model.InfrastructureDescriptor{}, nextCursor, nil
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
	if len(descIDs) > 0 {
		endpointsByDesc, err = ReadEndpointsByDescriptorIDs(ctx, db, descIDs, "infrastructure")
		if err != nil {
			return nil, "", err
		}
	}

	nameOptionsByDesc := map[int64][]string{}
	if len(descIDs) > 0 {
		nameOptionsByDesc, err = readInfrastructureNameOptionsByDescriptorIDs(ctx, db, descIDs)
		if err != nil {
			return nil, "", err
		}
	}

	assetIDRegexByDesc := map[int64][]string{}
	if len(descIDs) > 0 {
		assetIDRegexByDesc, err = readInfrastructureAssetIDRegexPatternsByDescriptorIDs(ctx, db, descIDs)
		if err != nil {
			return nil, "", err
		}
	}

	idLinkRegexByDesc := map[int64][]string{}
	if len(descIDs) > 0 {
		idLinkRegexByDesc, err = readInfrastructureIDLinkRegexPatternsByDescriptorIDs(ctx, db, descIDs)
		if err != nil {
			return nil, "", err
		}
	}

	out := make([]model.InfrastructureDescriptor, 0, len(descRows))
	for _, r := range descRows {
		adminInfo, err := parseAdministrativeInfoPayload(r.AdministrativeInfoPayload)
		if err != nil {
			return nil, "", common.NewInternalServerError("INFDESC-LIST-ADMINPAYLOAD")
		}
		displayName, err := parseLangStringNamePayload(r.DisplayNamePayload)
		if err != nil {
			return nil, "", common.NewInternalServerError("INFDESC-LIST-DISPLAYNAMEPAYLOAD")
		}
		description, err := parseLangStringTextPayload(r.DescriptionPayload)
		if err != nil {
			return nil, "", common.NewInternalServerError("INFDESC-LIST-DESCRIPTIONPAYLOAD")
		}

		out = append(out, model.InfrastructureDescriptor{
			GlobalAssetId:        r.GlobalAssetID.String,
			IdShort:              r.IDShort.String,
			Name:                 r.Name.String,
			Domain:               r.Domain.String,
			NameOptions:          nameOptionsByDesc[r.DescID],
			AssetIdRegexPatterns: assetIDRegexByDesc[r.DescID],
			IdLinkRegexPatterns:  idLinkRegexByDesc[r.DescID],
			Administration:       adminInfo,
			DisplayName:          displayName,
			Description:          description,
			Endpoints:            endpointsByDesc[r.DescID],
		})
	}

	return out, nextCursor, nil
}

// ExistsInfrastructureByID performs a lightweight existence check for an Infrastructure by its Id
// string. It returns true when a descriptor exists, false when it does not.
func ExistsInfrastructureByID(ctx context.Context, db *sql.DB, infrastructureIdentifier string) (bool, error) {
	d := goqu.Dialect(common.Dialect)
	inf := goqu.T(common.TblInfrastructureDescriptor).As("inf")

	ds := d.From(inf).Select(goqu.L("1")).Where(inf.Col(common.ColCompanyDomain).Eq(infrastructureIdentifier)).Limit(1)
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
