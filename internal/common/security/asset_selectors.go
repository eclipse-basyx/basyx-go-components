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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"context"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// WithAuthorizedAssetIDSelectors requires every pair within the same resource's visible asset identifiers.
func WithAuthorizedAssetIDSelectors(ctx context.Context, resource SemanticResourceKind, assets []types.ISpecificAssetID) (context.Context, error) {
	prefix := "$aas#assetInformation."
	if resource == SemanticResourceAASDesc {
		prefix = "$aasdesc#"
	}
	conditions := make([]grammar.LogicalExpression, 0, len(assets))
	for _, asset := range assets {
		if asset == nil {
			continue
		}
		value := grammar.StandardString(asset.Value())
		if asset.Name() == common.GlobalAssetIDAssetLinkName {
			field := grammar.ModelStringPattern(prefix + "globalAssetId")
			conditions = append(conditions, grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &value}}})
			continue
		}
		nameField := grammar.ModelStringPattern(prefix + "specificAssetIds[].name")
		valueField := grammar.ModelStringPattern(prefix + "specificAssetIds[].value")
		name := grammar.StandardString(asset.Name())
		conditions = append(conditions, grammar.LogicalExpression{Match: []grammar.MatchExpression{
			{Eq: grammar.ComparisonItems{{Field: &nameField}, {StrVal: &name}}},
			{Eq: grammar.ComparisonItems{{Field: &valueField}, {StrVal: &value}}},
		}})
	}
	if len(conditions) == 0 {
		return ctx, nil
	}
	condition := grammar.LogicalExpression{And: conditions}
	return WithAuthorizedQuery(ctx, resource, grammar.Query{Condition: &condition})
}
