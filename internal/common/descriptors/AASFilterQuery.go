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

// getJoinTables relevant for only aas descriptors
func getJoinTables(d goqu.DialectWrapper) *goqu.SelectDataset {
	specificAssetID := goqu.T(tblSpecificAssetID).As(aliasSpecificAssetID)
	externalSubjectReference := goqu.T(tblReference).As(aliasExternalSubjectReference)
	externalSubjectReferenceKey := goqu.T(tblReferenceKey).As(aliasExternalSubjectReferenceKey)
	aasDescriptorEndpoint := goqu.T(tblAASDescriptorEndpoint).As(aliasAASDescriptorEndpoint)
	submodelDescriptor := goqu.T(tblSubmodelDescriptor).As(aliasSubmodelDescriptor)
	submodelDescriptorEndpoint := goqu.T(tblAASDescriptorEndpoint).As(aliasSubmodelDescriptorEndpoint)
	submodelDescriptorSemanticIDReference := goqu.T(tblReference).As(aliasSubmodelDescriptorSemanticIDReference)
	submodelDescriptorSemanticIDReferenceKey := goqu.T(tblReferenceKey).As(aliasSubmodelDescriptorSemanticIDReferenceKey)

	joinTables := d.From(tDescriptor).
		InnerJoin(
			tAASDescriptor,
			goqu.On(tAASDescriptor.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		).
		LeftJoin(
			specificAssetID,
			goqu.On(specificAssetID.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		).
		LeftJoin(
			externalSubjectReference,
			goqu.On(externalSubjectReference.Col(colID).Eq(specificAssetID.Col(colExternalSubjectRef))),
		).
		LeftJoin(
			externalSubjectReferenceKey,
			goqu.On(externalSubjectReferenceKey.Col(colReferenceID).Eq(externalSubjectReference.Col(colID))),
		).
		LeftJoin(
			aasDescriptorEndpoint,
			goqu.On(aasDescriptorEndpoint.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		).
		LeftJoin(
			submodelDescriptor,
			goqu.On(submodelDescriptor.Col(colAASDescriptorID).Eq(tAASDescriptor.Col(colDescriptorID))),
		).
		LeftJoin(
			submodelDescriptorEndpoint,
			goqu.On(submodelDescriptorEndpoint.Col(colDescriptorID).Eq(submodelDescriptor.Col(colDescriptorID))),
		).
		LeftJoin(
			submodelDescriptorSemanticIDReference,
			goqu.On(submodelDescriptorSemanticIDReference.Col(colID).Eq(submodelDescriptor.Col(colSemanticID))),
		).
		LeftJoin(
			submodelDescriptorSemanticIDReferenceKey,
			goqu.On(submodelDescriptorSemanticIDReferenceKey.Col(colReferenceID).Eq(submodelDescriptorSemanticIDReference.Col(colID))),
		)
	return joinTables
}

// getFilterQueryFromContext relevant for only aas descriptors
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
					tDescriptor.Col(colID).Eq(tableCol.Col(colDescriptorID)),
					wc,
				)

		ds = ds.Where(
			goqu.L("EXISTS (?)", existsDataset),
		)
	}
	return ds, nil
}
