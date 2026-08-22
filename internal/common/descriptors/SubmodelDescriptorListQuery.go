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
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

type submodelDescriptorListScope struct {
	collectorRoot   grammar.CollectorRoot
	fragmentPrefix  string
	aasDescriptorID *int64
}

func (scope submodelDescriptorListScope) fragment(suffix string) grammar.FragmentStringPattern {
	separator := "#"
	if scope.collectorRoot == grammar.CollectorRootAASDesc {
		separator = "."
	}
	return grammar.FragmentStringPattern(scope.fragmentPrefix + separator + suffix)
}

func listSubmodelDescriptorsSingleStatement(
	ctx context.Context,
	db DBQueryer,
	limit int32,
	cursor string,
	createdFrom time.Time,
	updatedFrom time.Time,
) ([]model.SubmodelDescriptor, string, error) {
	return listSubmodelDescriptorsFromPageQuery(
		ctx,
		db,
		limit,
		cursor,
		createdFrom,
		updatedFrom,
		submodelDescriptorListScope{
			collectorRoot:  grammar.CollectorRootSMDesc,
			fragmentPrefix: "$smdesc",
		},
	)
}

func listSubmodelDescriptorsForAASSingleStatement(
	ctx context.Context,
	db DBQueryer,
	aasDescriptorID int64,
	limit int32,
	cursor string,
) ([]model.SubmodelDescriptor, string, error) {
	return listSubmodelDescriptorsFromPageQuery(
		ctx,
		db,
		limit,
		cursor,
		time.Time{},
		time.Time{},
		submodelDescriptorListScope{
			collectorRoot:   grammar.CollectorRootAASDesc,
			fragmentPrefix:  "$aasdesc#submodelDescriptors[]",
			aasDescriptorID: &aasDescriptorID,
		},
	)
}

func listSubmodelDescriptorsFromPageQuery(
	ctx context.Context,
	db DBQueryer,
	limit int32,
	cursor string,
	createdFrom time.Time,
	updatedFrom time.Time,
	scope submodelDescriptorListScope,
) ([]model.SubmodelDescriptor, string, error) {
	if limit <= 0 {
		limit = 100
	}
	peekLimit := limit
	if limit < 1<<31-1 {
		peekLimit++
	}
	dataset, err := buildSubmodelDescriptorListQuery(
		ctx,
		peekLimit,
		cursor,
		createdFrom,
		updatedFrom,
		scope,
	)
	if err != nil {
		return nil, "", err
	}
	query, args, err := dataset.Prepared(true).ToSQL()
	if err != nil {
		return nil, "", common.NewInternalServerError("SMDESC-LIST-BUILDQUERY " + err.Error())
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("SMDESC-LIST-QUERYDB " + err.Error())
	}
	defer func() { _ = rows.Close() }()

	descriptors := make([]model.SubmodelDescriptor, 0, peekLimit)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, "", common.NewInternalServerError("SMDESC-LIST-SCANROW " + err.Error())
		}
		descriptor, err := model.DecodeStoredSubmodelDescriptor(payload)
		if err != nil {
			return nil, "", common.NewInternalServerError("SMDESC-LIST-DECODE " + err.Error())
		}
		descriptors = append(descriptors, descriptor)
	}
	if err := rows.Err(); err != nil {
		return nil, "", common.NewInternalServerError("SMDESC-LIST-ITERATEROWS " + err.Error())
	}

	page, nextCursor := applyCursorLimit(descriptors, limit, func(descriptor model.SubmodelDescriptor) string {
		return descriptor.Id
	})
	return page, nextCursor, nil
}

func buildSubmodelDescriptorListQuery(
	ctx context.Context,
	peekLimit int32,
	cursor string,
	createdFrom time.Time,
	updatedFrom time.Time,
	scope submodelDescriptorListScope,
) (*goqu.SelectDataset, error) {
	dialect := goqu.Dialect(common.Dialect)
	authorized, maskRuntime, maskedColumns, err := buildAuthorizedSubmodelDescriptorRows(
		ctx,
		dialect,
		createdFrom,
		updatedFrom,
		scope,
	)
	if err != nil {
		return nil, err
	}

	const (
		authorizedAlias = "authorized_submodel_descriptors"
		pageAlias       = "submodel_descriptor_page"
	)
	authorizedTable := goqu.T(authorizedAlias)
	page := dialect.From(authorizedTable).
		Select(
			authorizedTable.Col(common.ColDescriptorID),
			authorizedTable.Col(common.ColAASDescriptorID),
			authorizedTable.Col(common.ColAASID),
			authorizedTable.Col(common.ColIDShort),
			authorizedTable.Col(common.ColAdministrativeInfoPayload),
			authorizedTable.Col(common.ColDisplayNamePayload),
			authorizedTable.Col(common.ColDescriptionPayload),
			authorizedTable.Col(common.ColExtensionsPayload),
			authorizedTable.Col("semantic_reference_id"),
			authorizedTable.Col(common.ColPosition),
		).
		Order(
			authorizedTable.Col(common.ColAASID).Asc(),
			authorizedTable.Col(common.ColDescriptorID).Asc(),
		)
	//nolint:gosec // peekLimit is normalized to a positive int32 value
	page = page.Limit(uint(peekLimit))
	if maskRuntime != nil {
		for _, column := range maskedColumns {
			page = page.SelectAppend(authorizedTable.Col(column.FlagAlias))
		}
	}
	if cursor != "" {
		cursorExists := dialect.From(authorizedTable).
			Select(goqu.L("1")).
			Where(authorizedTable.Col(common.ColAASID).Eq(cursor))
		page = page.Where(
			goqu.Func("EXISTS", cursorExists),
			authorizedTable.Col(common.ColAASID).Gte(cursor),
		)
	}

	pageTable := goqu.T(pageAlias)
	maskedExpressions, err := descriptorMaskedExpressions(maskRuntime, pageAlias, maskedColumns)
	if err != nil {
		return nil, err
	}
	descriptorJSON, err := buildSubmodelDescriptorJSON(ctx, dialect, pageAlias, pageTable, maskedExpressions, scope)
	if err != nil {
		return nil, err
	}
	return dialect.From(page.As(pageAlias)).
		With(authorizedAlias, authorized).
		Select(descriptorJSON).
		Order(
			pageTable.Col(common.ColAASID).Asc(),
			pageTable.Col(common.ColDescriptorID).Asc(),
		), nil
}

func buildAuthorizedSubmodelDescriptorRows(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	createdFrom time.Time,
	updatedFrom time.Time,
	scope submodelDescriptorListScope,
) (*goqu.SelectDataset, *auth.SharedFragmentMaskRuntime, []auth.MaskedInnerColumnSpec, error) {
	submodel := goqu.T(common.TblSubmodelDescriptor).As(common.AliasSubmodelDescriptor)
	payload := goqu.T(common.TblDescriptorPayload).As("submodel_descriptor_payload")
	semanticReference := goqu.T(submodelDescriptorSemanticReferenceTable).As(common.AliasSubmodelDescriptorSemanticIDReference)
	collector, err := newSubmodelDescriptorListCollector(scope)
	if err != nil {
		return nil, nil, nil, err
	}
	maskedColumns := []auth.MaskedInnerColumnSpec{
		{Fragment: scope.fragment("idShort"), FlagAlias: "flag_smdesc_idshort", RawAlias: common.ColIDShort},
		{Fragment: scope.fragment("semanticId"), FlagAlias: "flag_smdesc_semanticid", RawAlias: "semantic_reference_id"},
	}
	maskRuntime, err := buildOptionalDescriptorMaskRuntime(ctx, collector, maskedColumns)
	if err != nil {
		return nil, nil, nil, err
	}

	columns := []interface{}{
		submodel.Col(common.ColDescriptorID).As(common.ColDescriptorID),
		submodel.Col(common.ColAASDescriptorID).As(common.ColAASDescriptorID),
		submodel.Col(common.ColAASID).As(common.ColAASID),
		submodel.Col(common.ColIDShort).As(common.ColIDShort),
		payload.Col(common.ColAdministrativeInfoPayload).As(common.ColAdministrativeInfoPayload),
		payload.Col(common.ColDisplayNamePayload).As(common.ColDisplayNamePayload),
		payload.Col(common.ColDescriptionPayload).As(common.ColDescriptionPayload),
		payload.Col(common.ColExtensionsPayload).As(common.ColExtensionsPayload),
		semanticReference.Col(common.ColID).As("semantic_reference_id"),
		submodel.Col(common.ColPosition).As(common.ColPosition),
	}
	if maskRuntime != nil {
		columns = append(columns, maskRuntime.Projections()...)
	}
	query := dialect.From(submodel)
	if scope.aasDescriptorID != nil {
		query = dialect.From(common.TDescriptor).
			InnerJoin(
				common.TAASDescriptor,
				goqu.On(common.TAASDescriptor.Col(common.ColDescriptorID).Eq(common.TDescriptor.Col(common.ColID))),
			).
			InnerJoin(
				submodel,
				goqu.On(submodel.Col(common.ColAASDescriptorID).Eq(common.TAASDescriptor.Col(common.ColDescriptorID))),
			)
	}
	query = query.
		LeftJoin(payload, goqu.On(payload.Col(common.ColDescriptorID).Eq(submodel.Col(common.ColDescriptorID)))).
		LeftJoin(semanticReference, goqu.On(semanticReference.Col(common.ColID).Eq(submodel.Col(common.ColDescriptorID)))).
		Select(columns...)
	if scope.aasDescriptorID == nil {
		query = query.Where(submodel.Col(common.ColAASDescriptorID).IsNull())
	} else {
		query = query.Where(submodel.Col(common.ColAASDescriptorID).Eq(*scope.aasDescriptorID))
	}
	switch {
	case !createdFrom.IsZero() && !updatedFrom.IsZero():
		query = query.Where(goqu.Or(
			submodel.Col("administration_created_at").Gte(createdFrom.UTC()),
			submodel.Col("administration_updated_at").Gte(updatedFrom.UTC()),
		))
	case !createdFrom.IsZero():
		query = query.Where(submodel.Col("administration_created_at").Gte(createdFrom.UTC()))
	case !updatedFrom.IsZero():
		query = query.Where(submodel.Col("administration_updated_at").Gte(updatedFrom.UTC()))
	}

	if maskRuntime != nil && scope.aasDescriptorID == nil {
		query, err = maskRuntime.ApplyFilters(ctx, query, collector)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if scope.aasDescriptorID != nil {
		query, err = auth.AddCorrelatedFilterQueryFromContext(ctx, query, "$aasdesc#submodelDescriptors[]", collector)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	shouldEnforce, err := auth.ShouldEnforceFormula(ctx)
	if err != nil {
		return nil, nil, nil, common.NewInternalServerError("SMDESC-LIST-SHOULDENFORCE " + err.Error())
	}
	if shouldEnforce {
		query, err = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return query, maskRuntime, maskedColumns, nil
}

func newSubmodelDescriptorListCollector(scope submodelDescriptorListScope) (*grammar.ResolvedFieldPathCollector, error) {
	var collector *grammar.ResolvedFieldPathCollector
	var err error
	if scope.collectorRoot == grammar.CollectorRootSMDesc {
		collector, err = grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSMDesc)
	} else {
		collector, err = grammar.NewResolvedFieldPathCollectorForNestedSMDesc()
	}
	if err != nil {
		return nil, err
	}
	collector.AllowInlineAliases(
		"descriptor",
		"aas_descriptor",
		common.AliasSubmodelDescriptor,
		common.AliasSubmodelDescriptorSemanticIDReference,
	)
	return collector, nil
}

func buildSubmodelDescriptorJSON(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	sourceAlias string,
	source exp.IdentifierExpression,
	maskedExpressions []exp.Expression,
	scope submodelDescriptorListScope,
) (exp.Expression, error) {
	semanticCollector, err := newSubmodelDescriptorProjectionCollector(
		scope,
		sourceAlias,
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
		source.Col(common.ColDescriptorID),
		scope.fragment("semanticId.keys[]"),
		semanticCollector,
	)
	if err != nil {
		return nil, err
	}
	const (
		supplementalAlias    = "aasdesc_submodel_descriptor_supplemental_semantic_id_reference"
		supplementalKeyAlias = "aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key"
	)
	supplementalCollector, err := newSubmodelDescriptorProjectionCollector(
		scope,
		sourceAlias,
		supplementalAlias,
		supplementalKeyAlias,
	)
	if err != nil {
		return nil, err
	}
	supplementalReferences, err := buildReferenceArraySubquery(
		ctx,
		dialect,
		common.TblSubmodelDescriptorSuppSemantic,
		supplementalAlias,
		supplementalKeyAlias,
		common.ColDescriptorID,
		source.Col(common.ColDescriptorID),
		scope.fragment("supplementalSemanticIds[]"),
		scope.fragment("supplementalSemanticIds[].keys[]"),
		supplementalCollector,
	)
	if err != nil {
		return nil, err
	}
	endpointAlias := common.AliasSubmodelDescriptorEndpoint
	endpointCollector, err := newSubmodelDescriptorProjectionCollector(scope, sourceAlias, endpointAlias)
	if err != nil {
		return nil, err
	}
	endpoints, err := buildEndpointArraySubquery(
		ctx,
		dialect,
		common.TblAASDescriptorEndpoint,
		endpointAlias,
		source.Col(common.ColDescriptorID),
		scope.fragment("endpoints[]"),
		endpointCollector,
	)
	if err != nil {
		return nil, err
	}

	return buildMaskedJSONObject(ctx, nil, []descriptorJSONField{
		{name: "id", value: source.Col(common.ColAASID)},
		{name: "idShort", value: maskedExpressions[0]},
		{name: "administration", value: emptyJSONToNull(source.Col(common.ColAdministrativeInfoPayload))},
		{name: "displayName", value: source.Col(common.ColDisplayNamePayload)},
		{name: "description", value: source.Col(common.ColDescriptionPayload)},
		{name: "extensions", value: source.Col(common.ColExtensionsPayload)},
		{name: "semanticId", value: goqu.Case().When(goqu.L("? IS NOT NULL", maskedExpressions[1]), semanticReference).Else(nil)},
		{name: "supplementalSemanticIds", value: supplementalReferences},
		{name: "endpoints", value: endpoints},
	})
}

func newSubmodelDescriptorProjectionCollector(
	scope submodelDescriptorListScope,
	sourceAlias string,
	inlineAliases ...string,
) (*grammar.ResolvedFieldPathCollector, error) {
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(scope.collectorRoot)
	if err != nil {
		return nil, err
	}
	rootColumn := common.ColDescriptorID
	if scope.collectorRoot == grammar.CollectorRootAASDesc {
		rootColumn = common.ColAASDescriptorID
	}
	collector.SetRootJoinKey(sourceAlias, rootColumn)
	collector.AllowInlineAliases(inlineAliases...)
	return collector, nil
}
