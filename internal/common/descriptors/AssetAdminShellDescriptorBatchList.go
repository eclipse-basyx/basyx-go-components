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
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type aasDescriptorListChildren struct {
	endpointsByDescriptor map[int64][]model.Endpoint
	specificByDescriptor  map[int64][]types.ISpecificAssetID
	submodelsByDescriptor map[int64][]model.SubmodelDescriptor
}

func listAssetAdministrationShellDescriptorsBatched(
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
	db = withDescriptorDebugQueryer(ctx, db)
	if limit <= 0 {
		limit = 100
	}
	peekLimit := limit
	if peekLimit < 1<<31-1 {
		peekLimit++
	}

	query, args, err := buildBatchedAASDescriptorPageQuery(
		ctx,
		peekLimit,
		cursor,
		assetKind,
		assetType,
		identifiable,
		createdFrom,
		updatedFrom,
	)
	if err != nil {
		return nil, "", err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREG-LISTAAS-QUERYPAGE " + err.Error())
	}
	descriptorRows, err := scanAASDescriptorListRows(rows, peekLimit)
	_ = rows.Close()
	if err != nil {
		return nil, "", err
	}

	descriptorRows, nextCursor := applyCursorLimit(descriptorRows, limit, func(row model.AssetAdministrationShellDescriptorRow) string {
		return row.IDStr
	})
	if len(descriptorRows) == 0 {
		return []model.AssetAdministrationShellDescriptor{}, nextCursor, nil
	}

	descriptorIDs := uniqueAASDescriptorListIDs(descriptorRows)
	children, err := readAASDescriptorListChildren(ctx, db, descriptorIDs)
	if err != nil {
		return nil, "", err
	}
	descriptors, err := assembleAASDescriptorList(descriptorRows, children)
	if err != nil {
		return nil, "", err
	}
	return descriptors, nextCursor, nil
}

func buildBatchedAASDescriptorPageQuery(
	ctx context.Context,
	peekLimit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	identifiable string,
	createdFrom time.Time,
	updatedFrom time.Time,
) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	collector, err := newAASDescriptorJSONCollector(common.TblAASDescriptor, common.ColDescriptorID, common.TblAASDescriptor)
	if err != nil {
		return "", nil, err
	}
	page, err := buildListAASDescriptorPageQuery(
		ctx,
		peekLimit,
		cursor,
		assetKind,
		assetType,
		identifiable,
		createdFrom,
		updatedFrom,
		collector,
	)
	if err != nil {
		return "", nil, err
	}
	if cursor != "" {
		cursorAAS := goqu.T(common.TblAASDescriptor).As("cursor_aas")
		cursorExists := dialect.From(cursorAAS).
			Select(goqu.L("1")).
			Where(cursorAAS.Col(common.ColAASID).Eq(cursor)).
			Limit(1)
		page = page.Where(goqu.L("EXISTS ?", cursorExists))
	}

	const pageAlias = "aas_descriptor_page"
	pageTable := goqu.T(pageAlias)
	aas := goqu.T(common.TblAASDescriptor).As("aas_descriptor")
	payload := goqu.T(common.TblDescriptorPayload).As("aas_descriptor_payload")
	var createdAt exp.Expression = goqu.L("NULL")
	if includeAASDescriptorCreatedAtFromContext(ctx) {
		createdAt = aas.Col(common.ColCreatedAt)
	}
	dataset := dialect.From(page.As(pageAlias)).
		InnerJoin(aas, goqu.On(aas.Col(common.ColDescriptorID).Eq(pageTable.Col(common.ColDescriptorID)))).
		LeftJoin(payload, goqu.On(payload.Col(common.ColDescriptorID).Eq(pageTable.Col(common.ColDescriptorID)))).
		Select(
			aas.Col(common.ColDescriptorID),
			aas.Col(common.ColAssetKind),
			aas.Col(common.ColAssetType),
			aas.Col(common.ColGlobalAssetID),
			aas.Col(common.ColIDShort),
			aas.Col(common.ColAASID),
			createdAt,
			payload.Col(common.ColAdministrativeInfoPayload),
			payload.Col(common.ColDisplayNamePayload),
			payload.Col(common.ColDescriptionPayload),
			payload.Col(common.ColExtensionsPayload),
		).
		Order(pageTable.Col("sort_aas_id").Asc()).
		Prepared(true)
	query, args, err := dataset.ToSQL()
	if err != nil {
		return "", nil, common.NewInternalServerError("AASREG-LISTAAS-BUILDPAGE " + err.Error())
	}
	return query, args, nil
}

func scanAASDescriptorListRows(rows *sql.Rows, capacity int32) ([]model.AssetAdministrationShellDescriptorRow, error) {
	descriptorRows := make([]model.AssetAdministrationShellDescriptorRow, 0, capacity)
	for rows.Next() {
		var row model.AssetAdministrationShellDescriptorRow
		if err := rows.Scan(
			&row.DescID,
			&row.AssetKind,
			&row.AssetType,
			&row.GlobalAssetID,
			&row.IDShort,
			&row.IDStr,
			&row.CreatedAt,
			&row.AdministrativeInfoPayload,
			&row.DisplayNamePayload,
			&row.DescriptionPayload,
			&row.ExtensionsPayload,
		); err != nil {
			return nil, common.NewInternalServerError("AASREG-LISTAAS-SCANPAGE " + err.Error())
		}
		descriptorRows = append(descriptorRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, common.NewInternalServerError("AASREG-LISTAAS-ITERATEPAGE " + err.Error())
	}
	return descriptorRows, nil
}

func uniqueAASDescriptorListIDs(rows []model.AssetAdministrationShellDescriptorRow) []int64 {
	ids := make([]int64, 0, len(rows))
	seen := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if _, exists := seen[row.DescID]; exists {
			continue
		}
		seen[row.DescID] = struct{}{}
		ids = append(ids, row.DescID)
	}
	return ids
}

func readAASDescriptorListChildren(
	ctx context.Context,
	db DBQueryer,
	descriptorIDs []int64,
) (aasDescriptorListChildren, error) {
	children := aasDescriptorListChildren{}
	var err error
	children.endpointsByDescriptor, err = ReadEndpointsByDescriptorIDs(ctx, db, descriptorIDs, "aas")
	if err != nil {
		return aasDescriptorListChildren{}, common.NewInternalServerError("AASREG-LISTAAS-READENDPOINTS " + err.Error())
	}
	children.specificByDescriptor, err = ReadSpecificAssetIDsByDescriptorIDs(ctx, db, descriptorIDs)
	if err != nil {
		return aasDescriptorListChildren{}, common.NewInternalServerError("AASREG-LISTAAS-READSPECIFICASSETIDS " + err.Error())
	}
	children.submodelsByDescriptor, err = ReadSubmodelDescriptorsByAASDescriptorIDs(ctx, db, descriptorIDs, false)
	if err != nil {
		return aasDescriptorListChildren{}, common.NewInternalServerError("AASREG-LISTAAS-READSUBMODELS " + err.Error())
	}
	return children, nil
}

func assembleAASDescriptorList(
	rows []model.AssetAdministrationShellDescriptorRow,
	children aasDescriptorListChildren,
) ([]model.AssetAdministrationShellDescriptor, error) {
	descriptors := make([]model.AssetAdministrationShellDescriptor, 0, len(rows))
	for _, row := range rows {
		descriptor, err := assembleAASDescriptorListRow(row, children)
		if err != nil {
			return nil, err
		}
		descriptors = append(descriptors, descriptor)
	}
	return descriptors, nil
}

func assembleAASDescriptorListRow(
	row model.AssetAdministrationShellDescriptorRow,
	children aasDescriptorListChildren,
) (model.AssetAdministrationShellDescriptor, error) {
	var assetKind *types.AssetKind
	if row.AssetKind.Valid {
		value := types.AssetKind(row.AssetKind.Int64)
		assetKind = &value
	}
	administration, err := parseAdministrativeInfoPayload(row.AdministrativeInfoPayload)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, common.NewInternalServerError("AASREG-LISTAAS-PARSEADMINISTRATION " + err.Error())
	}
	displayName, err := parseLangStringNamePayload(row.DisplayNamePayload)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, common.NewInternalServerError("AASREG-LISTAAS-PARSEDISPLAYNAME " + err.Error())
	}
	description, err := parseLangStringTextPayload(row.DescriptionPayload)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, common.NewInternalServerError("AASREG-LISTAAS-PARSEDESCRIPTION " + err.Error())
	}
	extensions, err := parseExtensionsPayload(row.ExtensionsPayload)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, common.NewInternalServerError("AASREG-LISTAAS-PARSEEXTENSIONS " + err.Error())
	}
	return model.AssetAdministrationShellDescriptor{
		AssetKind:           assetKind,
		AssetType:           row.AssetType.String,
		GlobalAssetId:       row.GlobalAssetID.String,
		IdShort:             row.IDShort.String,
		Id:                  row.IDStr,
		CreatedAt:           nullTimeToPtr(row.CreatedAt),
		Administration:      administration,
		DisplayName:         displayName,
		Description:         description,
		Endpoints:           children.endpointsByDescriptor[row.DescID],
		SpecificAssetIds:    children.specificByDescriptor[row.DescID],
		Extensions:          extensions,
		SubmodelDescriptors: children.submodelsByDescriptor[row.DescID],
	}, nil
}

func nullTimeToPtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
