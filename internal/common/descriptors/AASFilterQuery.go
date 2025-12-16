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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func getJoinTables(d goqu.DialectWrapper) *goqu.SelectDataset {

	joinTables := d.From(goqu.T(tblDescriptor).As("descriptor")).
		LeftJoin(goqu.T(tblAASDescriptor).As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id")))).
		LeftJoin(goqu.T(tblSpecificAssetID).As("specific_asset_id"),
			goqu.On(goqu.I("specific_asset_id.descriptor_id").Eq(goqu.I("descriptor.id")))).
		LeftJoin(goqu.T(tblReference).As("external_subject_reference"),
			goqu.On(goqu.I("external_subject_reference.id").Eq(goqu.I("specific_asset_id.external_subject_ref")))).
		LeftJoin(goqu.T(tblReferenceKey).As("external_subject_reference_key"),
			goqu.On(goqu.I("external_subject_reference_key.reference_id").Eq(goqu.I("external_subject_reference.id")))).
		LeftJoin(goqu.T(tblAASDescriptorEndpoint).As("aas_descriptor_endpoint"),
			goqu.On(goqu.I("aas_descriptor_endpoint.descriptor_id").Eq(goqu.I("descriptor.id")))).
		LeftJoin(goqu.T(tblSubmodelDescriptor).As("submodel_descriptor"),
			goqu.On(goqu.I("submodel_descriptor.aas_descriptor_id").Eq(goqu.I("aas_descriptor.descriptor_id")))).
		LeftJoin(goqu.T(tblAASDescriptorEndpoint).As("submodel_descriptor_endpoint"),
			goqu.On(goqu.I("submodel_descriptor_endpoint.descriptor_id").Eq(goqu.I("submodel_descriptor.descriptor_id")))).
		LeftJoin(goqu.T(tblReference).As("aasdesc_submodel_descriptor_semantic_id_reference"),
			goqu.On(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id").Eq(goqu.I("submodel_descriptor.semantic_id")))).
		LeftJoin(goqu.T(tblReferenceKey).As("aasdesc_submodel_descriptor_semantic_id_reference_key"),
			goqu.On(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference_key.reference_id").
				Eq(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id"))))
	return joinTables
}
func getFilterQueryFromContext(ctx context.Context, d goqu.DialectWrapper, ds *goqu.SelectDataset, tableCol exp.AliasedExpression) (*goqu.SelectDataset, error) {

	p := auth.GetQueryFilter(ctx)
	if p != nil && p.Formula != nil {

		wc, err := p.Formula.EvaluateToExpression()
		if err != nil {
			return nil, err
		}
		existsDataset :=
			getJoinTables(d).
				Where(
					goqu.I("descriptor.id").Eq(tableCol.Col(colDescriptorID)),
					wc,
				)

		ds = ds.Where(
			goqu.L("EXISTS (?)", existsDataset),
		)
	}
	return ds, nil
}

func addSpecificAssetFilter(
	ctx context.Context,
	ds *goqu.SelectDataset,
	identifable string,

) (*goqu.SelectDataset, error) {
	p := auth.GetQueryFilter(ctx)
	if p == nil {
		return ds, nil
	}

	ok, filter := p.FilterExpressionFor(identifable, false)

	if !ok {
		return ds, nil
	}

	wc, err := filter.EvaluateToExpression()
	if err != nil {
		return nil, err
	}

	ds = ds.Where(wc)

	return ds, nil
}

// ExpressionIdentifiableMapper links a selectable expression with an optional
// identifier name used for ABAC fragment filtering; canBeFiltered controls
// whether the expression participates in filter-based projections.
type ExpressionIdentifiableMapper struct {
	iexp          exp.Expression
	canBeFiltered bool
	identifable   *string
}
type expressionIdentifiableMapperIntermediate struct {
	iexp          exp.Expression
	canBeFiltered bool
	identifable   *string
	mapper        *grammar.LogicalExpression
}

func extractExpressions(mappers []ExpressionIdentifiableMapper) []exp.Expression {
	expressions := make([]exp.Expression, 0, len(mappers))

	for _, m := range mappers {
		expressions = append(expressions, m.iexp)
	}

	return expressions
}

func getColumnSelectStatement(ctx context.Context, expressionMappers []ExpressionIdentifiableMapper) ([]exp.Expression, error) {

	defaultReturn := extractExpressions(expressionMappers)
	p := auth.GetQueryFilter(ctx)
	if p == nil {
		return defaultReturn, nil
	}

	var ok = false
	expressionMappersIntermediate := []expressionIdentifiableMapperIntermediate{}
	for _, expMapper := range expressionMappers {
		mapper := expressionIdentifiableMapperIntermediate{
			iexp:          expMapper.iexp,
			canBeFiltered: expMapper.canBeFiltered,
			identifable:   expMapper.identifable,
		}
		if expMapper.identifable != nil {

			isOk, filter := p.ExistsExpressionFor(*expMapper.identifable, true)
			if isOk {
				ok = true
			}

			mapper.mapper = &filter
		}
		expressionMappersIntermediate = append(expressionMappersIntermediate, mapper)
	}
	if !ok {
		return defaultReturn, nil
	}
	result := []exp.Expression{}
	for i, expMapper := range expressionMappersIntermediate {
		if !expMapper.canBeFiltered {
			result = append(result, expMapper.iexp)
			continue
		}
		conditions := []grammar.LogicalExpression{}
		for j, expMapper2 := range expressionMappersIntermediate {
			if i == j {
				continue
			}
			if expMapper2.identifable != nil {
				conditions = append(conditions, *expMapper2.mapper)
			}
		}
		//TODO: use simplify function on that (need to be refactored out of security first)
		finalFilter := grammar.LogicalExpression{And: conditions}
		wc, err := finalFilter.EvaluateToExpression()
		if err != nil {
			return nil, err
		}
		result = append(result, goqu.MAX(caseWhenColumn(wc, expMapper.iexp)))

	}

	return result, nil

}

func caseWhenColumn(wc exp.Expression, iexp exp.Expression) exp.CaseExpression {
	return goqu.Case().
		When(
			wc,
			iexp,
		).
		Else(nil)
}
