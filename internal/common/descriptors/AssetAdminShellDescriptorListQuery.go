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
// Author: Aaron Zielstorff (Fraunhofer IESE)

package descriptors

import (
	"context"
	"time"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const (
	submodelDescriptorSemanticReferenceTable = "submodel_descriptor_semantic_id_reference"
	specificAssetExternalReferenceTable      = "specific_asset_id_external_subject_id_reference"
)

func useSingleStatementAASDescriptorList(
	ctx context.Context,
	identifiable string,
) bool {
	queryFilter := auth.GetQueryFilter(ctx)
	return identifiable == "" && !hasRestrictiveFragmentFilters(queryFilter)
}

func hasRestrictiveFragmentFilters(queryFilter *auth.QueryFilter) bool {
	if queryFilter == nil {
		return false
	}
	for _, filter := range queryFilter.Filters {
		if filter.Boolean == nil || !*filter.Boolean {
			return true
		}
	}
	return false
}

func listAssetAdministrationShellDescriptorsSingleStatement(
	ctx context.Context,
	db DBQueryer,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	createdFrom time.Time,
	updatedFrom time.Time,
) ([]model.AssetAdministrationShellDescriptor, string, error) {
	db = withDescriptorDebugQueryer(ctx, db)
	if limit <= 0 {
		limit = 100
	}
	peekLimit := limit
	if limit < 1<<31-1 {
		peekLimit++
	}

	ds, err := buildSingleStatementAASDescriptorListQuery(
		ctx,
		peekLimit,
		cursor,
		assetKind,
		assetType,
		createdFrom,
		updatedFrom,
	)
	if err != nil {
		return nil, "", err
	}
	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREG-LISTAAS-BUILDQUERY " + err.Error())
	}
	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("AASREG-LISTAAS-QUERYDB " + err.Error())
	}
	defer func() { _ = rows.Close() }()

	descriptors := make([]model.AssetAdministrationShellDescriptor, 0, peekLimit)
	for rows.Next() {
		var payload []byte
		if scanErr := rows.Scan(&payload); scanErr != nil {
			return nil, "", common.NewInternalServerError("AASREG-LISTAAS-SCANROW " + scanErr.Error())
		}
		descriptor, decodeErr := model.DecodeStoredAssetAdministrationShellDescriptor(payload)
		if decodeErr != nil {
			return nil, "", common.NewInternalServerError("AASREG-LISTAAS-DECODE " + decodeErr.Error())
		}
		descriptors = append(descriptors, descriptor)
	}
	if err := rows.Err(); err != nil {
		return nil, "", common.NewInternalServerError("AASREG-LISTAAS-ITERATEROWS " + err.Error())
	}

	page, nextCursor := applyCursorLimit(descriptors, limit, func(descriptor model.AssetAdministrationShellDescriptor) string {
		return descriptor.Id
	})
	return page, nextCursor, nil
}

func buildSingleStatementAASDescriptorListQuery(
	ctx context.Context,
	peekLimit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
	createdFrom time.Time,
	updatedFrom time.Time,
) (*goqu.SelectDataset, error) {
	dialect := goqu.Dialect(common.Dialect)
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		return nil, err
	}
	page, err := buildListAASDescriptorPageQuery(
		ctx,
		peekLimit,
		cursor,
		assetKind,
		assetType,
		"",
		createdFrom,
		updatedFrom,
		collector,
	)
	if err != nil {
		return nil, err
	}
	if cursor != "" {
		cursorAAS := goqu.T(common.TblAASDescriptor).As("cursor_aas")
		cursorExists := dialect.From(cursorAAS).
			Select(goqu.L("1")).
			Where(cursorAAS.Col(common.ColAASID).Eq(cursor)).
			Limit(1)
		page = page.Where(goqu.L("EXISTS ?", cursorExists))
	}

	pageAlias := goqu.T("aas_page")
	aasData := goqu.T(common.TblAASDescriptor).As("aas_data")
	payload := goqu.T(common.TblDescriptorPayload).As("aas_payload")
	descriptorJSON := buildAASDescriptorJSON(ctx, dialect, aasData, payload)

	return dialect.From(page.As("aas_page")).
		InnerJoin(aasData, goqu.On(aasData.Col(common.ColDescriptorID).Eq(pageAlias.Col(common.ColDescriptorID)))).
		InnerJoin(payload, goqu.On(payload.Col(common.ColDescriptorID).Eq(pageAlias.Col(common.ColDescriptorID)))).
		Select(descriptorJSON).
		Order(pageAlias.Col("sort_aas_id").Asc()), nil
}

func buildAASDescriptorJSON(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	aas exp.AliasedExpression,
	payload exp.AliasedExpression,
) exp.Expression {
	fields := []interface{}{
		goqu.V("id"), aas.Col(common.ColAASID),
		goqu.V("idShort"), aas.Col(common.ColIDShort),
		goqu.V("assetKind"), buildAssetKindStringExpression(aas.Col(common.ColAssetKind)),
		goqu.V("assetType"), aas.Col(common.ColAssetType),
		goqu.V("globalAssetId"), aas.Col(common.ColGlobalAssetID),
		goqu.V("administration"), emptyJSONToNull(payload.Col(common.ColAdministrativeInfoPayload)),
		goqu.V("displayName"), payload.Col(common.ColDisplayNamePayload),
		goqu.V("description"), payload.Col(common.ColDescriptionPayload),
		goqu.V("extensions"), payload.Col(common.ColExtensionsPayload),
		goqu.V("endpoints"), buildEndpointArraySubquery(dialect, common.TblAASDescriptorEndpoint, aas.Col(common.ColDescriptorID)),
		goqu.V("specificAssetIds"), buildSpecificAssetIDArraySubquery(dialect, aas.Col(common.ColDescriptorID)),
		goqu.V("submodelDescriptors"), buildSubmodelDescriptorArraySubquery(dialect, aas.Col(common.ColDescriptorID)),
	}
	if includeAASDescriptorCreatedAtFromContext(ctx) {
		fields = append(fields, goqu.V("createdAt"), aas.Col(common.ColCreatedAt))
	}
	return goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object", fields...))
}

func buildEndpointArraySubquery(
	dialect goqu.DialectWrapper,
	table string,
	descriptorID exp.Expression,
) *goqu.SelectDataset {
	endpoint := goqu.T(table).As("endpoint_data")
	endpointJSON := goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object",
		goqu.V("interface"), endpoint.Col(common.ColInterface),
		goqu.V("protocolInformation"), goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object",
			goqu.V("href"), endpoint.Col(common.ColHref),
			goqu.V("endpointProtocol"), endpoint.Col(common.ColEndpointProtocol),
			goqu.V("endpointProtocolVersion"), endpoint.Col(common.ColEndpointProtocolVersion),
			goqu.V("subprotocol"), endpoint.Col(common.ColSubProtocol),
			goqu.V("subprotocolBody"), endpoint.Col(common.ColSubProtocolBody),
			goqu.V("subprotocolBodyEncoding"), endpoint.Col(common.ColSubProtocolBodyEncoding),
			goqu.V("securityAttributes"), endpoint.Col(common.ColSecurityAttributes),
		)),
	))
	return dialect.From(endpoint).
		Select(jsonArrayAggregate(endpointJSON, endpoint.Col(common.ColPosition))).
		Where(endpoint.Col(common.ColDescriptorID).Eq(descriptorID))
}

func buildSpecificAssetIDArraySubquery(dialect goqu.DialectWrapper, descriptorID exp.Expression) *goqu.SelectDataset {
	specificAssetID := goqu.T(common.TblSpecificAssetID).As("specific_asset_data")
	payload := goqu.T(common.TblSpecificAssetIDPayload).As("specific_asset_payload")
	externalReference := buildReferenceObjectSubquery(
		dialect,
		specificAssetExternalReferenceTable,
		common.ColID,
		specificAssetID.Col(common.ColID),
	)
	supplementalReferences := buildReferenceArraySubquery(
		dialect,
		common.TblSpecificAssetIDSuppSemantic,
		common.ColSpecificAssetIDID,
		specificAssetID.Col(common.ColID),
	)
	specificAssetJSON := goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object",
		goqu.V("name"), specificAssetID.Col(common.ColName),
		goqu.V("value"), specificAssetID.Col(common.ColValue),
		goqu.V("semanticId"), emptyJSONToNull(payload.Col("semantic_id_payload")),
		goqu.V("externalSubjectId"), externalReference,
		goqu.V("supplementalSemanticIds"), emptyJSONToNull(supplementalReferences),
	))
	return dialect.From(specificAssetID).
		LeftJoin(payload, goqu.On(payload.Col(common.ColSpecificAssetID).Eq(specificAssetID.Col(common.ColID)))).
		Select(jsonArrayAggregate(specificAssetJSON, specificAssetID.Col(common.ColPosition))).
		Where(
			specificAssetID.Col(common.ColDescriptorID).Eq(descriptorID),
			specificAssetID.Col(common.ColName).Neq(globalAssetIDSpecificAssetIDName),
		)
}

func buildSubmodelDescriptorArraySubquery(dialect goqu.DialectWrapper, aasDescriptorID exp.Expression) *goqu.SelectDataset {
	submodel := goqu.T(common.TblSubmodelDescriptor).As("submodel_descriptor_data")
	payload := goqu.T(common.TblDescriptorPayload).As("submodel_descriptor_payload")
	semanticReference := buildReferenceObjectSubquery(
		dialect,
		submodelDescriptorSemanticReferenceTable,
		common.ColID,
		submodel.Col(common.ColDescriptorID),
	)
	supplementalReferences := buildReferenceArraySubquery(
		dialect,
		common.TblSubmodelDescriptorSuppSemantic,
		common.ColDescriptorID,
		submodel.Col(common.ColDescriptorID),
	)
	submodelJSON := goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object",
		goqu.V("id"), submodel.Col(common.ColAASID),
		goqu.V("idShort"), submodel.Col(common.ColIDShort),
		goqu.V("administration"), emptyJSONToNull(payload.Col(common.ColAdministrativeInfoPayload)),
		goqu.V("displayName"), payload.Col(common.ColDisplayNamePayload),
		goqu.V("description"), payload.Col(common.ColDescriptionPayload),
		goqu.V("extensions"), payload.Col(common.ColExtensionsPayload),
		goqu.V("semanticId"), semanticReference,
		goqu.V("supplementalSemanticIds"), supplementalReferences,
		goqu.V("endpoints"), buildEndpointArraySubquery(dialect, common.TblAASDescriptorEndpoint, submodel.Col(common.ColDescriptorID)),
	))
	return dialect.From(submodel).
		InnerJoin(payload, goqu.On(payload.Col(common.ColDescriptorID).Eq(submodel.Col(common.ColDescriptorID)))).
		Select(jsonArrayAggregate(submodelJSON, submodel.Col(common.ColPosition), submodel.Col(common.ColDescriptorID))).
		Where(submodel.Col(common.ColAASDescriptorID).Eq(aasDescriptorID))
}

func buildReferenceObjectSubquery(
	dialect goqu.DialectWrapper,
	table string,
	ownerColumn string,
	ownerID exp.Expression,
) *goqu.SelectDataset {
	reference := goqu.T(table).As("single_reference")
	payload := goqu.T(table + "_payload").As("single_reference_payload")
	return dialect.From(reference).
		LeftJoin(payload, goqu.On(payload.Col(common.ColReferenceID).Eq(reference.Col(common.ColID)))).
		Select(buildReferenceJSON(dialect, table, reference, payload)).
		Where(reference.Col(ownerColumn).Eq(ownerID)).
		Limit(1)
}

func buildReferenceArraySubquery(
	dialect goqu.DialectWrapper,
	table string,
	ownerColumn string,
	ownerID exp.Expression,
) *goqu.SelectDataset {
	reference := goqu.T(table).As("reference_array_data")
	payload := goqu.T(table + "_payload").As("reference_array_payload")
	return dialect.From(reference).
		LeftJoin(payload, goqu.On(payload.Col(common.ColReferenceID).Eq(reference.Col(common.ColID)))).
		Select(jsonArrayAggregate(
			buildReferenceJSON(dialect, table, reference, payload),
			reference.Col(common.ColPosition),
			reference.Col(common.ColID),
		)).
		Where(reference.Col(ownerColumn).Eq(ownerID))
}

func buildReferenceJSON(
	dialect goqu.DialectWrapper,
	table string,
	reference exp.AliasedExpression,
	payload exp.AliasedExpression,
) exp.Expression {
	key := goqu.T(table + "_key").As("reference_key_data")
	keys := dialect.From(key).
		Select(jsonArrayAggregate(
			goqu.Func("jsonb_build_object",
				goqu.V("type"), buildKeyTypeStringExpression(key.Col(common.ColType)),
				goqu.V("value"), key.Col(common.ColValue),
			),
			key.Col(common.ColPosition),
			key.Col(common.ColID),
		)).
		Where(key.Col(common.ColReferenceID).Eq(reference.Col(common.ColID)))
	return goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object",
		goqu.V("type"), buildReferenceTypeStringExpression(reference.Col(common.ColType)),
		goqu.V("keys"), keys,
		goqu.V("referredSemanticId"), emptyJSONToNull(payload.Col("parent_reference_payload")),
	))
}

func jsonArrayAggregate(value exp.Expression, order ...exp.Expression) exp.Expression {
	orderExpressions := make([]interface{}, 0, len(order)+1)
	orderExpressions = append(orderExpressions, value)
	for _, expression := range order {
		orderExpressions = append(orderExpressions, expression)
	}
	orderSQL := "?"
	if len(order) > 0 {
		orderSQL += " ORDER BY "
		for index := range order {
			if index > 0 {
				orderSQL += ", "
			}
			orderSQL += "?"
		}
	}
	return goqu.COALESCE(goqu.Func("jsonb_agg", goqu.L(orderSQL, orderExpressions...)), goqu.L("'[]'::jsonb"))
}

func emptyJSONToNull(value exp.Expression) exp.Expression {
	return goqu.Func("NULLIF", goqu.Func("NULLIF", value, goqu.L("'[]'::jsonb")), goqu.L("'{}'::jsonb"))
}

func buildAssetKindStringExpression(value exp.Expression) exp.CaseExpression {
	result := goqu.Case().Value(value)
	for _, assetKind := range types.LiteralsOfAssetKind {
		result = result.When(int(assetKind), stringification.MustAssetKindToString(assetKind))
	}
	return result.Else(nil)
}

func buildReferenceTypeStringExpression(value exp.Expression) exp.CaseExpression {
	result := goqu.Case().Value(value)
	for _, referenceType := range types.LiteralsOfReferenceTypes {
		result = result.When(int(referenceType), stringification.MustReferenceTypesToString(referenceType))
	}
	return result.Else(nil)
}

func buildKeyTypeStringExpression(value exp.Expression) exp.CaseExpression {
	result := goqu.Case().Value(value)
	for _, keyType := range types.LiteralsOfKeyTypes {
		result = result.When(int(keyType), stringification.MustKeyTypesToString(keyType))
	}
	return result.Else(nil)
}
