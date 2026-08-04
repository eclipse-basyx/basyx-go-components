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

func listAssetAdministrationShellDescriptorsSingleStatement(
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
	if limit < 1<<31-1 {
		peekLimit++
	}

	ds, err := buildSingleStatementAASDescriptorListQuery(
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
	sqlStr, args, err := ds.Prepared(true).ToSQL()
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
	identifiable string,
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
		identifiable,
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

	const (
		pageAlias = "aas_page"
		dataAlias = "aas_descriptor_json_data"
	)
	pageTable := goqu.T(pageAlias)
	aasSource := goqu.T(common.TblAASDescriptor).As(common.TblAASDescriptor)
	payloadSource := goqu.T(common.TblDescriptorPayload).As("aas_payload")
	maskCollector, err := newAASDescriptorJSONCollector(common.TblAASDescriptor, common.ColDescriptorID, common.TblAASDescriptor)
	if err != nil {
		return nil, err
	}
	maskedColumns := []auth.MaskedInnerColumnSpec{
		{Fragment: "$aasdesc#idShort", FlagAlias: "flag_idshort", RawAlias: common.ColIDShort},
		{Fragment: "$aasdesc#assetKind", FlagAlias: "flag_assetkind", RawAlias: common.ColAssetKind},
		{Fragment: "$aasdesc#assetType", FlagAlias: "flag_assettype", RawAlias: common.ColAssetType},
		{Fragment: "$aasdesc#globalAssetId", FlagAlias: "flag_globalassetid", RawAlias: common.ColGlobalAssetID},
		{Fragment: "$aasdesc#administration", FlagAlias: "flag_administration", RawAlias: common.ColAdministrativeInfoPayload},
		{Fragment: "$aasdesc#displayName", FlagAlias: "flag_displayname", RawAlias: common.ColDisplayNamePayload},
		{Fragment: "$aasdesc#description", FlagAlias: "flag_description", RawAlias: common.ColDescriptionPayload},
		{Fragment: "$aasdesc#extension", FlagAlias: "flag_extension", RawAlias: common.ColExtensionsPayload},
	}
	includeCreatedAt := includeAASDescriptorCreatedAtFromContext(ctx)
	if includeCreatedAt {
		maskedColumns = append(maskedColumns, auth.MaskedInnerColumnSpec{
			Fragment: "$aasdesc#createdAt", FlagAlias: "flag_createdat", RawAlias: common.ColCreatedAt,
		})
	}
	maskRuntime, err := buildOptionalDescriptorMaskRuntime(ctx, maskCollector, maskedColumns)
	if err != nil {
		return nil, err
	}
	if maskRuntime == nil {
		aas := goqu.T(common.TblAASDescriptor)
		payload := goqu.T("aas_payload")
		rawExpressions := []exp.Expression{
			aas.Col(common.ColIDShort),
			aas.Col(common.ColAssetKind),
			aas.Col(common.ColAssetType),
			aas.Col(common.ColGlobalAssetID),
			payload.Col(common.ColAdministrativeInfoPayload),
			payload.Col(common.ColDisplayNamePayload),
			payload.Col(common.ColDescriptionPayload),
			payload.Col(common.ColExtensionsPayload),
		}
		if includeCreatedAt {
			rawExpressions = append(rawExpressions, aas.Col(common.ColCreatedAt))
		}
		descriptorJSON, err := buildAASDescriptorJSON(
			ctx,
			dialect,
			common.TblAASDescriptor,
			aas,
			rawExpressions,
		)
		if err != nil {
			return nil, err
		}
		return dialect.From(page.As(pageAlias)).
			InnerJoin(aasSource, goqu.On(aas.Col(common.ColDescriptorID).Eq(pageTable.Col(common.ColDescriptorID)))).
			InnerJoin(payloadSource, goqu.On(payload.Col(common.ColDescriptorID).Eq(pageTable.Col(common.ColDescriptorID)))).
			Select(descriptorJSON).
			Order(pageTable.Col("sort_aas_id").Asc()), nil
	}
	dataColumns := []interface{}{
		aasSource.Col(common.ColDescriptorID).As(common.ColDescriptorID),
		aasSource.Col(common.ColAASID).As(common.ColAASID),
		aasSource.Col(common.ColIDShort).As(common.ColIDShort),
		aasSource.Col(common.ColAssetKind).As(common.ColAssetKind),
		aasSource.Col(common.ColAssetType).As(common.ColAssetType),
		aasSource.Col(common.ColGlobalAssetID).As(common.ColGlobalAssetID),
		payloadSource.Col(common.ColAdministrativeInfoPayload).As(common.ColAdministrativeInfoPayload),
		payloadSource.Col(common.ColDisplayNamePayload).As(common.ColDisplayNamePayload),
		payloadSource.Col(common.ColDescriptionPayload).As(common.ColDescriptionPayload),
		payloadSource.Col(common.ColExtensionsPayload).As(common.ColExtensionsPayload),
		pageTable.Col("sort_aas_id").As("sort_aas_id"),
	}
	if includeCreatedAt {
		dataColumns = append(dataColumns, aasSource.Col(common.ColCreatedAt).As(common.ColCreatedAt))
	}
	dataDS := dialect.From(page.As(pageAlias)).
		InnerJoin(aasSource, goqu.On(aasSource.Col(common.ColDescriptorID).Eq(pageTable.Col(common.ColDescriptorID)))).
		InnerJoin(payloadSource, goqu.On(payloadSource.Col(common.ColDescriptorID).Eq(pageTable.Col(common.ColDescriptorID)))).
		Select(append(dataColumns, maskRuntime.Projections()...)...)
	maskedExpressions, err := descriptorMaskedExpressions(maskRuntime, dataAlias, maskedColumns)
	if err != nil {
		return nil, err
	}
	data := goqu.T(dataAlias)
	descriptorJSON, err := buildAASDescriptorJSON(ctx, dialect, dataAlias, data, maskedExpressions)
	if err != nil {
		return nil, err
	}

	return dialect.From(dataDS.As(dataAlias)).
		Select(descriptorJSON).
		Order(data.Col("sort_aas_id").Asc()), nil
}

func buildAASDescriptorJSON(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	aasAlias string,
	aas exp.IdentifierExpression,
	maskedExpressions []exp.Expression,
) (exp.Expression, error) {
	endpointCollector, err := newAASDescriptorJSONCollector(
		aasAlias,
		common.ColDescriptorID,
		common.AliasAASDescriptorEndpoint,
	)
	if err != nil {
		return nil, err
	}
	endpoints, err := buildEndpointArraySubquery(
		ctx,
		dialect,
		common.TblAASDescriptorEndpoint,
		common.AliasAASDescriptorEndpoint,
		aas.Col(common.ColDescriptorID),
		"$aasdesc#endpoints[]",
		endpointCollector,
	)
	if err != nil {
		return nil, err
	}
	specificAssetIDs, err := buildSpecificAssetIDArraySubquery(ctx, dialect, aas.Col(common.ColDescriptorID))
	if err != nil {
		return nil, err
	}
	submodelDescriptors, err := buildSubmodelDescriptorArraySubquery(ctx, dialect, aas.Col(common.ColDescriptorID))
	if err != nil {
		return nil, err
	}

	fields := []descriptorJSONField{
		{name: "id", value: aas.Col(common.ColAASID)},
		{name: "idShort", value: maskedExpressions[0]},
		{name: "assetKind", value: buildAssetKindStringExpression(maskedExpressions[1])},
		{name: "assetType", value: maskedExpressions[2]},
		{name: "globalAssetId", value: maskedExpressions[3]},
		{name: "administration", value: emptyJSONToNull(maskedExpressions[4])},
		{name: "displayName", value: maskedExpressions[5]},
		{name: "description", value: maskedExpressions[6]},
		{name: "extensions", value: maskedExpressions[7]},
		{name: "endpoints", value: endpoints},
		{name: "specificAssetIds", value: specificAssetIDs},
		{name: "submodelDescriptors", value: submodelDescriptors},
	}
	if includeAASDescriptorCreatedAtFromContext(ctx) {
		fields = append(fields, descriptorJSONField{name: "createdAt", value: maskedExpressions[8]})
	}
	return buildMaskedJSONObject(ctx, nil, fields)
}

func buildEndpointArraySubquery(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	table string,
	alias string,
	descriptorID exp.Expression,
	fragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	endpoint := goqu.T(table).As(alias)
	endpointJSON := goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("interface"), endpoint.Col(common.ColInterface),
		common.PostgreSQLTextLiteral("protocolInformation"), goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object",
			common.PostgreSQLTextLiteral("href"), endpoint.Col(common.ColHref),
			common.PostgreSQLTextLiteral("endpointProtocol"), endpoint.Col(common.ColEndpointProtocol),
			common.PostgreSQLTextLiteral("endpointProtocolVersion"), endpoint.Col(common.ColEndpointProtocolVersion),
			common.PostgreSQLTextLiteral("subprotocol"), endpoint.Col(common.ColSubProtocol),
			common.PostgreSQLTextLiteral("subprotocolBody"), endpoint.Col(common.ColSubProtocolBody),
			common.PostgreSQLTextLiteral("subprotocolBodyEncoding"), endpoint.Col(common.ColSubProtocolBodyEncoding),
			common.PostgreSQLTextLiteral("securityAttributes"), endpoint.Col(common.ColSecurityAttributes),
		)),
	))
	ds := dialect.From(endpoint).
		Select(jsonArrayAggregate(endpointJSON, endpoint.Col(common.ColPosition))).
		Where(endpoint.Col(common.ColDescriptorID).Eq(descriptorID))
	return auth.AddCorrelatedFilterQueryFromContext(ctx, ds, fragment, collector)
}

func buildSpecificAssetIDArraySubquery(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	descriptorID exp.Expression,
) (*goqu.SelectDataset, error) {
	if !hasAASDescriptorFilters(ctx) {
		return buildUnfilteredSpecificAssetIDArraySubquery(ctx, dialect, descriptorID)
	}
	specificAssetID := goqu.T(common.TblSpecificAssetID).As(common.AliasSpecificAssetID)
	payload := goqu.T(common.TblSpecificAssetIDPayload).As("specific_asset_payload")
	externalReferenceRow := goqu.T(specificAssetExternalReferenceTable).As(common.AliasExternalSubjectReference)
	externalReferenceKey := goqu.T(specificAssetExternalReferenceTable + "_key").As(common.AliasExternalSubjectReferenceKey)
	collector, err := newAASDescriptorJSONCollector(
		common.AliasSpecificAssetID,
		common.ColDescriptorID,
		common.AliasSpecificAssetID,
		common.AliasExternalSubjectReference,
		common.AliasExternalSubjectReferenceKey,
	)
	if err != nil {
		return nil, err
	}
	const dataAlias = "specific_asset_json_data"
	maskedColumns := []auth.MaskedInnerColumnSpec{
		{Fragment: "$aasdesc#specificAssetIds[].name", FlagAlias: "flag_said_name", RawAlias: common.ColName},
		{Fragment: "$aasdesc#specificAssetIds[].value", FlagAlias: "flag_said_value", RawAlias: common.ColValue},
		{Fragment: "$aasdesc#specificAssetIds[].externalSubjectId", FlagAlias: "flag_said_external_subject", RawAlias: "external_reference_id"},
	}
	maskRuntime, err := buildOptionalDescriptorMaskRuntime(ctx, collector, maskedColumns)
	if err != nil {
		return nil, err
	}
	maskProjections, err := maskRuntime.BoolOrProjections(specificAssetID.Col(common.ColID))
	if err != nil {
		return nil, err
	}
	inner := dialect.From(specificAssetID).
		LeftJoin(payload, goqu.On(payload.Col(common.ColSpecificAssetID).Eq(specificAssetID.Col(common.ColID)))).
		LeftJoin(externalReferenceRow, goqu.On(externalReferenceRow.Col(common.ColID).Eq(specificAssetID.Col(common.ColID))))
	needsFilterJoins := maskRuntime != nil || hasAASDescriptorFragmentFilter(ctx, "$aasdesc#specificAssetIds[]")
	if needsFilterJoins {
		inner = inner.LeftJoin(
			externalReferenceKey,
			goqu.On(externalReferenceKey.Col(common.ColReferenceID).Eq(externalReferenceRow.Col(common.ColID))),
		)
	}
	inner = inner.
		Select(append([]interface{}{
			specificAssetID.Col(common.ColID).As(common.ColID),
			specificAssetID.Col(common.ColDescriptorID).As(common.ColDescriptorID),
			specificAssetID.Col(common.ColName).As(common.ColName),
			specificAssetID.Col(common.ColValue).As(common.ColValue),
			specificAssetID.Col(common.ColPosition).As(common.ColPosition),
			payload.Col("semantic_id_payload").As("semantic_id_payload"),
			externalReferenceRow.Col(common.ColID).As("external_reference_id"),
		}, maskProjections...)...).
		Where(
			specificAssetID.Col(common.ColDescriptorID).Eq(descriptorID),
			specificAssetID.Col(common.ColName).Neq(globalAssetIDSpecificAssetIDName),
		)
	if needsFilterJoins {
		inner = inner.Distinct()
	}
	inner, err = auth.AddCorrelatedFilterQueryFromContext(ctx, inner, "$aasdesc#specificAssetIds[]", collector)
	if err != nil {
		return nil, err
	}

	data := goqu.T(dataAlias)
	maskedExpressions, err := descriptorMaskedExpressions(maskRuntime, dataAlias, maskedColumns)
	if err != nil {
		return nil, err
	}
	externalReferenceCollector, err := newAASDescriptorJSONCollector(
		dataAlias,
		common.ColDescriptorID,
		common.AliasExternalSubjectReference,
		common.AliasExternalSubjectReferenceKey,
	)
	if err != nil {
		return nil, err
	}
	externalReference, err := buildReferenceObjectSubquery(
		ctx,
		dialect,
		specificAssetExternalReferenceTable,
		common.AliasExternalSubjectReference,
		common.AliasExternalSubjectReferenceKey,
		common.ColID,
		data.Col(common.ColID),
		"$aasdesc#specificAssetIds[].externalSubjectId.keys[]",
		externalReferenceCollector,
	)
	if err != nil {
		return nil, err
	}
	supplementalReferences, err := buildReferenceArraySubquery(
		ctx,
		dialect,
		common.TblSpecificAssetIDSuppSemantic,
		"specific_asset_supplemental_semantic_id_reference",
		"specific_asset_supplemental_semantic_id_reference_key",
		common.ColSpecificAssetIDID,
		data.Col(common.ColID),
		"",
		"",
		nil,
	)
	if err != nil {
		return nil, err
	}
	specificAssetJSON, err := buildMaskedJSONObject(ctx, nil, []descriptorJSONField{
		{name: "name", value: maskedExpressions[0]},
		{name: "value", value: maskedExpressions[1]},
		{name: "semanticId", value: emptyJSONToNull(data.Col("semantic_id_payload"))},
		{name: "externalSubjectId", value: goqu.Case().When(goqu.L("? IS NOT NULL", maskedExpressions[2]), externalReference).Else(nil)},
		{name: "supplementalSemanticIds", value: emptyJSONToNull(supplementalReferences)},
	})
	if err != nil {
		return nil, err
	}
	return dialect.From(inner.As(dataAlias)).
		Select(jsonArrayAggregate(specificAssetJSON, data.Col(common.ColPosition))), nil
}

func buildSubmodelDescriptorArraySubquery(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	aasDescriptorID exp.Expression,
) (*goqu.SelectDataset, error) {
	if !hasAASDescriptorFilters(ctx) {
		return buildUnfilteredSubmodelDescriptorArraySubquery(ctx, dialect, aasDescriptorID)
	}
	submodel := goqu.T(common.TblSubmodelDescriptor).As(common.AliasSubmodelDescriptor)
	payload := goqu.T(common.TblDescriptorPayload).As("submodel_descriptor_payload")
	semanticReferenceRow := goqu.T(submodelDescriptorSemanticReferenceTable).As(common.AliasSubmodelDescriptorSemanticIDReference)
	semanticReferenceKey := goqu.T(submodelDescriptorSemanticReferenceTable + "_key").As(common.AliasSubmodelDescriptorSemanticIDReferenceKey)
	const (
		supplementalReferenceAlias = "aasdesc_submodel_descriptor_supplemental_semantic_id_reference"
		supplementalKeyAlias       = "aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key"
		dataAlias                  = "submodel_descriptor_json_data"
	)
	rowFiltered := hasAASDescriptorFragmentFilter(ctx, "$aasdesc#submodelDescriptors[]")
	inlineAliases := []string{
		common.AliasSubmodelDescriptor,
		common.AliasSubmodelDescriptorSemanticIDReference,
		common.AliasSubmodelDescriptorSemanticIDReferenceKey,
	}
	if rowFiltered {
		inlineAliases = append(
			inlineAliases,
			common.AliasSubmodelDescriptorEndpoint,
			supplementalReferenceAlias,
			supplementalKeyAlias,
		)
	}
	collector, err := newAASDescriptorJSONCollector(
		common.AliasSubmodelDescriptor,
		common.ColAASDescriptorID,
		inlineAliases...,
	)
	if err != nil {
		return nil, err
	}
	maskedColumns := []auth.MaskedInnerColumnSpec{
		{Fragment: "$aasdesc#submodelDescriptors[].idShort", FlagAlias: "flag_smdesc_idshort", RawAlias: common.ColIDShort},
		{Fragment: "$aasdesc#submodelDescriptors[].semanticId", FlagAlias: "flag_smdesc_semanticid", RawAlias: "semantic_reference_id"},
	}
	maskRuntime, err := buildOptionalDescriptorMaskRuntime(ctx, collector, maskedColumns)
	if err != nil {
		return nil, err
	}
	maskProjections, err := maskRuntime.BoolOrProjections(submodel.Col(common.ColDescriptorID))
	if err != nil {
		return nil, err
	}
	inner := dialect.From(submodel).
		InnerJoin(payload, goqu.On(payload.Col(common.ColDescriptorID).Eq(submodel.Col(common.ColDescriptorID)))).
		LeftJoin(semanticReferenceRow, goqu.On(semanticReferenceRow.Col(common.ColID).Eq(submodel.Col(common.ColDescriptorID))))
	needsFilterJoins := maskRuntime != nil || rowFiltered
	if needsFilterJoins {
		inner = inner.LeftJoin(
			semanticReferenceKey,
			goqu.On(semanticReferenceKey.Col(common.ColReferenceID).Eq(semanticReferenceRow.Col(common.ColID))),
		)
	}
	if rowFiltered {
		endpoint := goqu.T(common.TblAASDescriptorEndpoint).As(common.AliasSubmodelDescriptorEndpoint)
		supplementalReference := goqu.T(common.TblSubmodelDescriptorSuppSemantic).As(supplementalReferenceAlias)
		supplementalKey := goqu.T(common.TblSubmodelDescriptorSuppSemantic + "_key").As(supplementalKeyAlias)
		inner = inner.
			LeftJoin(
				endpoint,
				goqu.On(endpoint.Col(common.ColDescriptorID).Eq(submodel.Col(common.ColDescriptorID))),
			).
			LeftJoin(
				supplementalReference,
				goqu.On(supplementalReference.Col(common.ColDescriptorID).Eq(submodel.Col(common.ColDescriptorID))),
			).
			LeftJoin(
				supplementalKey,
				goqu.On(supplementalKey.Col(common.ColReferenceID).Eq(supplementalReference.Col(common.ColID))),
			)
	}
	inner = inner.
		Select(append([]interface{}{
			submodel.Col(common.ColDescriptorID).As(common.ColDescriptorID),
			submodel.Col(common.ColAASDescriptorID).As(common.ColAASDescriptorID),
			submodel.Col(common.ColAASID).As(common.ColAASID),
			submodel.Col(common.ColIDShort).As(common.ColIDShort),
			submodel.Col(common.ColPosition).As(common.ColPosition),
			payload.Col(common.ColAdministrativeInfoPayload).As(common.ColAdministrativeInfoPayload),
			payload.Col(common.ColDisplayNamePayload).As(common.ColDisplayNamePayload),
			payload.Col(common.ColDescriptionPayload).As(common.ColDescriptionPayload),
			payload.Col(common.ColExtensionsPayload).As(common.ColExtensionsPayload),
			semanticReferenceRow.Col(common.ColID).As("semantic_reference_id"),
		}, maskProjections...)...).
		Where(submodel.Col(common.ColAASDescriptorID).Eq(aasDescriptorID))
	if needsFilterJoins {
		inner = inner.Distinct()
	}
	inner, err = auth.AddCorrelatedFilterQueryFromContext(ctx, inner, "$aasdesc#submodelDescriptors[]", collector)
	if err != nil {
		return nil, err
	}

	data := goqu.T(dataAlias)
	maskedExpressions, err := descriptorMaskedExpressions(maskRuntime, dataAlias, maskedColumns)
	if err != nil {
		return nil, err
	}
	semanticReferenceCollector, err := newAASDescriptorJSONCollector(
		dataAlias,
		common.ColAASDescriptorID,
		common.AliasSubmodelDescriptorSemanticIDReference,
		common.AliasSubmodelDescriptorSemanticIDReferenceKey,
	)
	if err != nil {
		return nil, err
	}
	semanticReference, err := buildReferenceObjectSubquery(
		ctx,
		dialect,
		submodelDescriptorSemanticReferenceTable,
		common.AliasSubmodelDescriptorSemanticIDReference,
		common.AliasSubmodelDescriptorSemanticIDReferenceKey,
		common.ColID,
		data.Col(common.ColDescriptorID),
		"$aasdesc#submodelDescriptors[].semanticId.keys[]",
		semanticReferenceCollector,
	)
	if err != nil {
		return nil, err
	}
	supplementalReferenceCollector, err := newAASDescriptorJSONCollector(
		dataAlias,
		common.ColAASDescriptorID,
		supplementalReferenceAlias,
		supplementalKeyAlias,
	)
	if err != nil {
		return nil, err
	}
	supplementalReferences, err := buildReferenceArraySubquery(
		ctx,
		dialect,
		common.TblSubmodelDescriptorSuppSemantic,
		supplementalReferenceAlias,
		supplementalKeyAlias,
		common.ColDescriptorID,
		data.Col(common.ColDescriptorID),
		"$aasdesc#submodelDescriptors[].supplementalSemanticIds[]",
		"$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[]",
		supplementalReferenceCollector,
	)
	if err != nil {
		return nil, err
	}
	endpointCollector, err := newAASDescriptorJSONCollector(
		dataAlias,
		common.ColAASDescriptorID,
		common.AliasSubmodelDescriptorEndpoint,
	)
	if err != nil {
		return nil, err
	}
	endpoints, err := buildEndpointArraySubquery(
		ctx,
		dialect,
		common.TblAASDescriptorEndpoint,
		common.AliasSubmodelDescriptorEndpoint,
		data.Col(common.ColDescriptorID),
		"$aasdesc#submodelDescriptors[].endpoints[]",
		endpointCollector,
	)
	if err != nil {
		return nil, err
	}
	submodelJSON, err := buildMaskedJSONObject(ctx, nil, []descriptorJSONField{
		{name: "id", value: data.Col(common.ColAASID)},
		{name: "idShort", value: maskedExpressions[0]},
		{name: "administration", value: emptyJSONToNull(data.Col(common.ColAdministrativeInfoPayload))},
		{name: "displayName", value: data.Col(common.ColDisplayNamePayload)},
		{name: "description", value: data.Col(common.ColDescriptionPayload)},
		{name: "extensions", value: data.Col(common.ColExtensionsPayload)},
		{name: "semanticId", value: goqu.Case().When(goqu.L("? IS NOT NULL", maskedExpressions[1]), semanticReference).Else(nil)},
		{name: "supplementalSemanticIds", value: supplementalReferences},
		{name: "endpoints", value: endpoints},
	})
	if err != nil {
		return nil, err
	}
	return dialect.From(inner.As(dataAlias)).
		Select(jsonArrayAggregate(submodelJSON, data.Col(common.ColPosition), data.Col(common.ColDescriptorID))), nil
}

func buildUnfilteredSpecificAssetIDArraySubquery(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	descriptorID exp.Expression,
) (*goqu.SelectDataset, error) {
	specificAssetID := goqu.T(common.TblSpecificAssetID).As(common.AliasSpecificAssetID)
	payload := goqu.T(common.TblSpecificAssetIDPayload).As("specific_asset_payload")
	externalReference, err := buildReferenceObjectSubquery(
		ctx,
		dialect,
		specificAssetExternalReferenceTable,
		common.AliasExternalSubjectReference,
		common.AliasExternalSubjectReferenceKey,
		common.ColID,
		specificAssetID.Col(common.ColID),
		"",
		nil,
	)
	if err != nil {
		return nil, err
	}
	supplementalReferences, err := buildReferenceArraySubquery(
		ctx,
		dialect,
		common.TblSpecificAssetIDSuppSemantic,
		"specific_asset_supplemental_semantic_id_reference",
		"specific_asset_supplemental_semantic_id_reference_key",
		common.ColSpecificAssetIDID,
		specificAssetID.Col(common.ColID),
		"",
		"",
		nil,
	)
	if err != nil {
		return nil, err
	}
	specificAssetJSON, err := buildMaskedJSONObject(ctx, nil, []descriptorJSONField{
		{name: "name", value: specificAssetID.Col(common.ColName)},
		{name: "value", value: specificAssetID.Col(common.ColValue)},
		{name: "semanticId", value: emptyJSONToNull(payload.Col("semantic_id_payload"))},
		{name: "externalSubjectId", value: externalReference},
		{name: "supplementalSemanticIds", value: emptyJSONToNull(supplementalReferences)},
	})
	if err != nil {
		return nil, err
	}
	return dialect.From(specificAssetID).
		LeftJoin(payload, goqu.On(payload.Col(common.ColSpecificAssetID).Eq(specificAssetID.Col(common.ColID)))).
		Select(jsonArrayAggregate(specificAssetJSON, specificAssetID.Col(common.ColPosition))).
		Where(
			specificAssetID.Col(common.ColDescriptorID).Eq(descriptorID),
			specificAssetID.Col(common.ColName).Neq(globalAssetIDSpecificAssetIDName),
		), nil
}

func buildUnfilteredSubmodelDescriptorArraySubquery(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	aasDescriptorID exp.Expression,
) (*goqu.SelectDataset, error) {
	submodel := goqu.T(common.TblSubmodelDescriptor).As(common.AliasSubmodelDescriptor)
	payload := goqu.T(common.TblDescriptorPayload).As("submodel_descriptor_payload")
	semanticReference, err := buildReferenceObjectSubquery(
		ctx,
		dialect,
		submodelDescriptorSemanticReferenceTable,
		common.AliasSubmodelDescriptorSemanticIDReference,
		common.AliasSubmodelDescriptorSemanticIDReferenceKey,
		common.ColID,
		submodel.Col(common.ColDescriptorID),
		"",
		nil,
	)
	if err != nil {
		return nil, err
	}
	supplementalReferences, err := buildReferenceArraySubquery(
		ctx,
		dialect,
		common.TblSubmodelDescriptorSuppSemantic,
		"aasdesc_submodel_descriptor_supplemental_semantic_id_reference",
		"aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key",
		common.ColDescriptorID,
		submodel.Col(common.ColDescriptorID),
		"",
		"",
		nil,
	)
	if err != nil {
		return nil, err
	}
	endpoints, err := buildEndpointArraySubquery(
		ctx,
		dialect,
		common.TblAASDescriptorEndpoint,
		common.AliasSubmodelDescriptorEndpoint,
		submodel.Col(common.ColDescriptorID),
		"",
		nil,
	)
	if err != nil {
		return nil, err
	}
	submodelJSON, err := buildMaskedJSONObject(ctx, nil, []descriptorJSONField{
		{name: "id", value: submodel.Col(common.ColAASID)},
		{name: "idShort", value: submodel.Col(common.ColIDShort)},
		{name: "administration", value: emptyJSONToNull(payload.Col(common.ColAdministrativeInfoPayload))},
		{name: "displayName", value: payload.Col(common.ColDisplayNamePayload)},
		{name: "description", value: payload.Col(common.ColDescriptionPayload)},
		{name: "extensions", value: payload.Col(common.ColExtensionsPayload)},
		{name: "semanticId", value: semanticReference},
		{name: "supplementalSemanticIds", value: supplementalReferences},
		{name: "endpoints", value: endpoints},
	})
	if err != nil {
		return nil, err
	}
	return dialect.From(submodel).
		InnerJoin(payload, goqu.On(payload.Col(common.ColDescriptorID).Eq(submodel.Col(common.ColDescriptorID)))).
		Select(jsonArrayAggregate(submodelJSON, submodel.Col(common.ColPosition), submodel.Col(common.ColDescriptorID))).
		Where(submodel.Col(common.ColAASDescriptorID).Eq(aasDescriptorID)), nil
}

func buildReferenceObjectSubquery(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	table string,
	referenceAlias string,
	keyAlias string,
	ownerColumn string,
	ownerID exp.Expression,
	keyFragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	referenceTable := goqu.T(table).As(referenceAlias)
	reference := goqu.T(referenceAlias)
	const payloadAlias = "single_reference_payload"
	payloadTable := goqu.T(table + "_payload").As(payloadAlias)
	payload := goqu.T(payloadAlias)
	referenceJSON, err := buildReferenceJSON(ctx, dialect, table, keyAlias, reference, payload, keyFragment, collector)
	if err != nil {
		return nil, err
	}
	ds := dialect.From(referenceTable).
		LeftJoin(payloadTable, goqu.On(payload.Col(common.ColReferenceID).Eq(reference.Col(common.ColID)))).
		Where(reference.Col(ownerColumn).Eq(ownerID))
	if hasAASDescriptorFragmentFilter(ctx, keyFragment) {
		key := goqu.T(table + "_key").As(keyAlias)
		ds = ds.InnerJoin(key, goqu.On(key.Col(common.ColReferenceID).Eq(reference.Col(common.ColID))))
		ds, err = auth.AddCorrelatedFilterQueryFromContext(ctx, ds, keyFragment, collector)
		if err != nil {
			return nil, err
		}
	}
	return ds.Select(referenceJSON).Limit(1), nil
}

func buildReferenceArraySubquery(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	table string,
	referenceAlias string,
	keyAlias string,
	ownerColumn string,
	ownerID exp.Expression,
	referenceFragment grammar.FragmentStringPattern,
	keyFragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (*goqu.SelectDataset, error) {
	referenceTable := goqu.T(table).As(referenceAlias)
	reference := goqu.T(referenceAlias)
	const payloadAlias = "reference_array_payload"
	payloadTable := goqu.T(table + "_payload").As(payloadAlias)
	payload := goqu.T(payloadAlias)
	filtered := hasAASDescriptorFragmentFilter(ctx, referenceFragment) || hasAASDescriptorFragmentFilter(ctx, keyFragment)
	if !filtered {
		referenceJSON, err := buildReferenceJSON(
			ctx,
			dialect,
			table,
			keyAlias,
			reference,
			payload,
			keyFragment,
			collector,
		)
		if err != nil {
			return nil, err
		}
		return dialect.From(referenceTable).
			LeftJoin(payloadTable, goqu.On(payload.Col(common.ColReferenceID).Eq(reference.Col(common.ColID)))).
			Select(jsonArrayAggregate(referenceJSON, reference.Col(common.ColPosition), reference.Col(common.ColID))).
			Where(reference.Col(ownerColumn).Eq(ownerID)), nil
	}
	ds := dialect.From(referenceTable).
		LeftJoin(payloadTable, goqu.On(payload.Col(common.ColReferenceID).Eq(reference.Col(common.ColID))))
	if hasAASDescriptorFragmentFilter(ctx, referenceFragment) || hasAASDescriptorFragmentFilter(ctx, keyFragment) {
		key := goqu.T(table + "_key").As(keyAlias)
		ds = ds.InnerJoin(key, goqu.On(key.Col(common.ColReferenceID).Eq(reference.Col(common.ColID))))
	}
	ds = ds.Where(reference.Col(ownerColumn).Eq(ownerID))
	ds, err := auth.AddCorrelatedFilterQueryFromContext(ctx, ds, referenceFragment, collector)
	if err != nil {
		return nil, err
	}
	ds, err = auth.AddCorrelatedFilterQueryFromContext(ctx, ds, keyFragment, collector)
	if err != nil {
		return nil, err
	}
	ds = ds.Select(
		reference.Col(common.ColID).As(common.ColID),
		reference.Col(common.ColType).As(common.ColType),
		reference.Col(common.ColPosition).As(common.ColPosition),
		payload.Col("parent_reference_payload").As("parent_reference_payload"),
	).Distinct()
	filteredReference := goqu.T(referenceAlias)
	referenceJSON, err := buildReferenceJSON(
		ctx,
		dialect,
		table,
		keyAlias,
		filteredReference,
		filteredReference,
		keyFragment,
		collector,
	)
	if err != nil {
		return nil, err
	}
	return dialect.From(ds.As(referenceAlias)).
		Select(jsonArrayAggregate(
			referenceJSON,
			filteredReference.Col(common.ColPosition),
			filteredReference.Col(common.ColID),
		)), nil
}

func buildReferenceJSON(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	table string,
	keyAlias string,
	reference exp.IdentifierExpression,
	payload exp.IdentifierExpression,
	keyFragment grammar.FragmentStringPattern,
	collector *grammar.ResolvedFieldPathCollector,
) (exp.Expression, error) {
	key := goqu.T(table + "_key").As(keyAlias)
	keys := dialect.From(key).
		Select(jsonArrayAggregate(
			goqu.Func("jsonb_build_object",
				common.PostgreSQLTextLiteral("type"), buildKeyTypeStringExpression(key.Col(common.ColType)),
				common.PostgreSQLTextLiteral("value"), key.Col(common.ColValue),
			),
			key.Col(common.ColPosition),
			key.Col(common.ColID),
		)).
		Where(key.Col(common.ColReferenceID).Eq(reference.Col(common.ColID)))
	keys, err := auth.AddCorrelatedFilterQueryFromContext(ctx, keys, keyFragment, collector)
	if err != nil {
		return nil, err
	}
	return goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object",
		common.PostgreSQLTextLiteral("type"), buildReferenceTypeStringExpression(reference.Col(common.ColType)),
		common.PostgreSQLTextLiteral("keys"), keys,
		common.PostgreSQLTextLiteral("referredSemanticId"), emptyJSONToNull(payload.Col("parent_reference_payload")),
	)), nil
}

type descriptorJSONField struct {
	name     string
	value    exp.Expression
	fragment grammar.FragmentStringPattern
}

func buildMaskedJSONObject(
	ctx context.Context,
	collector *grammar.ResolvedFieldPathCollector,
	fields []descriptorJSONField,
) (exp.Expression, error) {
	columns := make([]auth.FilterColumnSpec, 0, len(fields))
	for _, field := range fields {
		if field.fragment == "" {
			columns = append(columns, auth.Column(field.value))
			continue
		}
		columns = append(columns, auth.MaskedColumn(field.value, field.fragment))
	}
	values, err := auth.GetColumnSelectStatement(ctx, columns, collector)
	if err != nil {
		return nil, err
	}
	jsonFields := make([]interface{}, 0, len(fields)*2)
	for index, field := range fields {
		jsonFields = append(jsonFields, common.PostgreSQLTextLiteral(field.name), values[index])
	}
	return goqu.Func("jsonb_strip_nulls", goqu.Func("jsonb_build_object", jsonFields...)), nil
}

func newAASDescriptorJSONCollector(
	rootAlias string,
	rootColumn string,
	inlineAliases ...string,
) (*grammar.ResolvedFieldPathCollector, error) {
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		return nil, err
	}
	collector.SetRootJoinKey(rootAlias, rootColumn)
	collector.AllowInlineAliases(inlineAliases...)
	return collector, nil
}

func hasAASDescriptorFragmentFilter(ctx context.Context, fragment grammar.FragmentStringPattern) bool {
	queryFilter := auth.GetQueryFilter(ctx)
	return queryFilter != nil && len(queryFilter.FilterExpressionEntriesFor(fragment)) > 0
}

func hasAASDescriptorFilters(ctx context.Context) bool {
	queryFilter := auth.GetQueryFilter(ctx)
	return queryFilter != nil && len(queryFilter.Filters) > 0
}

func buildOptionalDescriptorMaskRuntime(
	ctx context.Context,
	collector *grammar.ResolvedFieldPathCollector,
	columns []auth.MaskedInnerColumnSpec,
) (*auth.SharedFragmentMaskRuntime, error) {
	for _, column := range columns {
		if hasAASDescriptorFragmentFilter(ctx, column.Fragment) {
			return auth.BuildSharedFragmentMaskRuntime(ctx, collector, columns)
		}
	}
	return nil, nil
}

func descriptorMaskedExpressions(
	runtime *auth.SharedFragmentMaskRuntime,
	dataAlias string,
	columns []auth.MaskedInnerColumnSpec,
) ([]exp.Expression, error) {
	if runtime != nil {
		return runtime.MaskedInnerAliasExprs(dataAlias, columns)
	}
	expressions := make([]exp.Expression, 0, len(columns))
	for _, column := range columns {
		expressions = append(expressions, goqu.I(dataAlias+"."+column.RawAlias))
	}
	return expressions, nil
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
		result = result.When(int(assetKind), common.PostgreSQLTextLiteral(stringification.MustAssetKindToString(assetKind)))
	}
	return result.Else(goqu.L("NULL"))
}

func buildReferenceTypeStringExpression(value exp.Expression) exp.CaseExpression {
	result := goqu.Case().Value(value)
	for _, referenceType := range types.LiteralsOfReferenceTypes {
		result = result.When(int(referenceType), common.PostgreSQLTextLiteral(stringification.MustReferenceTypesToString(referenceType)))
	}
	return result.Else(goqu.L("NULL"))
}

func buildKeyTypeStringExpression(value exp.Expression) exp.CaseExpression {
	result := goqu.Case().Value(value)
	for _, keyType := range types.LiteralsOfKeyTypes {
		result = result.When(int(keyType), common.PostgreSQLTextLiteral(stringification.MustKeyTypesToString(keyType)))
	}
	return result.Else(goqu.L("NULL"))
}
