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
	"github.com/lib/pq"
)

// InsertCompanyDescriptor creates a new CompanyDescriptor
// and all its related entities (display name, description,
// administration, and endpoints).
//
// The operation runs in its own database transaction. If any part of the write
// fails, the transaction is rolled back and no partial data is left behind.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - companyDescriptor: descriptor to persist
//
// Returns an error when SQL building/execution fails or when writing any of the
// dependent rows fails. Errors are wrapped into common errors where relevant.
func InsertCompanyDescriptor(ctx context.Context, db *sql.DB, companyDescriptor model.CompanyDescriptor) (model.CompanyDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.CompanyDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()
	if err = InsertCompanyDescriptorTx(ctx, tx, companyDescriptor); err != nil {
		_ = tx.Rollback()
		return model.CompanyDescriptor{}, err
	}
	result, err := GetCompanyDescriptorByIDTx(ctx, tx, companyDescriptor.Domain)
	if err != nil {
		_ = tx.Rollback()
		return model.CompanyDescriptor{}, err
	}
	return result, tx.Commit()
}

// InsertCompanyDescriptorTx performs the same insert as
// InsertCompanyDescriptor but uses the provided transaction. This allows
// callers to compose multiple writes into a single atomic unit.
//
// The function inserts the base descriptor row first and then creates related
// entities (display name/description/admin info/endpoints). If any step fails,
// the error is returned and the caller is responsible for rolling back the transaction.
func InsertCompanyDescriptorTx(_ context.Context, tx *sql.Tx, comdesc model.CompanyDescriptor) error {
	if err := model.AssertCompanyDescriptorConstraints(comdesc); err != nil {
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

	descriptionPayload, err := buildLangStringTextPayload(comdesc.Description)
	if err != nil {
		return common.NewInternalServerError("COMDESC-INSERT-DESCRIPTIONPAYLOAD")
	}
	displayNamePayload, err := buildLangStringNamePayload(comdesc.DisplayName)
	if err != nil {
		return common.NewInternalServerError("COMDESC-INSERT-DISPLAYNAMEPAYLOAD")
	}
	administrationPayload, err := buildAdministrativeInfoPayload(comdesc.Administration)
	if err != nil {
		return common.NewInternalServerError("COMDESC-INSERT-ADMINPAYLOAD")
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
		if mappedErr := mapInsertCompanyDescriptorError(err); mappedErr != nil {
			return mappedErr
		}
		return err
	}

	sqlStr, args, buildErr = d.
		Insert(common.TblCompanyDescriptor).
		Rows(goqu.Record{
			common.ColDescriptorID:  descriptorID,
			common.ColGlobalAssetID: comdesc.GlobalAssetId,
			common.ColIDShort:       comdesc.IdShort,
			common.ColCompanyName:   comdesc.Name,
			common.ColCompanyDomain: comdesc.Domain,
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		if mappedErr := mapInsertCompanyDescriptorError(err); mappedErr != nil {
			return mappedErr
		}
		return err
	}

	if err = CreateEndpoints(tx, descriptorID, comdesc.Endpoints); err != nil {
		return common.NewInternalServerError("Failed to create Endpoints - no changes applied - see console for details")
	}

	if err = createCompanyNameOptions(tx, descriptorID, comdesc.NameOptions); err != nil {
		return common.NewInternalServerError("Failed to create Company Name Options - no changes applied - see console for details")
	}

	if err = createCompanyAssetIDRegexPatterns(tx, descriptorID, comdesc.AssetIdRegexPatterns); err != nil {
		return common.NewInternalServerError("Failed to create Company AssetID Regex Patterns - no changes applied - see console for details")
	}

	return nil
}

func mapInsertCompanyDescriptorError(err error) error {
	if err == nil {
		return nil
	}

	pqErr, ok := err.(*pq.Error)
	if !ok {
		return nil
	}

	if pqErr.Code == "23505" {
		return common.NewErrConflict("ROI-COMDESC-INSERT-CONFLICT Company Descriptor with given domain already exists")
	}

	return nil
}

// GetCompanyDescriptorByID returns a fully materialized
// CompanyDescriptor by its company domain identifier string.
// The function loads optional related entities (administration, display name,
// description, and endpoints) concurrently to minimize latency. If the
// company does not exist, a NotFound error is returned.
func GetCompanyDescriptorByID(ctx context.Context, db *sql.DB, companyIdentifier string) (model.CompanyDescriptor, error) {
	d := goqu.Dialect(common.Dialect)

	comp := goqu.T(common.TblCompanyDescriptor).As("comp")
	payload := common.TDescriptorPayload.As("comp_payload")

	sqlStr, args, buildErr := d.
		From(comp).
		LeftJoin(
			payload,
			goqu.On(payload.Col(common.ColDescriptorID).Eq(comp.Col(common.ColDescriptorID))),
		).
		Select(
			comp.Col(common.ColDescriptorID),
			comp.Col(common.ColGlobalAssetID),
			comp.Col(common.ColIDShort),
			comp.Col(common.ColCompanyName),
			comp.Col(common.ColCompanyDomain),
			payload.Col(common.ColAdministrativeInfoPayload),
			payload.Col(common.ColDisplayNamePayload),
			payload.Col(common.ColDescriptionPayload),
		).
		Where(comp.Col(common.ColCompanyDomain).Eq(companyIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return model.CompanyDescriptor{}, buildErr
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
			return model.CompanyDescriptor{}, common.NewErrNotFound("Company Descriptor not found")
		}
		return model.CompanyDescriptor{}, err
	}

	adminInfo, err := parseAdministrativeInfoPayload(administrativeInfoPayload)
	if err != nil {
		return model.CompanyDescriptor{}, common.NewInternalServerError("COMDESC-READ-ADMINPAYLOAD")
	}
	displayName, err := parseLangStringNamePayload(displayNamePayload)
	if err != nil {
		return model.CompanyDescriptor{}, common.NewInternalServerError("COMDESC-READ-DISPLAYNAMEPAYLOAD")
	}
	description, err := parseLangStringTextPayload(descriptionPayload)
	if err != nil {
		return model.CompanyDescriptor{}, common.NewInternalServerError("COMDESC-READ-DESCRIPTIONPAYLOAD")
	}
	endpoints, err := ReadEndpointsByDescriptorID(ctx, db, descID, "company")
	if err != nil {
		return model.CompanyDescriptor{}, err
	}
	nameOptions, err := readCompanyNameOptionsByDescriptorID(ctx, db, descID)
	if err != nil {
		return model.CompanyDescriptor{}, err
	}
	assetIDRegexPatterns, err := readCompanyAssetIDRegexPatternsByDescriptorID(ctx, db, descID)
	if err != nil {
		return model.CompanyDescriptor{}, err
	}

	return model.CompanyDescriptor{
		GlobalAssetId:        globalAssetID.String,
		IdShort:              idShort.String,
		Name:                 name.String,
		Domain:               domain.String,
		NameOptions:          nameOptions,
		AssetIdRegexPatterns: assetIDRegexPatterns,
		Administration:       adminInfo,
		DisplayName:          displayName,
		Description:          description,
		Endpoints:            endpoints,
	}, nil
}

// GetCompanyDescriptorByIDTx returns a fully materialized
// CompanyDescriptor by its company domain identifier string using the provided
// transaction. It avoids concurrent queries, which are unsafe on *sql.Tx.
func GetCompanyDescriptorByIDTx(ctx context.Context, tx *sql.Tx, companyIdentifier string) (model.CompanyDescriptor, error) {
	d := goqu.Dialect(common.Dialect)

	comp := goqu.T(common.TblCompanyDescriptor).As("comp")
	payload := common.TDescriptorPayload.As("comp_payload")

	sqlStr, args, buildErr := d.
		From(comp).
		LeftJoin(
			payload,
			goqu.On(payload.Col(common.ColDescriptorID).Eq(comp.Col(common.ColDescriptorID))),
		).
		Select(
			comp.Col(common.ColDescriptorID),
			comp.Col(common.ColGlobalAssetID),
			comp.Col(common.ColIDShort),
			comp.Col(common.ColCompanyName),
			comp.Col(common.ColCompanyDomain),
			payload.Col(common.ColAdministrativeInfoPayload),
			payload.Col(common.ColDisplayNamePayload),
			payload.Col(common.ColDescriptionPayload),
		).
		Where(comp.Col(common.ColCompanyDomain).Eq(companyIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return model.CompanyDescriptor{}, buildErr
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
			return model.CompanyDescriptor{}, common.NewErrNotFound("Company Descriptor not found")
		}
		return model.CompanyDescriptor{}, err
	}
	adminInfo, err := parseAdministrativeInfoPayload(administrativeInfoPayload)
	if err != nil {
		return model.CompanyDescriptor{}, common.NewInternalServerError("COMDESC-READ-ADMINPAYLOAD")
	}
	displayName, err := parseLangStringNamePayload(displayNamePayload)
	if err != nil {
		return model.CompanyDescriptor{}, common.NewInternalServerError("COMDESC-READ-DISPLAYNAMEPAYLOAD")
	}
	description, err := parseLangStringTextPayload(descriptionPayload)
	if err != nil {
		return model.CompanyDescriptor{}, common.NewInternalServerError("COMDESC-READ-DESCRIPTIONPAYLOAD")
	}
	endpoints, err := ReadEndpointsByDescriptorID(ctx, tx, descID, "company")
	if err != nil {
		return model.CompanyDescriptor{}, err
	}
	nameOptions, err := readCompanyNameOptionsByDescriptorID(ctx, tx, descID)
	if err != nil {
		return model.CompanyDescriptor{}, err
	}
	assetIDRegexPatterns, err := readCompanyAssetIDRegexPatternsByDescriptorID(ctx, tx, descID)
	if err != nil {
		return model.CompanyDescriptor{}, err
	}

	return model.CompanyDescriptor{
		GlobalAssetId:        globalAssetID.String,
		IdShort:              idShort.String,
		Name:                 name.String,
		Domain:               domain.String,
		NameOptions:          nameOptions,
		AssetIdRegexPatterns: assetIDRegexPatterns,
		Administration:       adminInfo,
		DisplayName:          displayName,
		Description:          description,
		Endpoints:            endpoints,
	}, nil
}

// DeleteCompanyDescriptorByID deletes the descriptor for the
// given Company Descriptor Id string. Deletion happens on the base descriptor row with ON
// DELETE CASCADE removing dependent rows.
// The delete runs in its own transaction.
func DeleteCompanyDescriptorByID(ctx context.Context, db *sql.DB, companyIdentifier string) error {
	return WithTx(ctx, db, func(tx *sql.Tx) error {
		return DeleteCompanyDescriptorByIDTx(ctx, tx, companyIdentifier)
	})
}

// DeleteCompanyDescriptorByIDTx deletes using the provided
// transaction. It resolves the internal descriptor id and removes the base
// descriptor row. Dependent rows are removed via ON DELETE CASCADE.
func DeleteCompanyDescriptorByIDTx(ctx context.Context, tx *sql.Tx, companyIdentifier string) error {
	d := goqu.Dialect("postgres")
	comp := goqu.T(common.TblCompanyDescriptor).As("comp")

	sqlStr, args, buildErr := d.
		From(comp).
		Select(comp.Col(common.ColDescriptorID)).
		Where(comp.Col(common.ColCompanyDomain).Eq(companyIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}

	var descID int64
	if scanErr := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return common.NewErrNotFound("Company Descriptor not found")
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

// ReplaceCompanyDescriptor atomically replaces the descriptor with the same
// Company Id: if a descriptor exists it is deleted (base descriptor row), then
// the provided descriptor is inserted. Related rows are recreated from the input.
// The returned descriptor is the stored Company Descriptor after replacement.
func ReplaceCompanyDescriptor(ctx context.Context, db *sql.DB, companyDescriptor model.CompanyDescriptor) (model.CompanyDescriptor, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return model.CompanyDescriptor{}, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
		}
	}()

	// delete existing descriptor
	if err = DeleteCompanyDescriptorByIDTx(ctx, tx, companyDescriptor.Domain); err != nil {
		_ = tx.Rollback()
		return model.CompanyDescriptor{}, err
	}
	// insert new descriptor
	if err = InsertCompanyDescriptorTx(ctx, tx, companyDescriptor); err != nil {
		_ = tx.Rollback()
		return model.CompanyDescriptor{}, err
	}

	result, err := GetCompanyDescriptorByIDTx(ctx, tx, companyDescriptor.Domain)
	if err != nil {
		_ = tx.Rollback()
		return model.CompanyDescriptor{}, err
	}
	return result, tx.Commit()
}

// ListCompanyDescriptors lists Company Descriptors with optional
// filtering by name and assetId regex matching.
// Results are ordered by company domain identifier ascending and support
// cursor‑based pagination where the cursor is the company domain identifier
// of the first element to include (i.e. Id >= cursor).
//
// It returns the page of fully assembled descriptors and, when more results are
// available, a next cursor value (the Id immediately after the page). When
// limit <= 0, a conservative large default is applied.
//
// nolint:revive // complexity is 31 which is +1 above the allowed threshold of 30
func ListCompanyDescriptors(
	ctx context.Context,
	db *sql.DB,
	limit int32,
	cursor string,
	name string,
	assetID string,
) ([]model.CompanyDescriptor, string, error) {
	if limit <= 0 {
		limit = 100
	}
	peekLimit := int(limit) + 1

	d := goqu.Dialect(common.Dialect)
	comp := goqu.T(common.TblCompanyDescriptor).As("comp")
	payload := common.TDescriptorPayload.As("comp_payload")
	compNameOpt := goqu.T(common.TblCompanyDescriptorNameOption).As("comp_name_opt")
	assetIdPattern := goqu.T(common.TblCompanyDescriptorAssetIDRegex).As("comp_asset_id_pattern")

	ds := d.
		From(comp).
		LeftJoin(
			payload,
			goqu.On(payload.Col(common.ColDescriptorID).Eq(comp.Col(common.ColDescriptorID))),
		).
		Select(
			comp.Col(common.ColDescriptorID),
			comp.Col(common.ColGlobalAssetID),
			comp.Col(common.ColIDShort),
			comp.Col(common.ColCompanyName),
			comp.Col(common.ColCompanyDomain),
			comp.Col(common.ColCompanyDomain),
			payload.Col(common.ColAdministrativeInfoPayload),
			payload.Col(common.ColDisplayNamePayload),
			payload.Col(common.ColDescriptionPayload),
		)

	if cursor != "" {
		ds = ds.Where(comp.Col(common.ColCompanyDomain).Gte(cursor))
	}

	if strings.TrimSpace(name) != "" {
		nameLower := strings.ToLower(name)
		ds = ds.
			LeftJoin(
				compNameOpt,
				goqu.On(
					comp.Col(common.ColDescriptorID).Eq(compNameOpt.Col(common.ColDescriptorID)),
					goqu.Func("LOWER", compNameOpt.Col(common.ColNameOption)).Eq(nameLower),
				),
			).
			Where(
				goqu.Or(
					goqu.Func("LOWER", comp.Col(common.ColCompanyName)).Eq(nameLower),
					compNameOpt.Col(common.ColDescriptorID).IsNotNull(),
				),
			)
	}

	if strings.TrimSpace(assetID) != "" {
		assetIdRegexExists := d.
			From(assetIdPattern).
			Select(goqu.V(true)).
			Where(
				assetIdPattern.Col(common.ColDescriptorID).Eq(comp.Col(common.ColDescriptorID)),
				goqu.L("? ~ ?", assetID, assetIdPattern.Col(common.ColRegexPattern)),
			)

		ds = ds.Where(goqu.L("EXISTS ?", assetIdRegexExists))
	}

	if peekLimit < 0 {
		return nil, "", common.NewErrBadRequest("Limit is too high.")
	}

	ds = ds.
		Order(comp.Col(common.ColCompanyDomain).Asc()).
		Limit(uint(peekLimit))

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return nil, "", common.NewInternalServerError("Failed to build Company Descriptor query. See server logs for details.")
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("Failed to query Company Descriptors. See server logs for details.")
	}
	defer func() {
		_ = rows.Close()
	}()

	descRows := make([]model.CompanyDescriptorRow, 0, peekLimit)
	for rows.Next() {
		var r model.CompanyDescriptorRow
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
			return nil, "", common.NewInternalServerError("Failed to scan Company Descriptor row. See server logs for details.")
		}
		descRows = append(descRows, r)
	}
	if rows.Err() != nil {
		return nil, "", common.NewInternalServerError("Failed to iterate Company Descriptors. See server logs for details.")
	}

	var nextCursor string
	if len(descRows) > int(limit) {
		nextCursor = descRows[limit].IDStr
		descRows = descRows[:limit]
	}

	if len(descRows) == 0 {
		return []model.CompanyDescriptor{}, nextCursor, nil
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
		endpointsByDesc, err = ReadEndpointsByDescriptorIDs(ctx, db, descIDs, "company")
		if err != nil {
			return nil, "", err
		}
	}

	nameOptionsByDesc := map[int64][]string{}
	if len(descIDs) > 0 {
		nameOptionsByDesc, err = readCompanyNameOptionsByDescriptorIDs(ctx, db, descIDs)
		if err != nil {
			return nil, "", err
		}
	}

	assetIDRegexByDesc := map[int64][]string{}
	if len(descIDs) > 0 {
		assetIDRegexByDesc, err = readCompanyAssetIDRegexPatternsByDescriptorIDs(ctx, db, descIDs)
		if err != nil {
			return nil, "", err
		}
	}

	out := make([]model.CompanyDescriptor, 0, len(descRows))
	for _, r := range descRows {
		adminInfo, err := parseAdministrativeInfoPayload(r.AdministrativeInfoPayload)
		if err != nil {
			return nil, "", common.NewInternalServerError("COMDESC-LIST-ADMINPAYLOAD")
		}
		displayName, err := parseLangStringNamePayload(r.DisplayNamePayload)
		if err != nil {
			return nil, "", common.NewInternalServerError("COMDESC-LIST-DISPLAYNAMEPAYLOAD")
		}
		description, err := parseLangStringTextPayload(r.DescriptionPayload)
		if err != nil {
			return nil, "", common.NewInternalServerError("COMDESC-LIST-DESCRIPTIONPAYLOAD")
		}

		out = append(out, model.CompanyDescriptor{
			GlobalAssetId:        r.GlobalAssetID.String,
			IdShort:              r.IDShort.String,
			Name:                 r.Name.String,
			Domain:               r.Domain.String,
			NameOptions:          nameOptionsByDesc[r.DescID],
			AssetIdRegexPatterns: assetIDRegexByDesc[r.DescID],
			Administration:       adminInfo,
			DisplayName:          displayName,
			Description:          description,
			Endpoints:            endpointsByDesc[r.DescID],
		})
	}

	return out, nextCursor, nil
}

// ExistsCompanyDescriptorByID performs a lightweight existence check for a company descriptor by its identifier
// string. It returns true when a descriptor exists, false when it does not.
func ExistsCompanyDescriptorByID(ctx context.Context, db *sql.DB, companyIdentifier string) (bool, error) {
	d := goqu.Dialect(common.Dialect)
	comp := goqu.T(common.TblCompanyDescriptor).As("comp")

	ds := d.From(comp).Select(goqu.L("1")).Where(comp.Col(common.ColCompanyDomain).Eq(companyIdentifier)).Limit(1)
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
