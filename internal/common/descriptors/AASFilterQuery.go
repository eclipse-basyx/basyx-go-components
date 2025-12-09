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
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func getFilterQueryFromContext(ctx context.Context, d goqu.DialectWrapper, ds *goqu.SelectDataset, tableCol exp.AliasedExpression) (*goqu.SelectDataset, error) {

	p := auth.GetQueryFilter(ctx)
	if p != nil && p.Formula != nil {

		wc, err := p.Formula.EvaluateToExpression()
		if err != nil {
			return nil, err
		}
		existsDataset := d.
			From(goqu.T(tblDescriptor).As("descriptor")).
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
					Eq(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id")))).
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

// addSpecificAssetFilter applies ABAC filtering on SpecificAssetID rows by correlating the policy
// EXISTS query to the current SAI row (sai alias) while preserving descriptor-level joins.
func addSpecificAssetFilter(ctx context.Context, d goqu.DialectWrapper, ds *goqu.SelectDataset, sai exp.AliasedExpression) (*goqu.SelectDataset, error) {
	p := auth.GetQueryFilter(ctx)
	if p == nil {
		return ds, nil
	}
	filter, ok := p.Filters["$aasdesc#specificAssetIds[]"]
	if !ok {
		return ds, nil
	}

	wc, err := filter.EvaluateToExpression()
	if err != nil {
		return nil, err
	}

	existsDataset := d.
		From(goqu.T(tblDescriptor).As("descriptor")).
		LeftJoin(goqu.T(tblAASDescriptor).As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id")))).
		LeftJoin(goqu.T(tblSpecificAssetID).As("specific_asset_id_a"),
			goqu.On(
				goqu.I("specific_asset_id_a.descriptor_id").Eq(goqu.I("descriptor.id")),
				goqu.I("specific_asset_id_a.id").Eq(sai.Col(colID)),
			)).
		LeftJoin(goqu.T(tblReference).As("external_subject_reference"),
			goqu.On(goqu.I("external_subject_reference.id").Eq(goqu.I("specific_asset_id_a.external_subject_ref")))).
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
				Eq(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id")))).
		Where(
			goqu.I("descriptor.id").Eq(sai.Col(colDescriptorID)),
			wc,
		)

	ds = ds.Where(goqu.L("EXISTS (?)", existsDataset))

	return ds, nil

}

func getColumnSelectStatement(ctx context.Context, sai exp.AliasedExpression, colName string) (exp.AliasedExpression, error) {

	p := auth.GetQueryFilter(ctx)
	if p == nil {
		return sai.Col(colName).As(colName), nil
	}
	filter, ok := p.Filters["$aasdesc#specificAssetIds[].name"]

	if !ok {
		return sai.Col(colName).As(colName), nil
	}

	wc, err := filter.EvaluateToExpression()
	if err != nil {
		return nil, err
	}
	return goqu.Case().
		When(
			wc,
			sai.Col(colName),
		).
		Else(nil).
		As(colName), nil

}
