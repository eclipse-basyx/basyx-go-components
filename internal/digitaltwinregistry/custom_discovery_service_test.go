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

package digitaltwinregistry

import (
	"context"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func TestBuildAssetLinkQuery_ReturnsEmptyWhenReadFormulaIsUnrestricted(t *testing.T) {
	t.Parallel()

	b := true
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{
		Condition: &grammar.LogicalExpression{Boolean: &b},
	})

	query := buildAssetLinkQuery(ctx, []model.AssetLink{{Name: "name", Value: "value"}})
	if query.Condition != nil {
		t.Fatalf("expected no additional condition when READ formula is unrestricted, got %#v", query.Condition)
	}
}

func TestBuildAssetLinkQuery_BuildsConditionWhenReadFormulaIsRestricted(t *testing.T) {
	t.Parallel()

	b := false
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{
		Condition: &grammar.LogicalExpression{Boolean: &b},
	})

	query := buildAssetLinkQuery(ctx, []model.AssetLink{{Name: "name", Value: "value"}})
	if query.Condition == nil {
		t.Fatalf("expected asset-link condition when READ formula is restricted")
	}
	if len(query.Condition.And) == 0 {
		t.Fatalf("expected AND conditions for asset-link query, got %#v", query.Condition)
	}
}

func TestBuildAssetLinkDescriptorQuery_FiltersWhenReadFormulaIsUnrestricted(t *testing.T) {
	t.Parallel()

	b := true
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{
		Condition: &grammar.LogicalExpression{Boolean: &b},
	})

	query := buildAssetLinkDescriptorQuery(ctx, []model.AssetLink{{Name: "name", Value: "value"}})
	if query.Condition == nil {
		t.Fatalf("expected asset-link condition for descriptor query")
	}
	if len(query.Condition.And) != 1 {
		t.Fatalf("expected one AND condition for descriptor asset-link query, got %#v", query.Condition)
	}
	if len(query.Condition.And[0].Match) != 2 {
		t.Fatalf("expected descriptor query to match name and value, got %#v", query.Condition.And[0].Match)
	}
}

func TestBuildAssetLinkDescriptorQuery_UsesSecurityAwareQueryWhenReadFormulaIsRestricted(t *testing.T) {
	t.Parallel()

	b := false
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{
		Condition: &grammar.LogicalExpression{Boolean: &b},
	})

	query := buildAssetLinkDescriptorQuery(ctx, []model.AssetLink{{Name: "name", Value: "value"}})
	if query.Condition == nil {
		t.Fatalf("expected security-aware asset-link condition")
	}
	if len(query.Condition.And) != 1 || len(query.Condition.And[0].Or) == 0 {
		t.Fatalf("expected restricted descriptor query to include authorization alternatives, got %#v", query.Condition)
	}
}

func TestDecodeRegistryAssetLinkQueryAssetIDs_DecodesAssetLink(t *testing.T) {
	t.Parallel()

	encoded := common.EncodeString(`{"name":"customerPartId","value":"part-1"}`)
	links, resp, err := decodeRegistryAssetLinkQueryAssetIDs([]string{encoded})
	if err != nil {
		t.Fatalf("decodeRegistryAssetLinkQueryAssetIDs returned error: %v", err)
	}
	if resp != nil {
		t.Fatalf("decodeRegistryAssetLinkQueryAssetIDs returned response: %#v", resp)
	}
	if len(links) != 1 || links[0].Name != "customerPartId" || links[0].Value != "part-1" {
		t.Fatalf("unexpected decoded links: %#v", links)
	}
}

func TestDecodeRegistryAssetLinkQueryAssetIDs_IgnoresBlankValues(t *testing.T) {
	t.Parallel()

	links, resp, err := decodeRegistryAssetLinkQueryAssetIDs([]string{"", "  "})
	if err != nil {
		t.Fatalf("decodeRegistryAssetLinkQueryAssetIDs returned error: %v", err)
	}
	if resp != nil {
		t.Fatalf("decodeRegistryAssetLinkQueryAssetIDs returned response: %#v", resp)
	}
	if len(links) != 0 {
		t.Fatalf("expected no decoded links, got %#v", links)
	}
}

func TestSplitGlobalAssetIDLinks(t *testing.T) {
	t.Parallel()

	globalAssetIDs, assetLinks := splitGlobalAssetIDLinks([]model.AssetLink{
		{Name: "customerPartId", Value: "part-1"},
		{Name: "globalAssetId", Value: "global-1"},
		{Name: "manufacturerPartId", Value: "part-2"},
	})

	if len(globalAssetIDs) != 1 || globalAssetIDs[0] != "global-1" {
		t.Fatalf("unexpected global asset IDs: %#v", globalAssetIDs)
	}
	if len(assetLinks) != 2 {
		t.Fatalf("unexpected asset links: %#v", assetLinks)
	}
	if assetLinks[0].Name != "customerPartId" || assetLinks[1].Name != "manufacturerPartId" {
		t.Fatalf("unexpected non-global links: %#v", assetLinks)
	}
}

func TestBuildGlobalAssetIDQuery(t *testing.T) {
	t.Parallel()

	query := buildGlobalAssetIDQuery([]string{"global-1", "global-2"})
	if query.Condition == nil {
		t.Fatalf("expected global asset ID query condition")
	}
	if len(query.Condition.And) != 2 {
		t.Fatalf("expected one AND condition per global asset ID, got %#v", query.Condition)
	}
	for _, condition := range query.Condition.And {
		if len(condition.Eq) != 2 || condition.Eq[0].Field == nil || *condition.Eq[0].Field != "$aasdesc#globalAssetId" {
			t.Fatalf("expected query to filter $aasdesc#globalAssetId, got %#v", condition)
		}
	}
}

func TestBuildBasicDiscoveryGlobalAssetIDQuery(t *testing.T) {
	t.Parallel()

	query := buildBasicDiscoveryGlobalAssetIDQuery([]string{"global-1"})
	if query.Condition == nil || len(query.Condition.And) != 1 {
		t.Fatalf("expected one basic-discovery global asset ID condition, got %#v", query.Condition)
	}
	condition := query.Condition.And[0]
	if len(condition.Eq) != 2 || condition.Eq[0].Field == nil || *condition.Eq[0].Field != "$aasdesc#globalAssetId" {
		t.Fatalf("expected query to filter $aasdesc#globalAssetId, got %#v", condition)
	}
}

func TestMergeGlobalAssetIDLookupVisibility_PreservesExistingFormula(t *testing.T) {
	t.Parallel()

	descriptorIDField := grammar.ModelStringPattern("$aasdesc#id")
	descriptorIDValue := grammar.StandardString("allowed-aas")
	existingFormula := grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			{Field: &descriptorIDField},
			{StrVal: &descriptorIDValue},
		},
	}
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{Condition: &existingFormula})

	ctx = mergeGlobalAssetIDLookupVisibility(ctx, []string{"global-1"})

	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter == nil || queryFilter.Formula == nil {
		t.Fatalf("expected merged query filter formula")
	}
	if !logicalExpressionHasEqField(*queryFilter.Formula, descriptorIDField) {
		t.Fatalf("expected merged formula to preserve existing field %q, got %#v", descriptorIDField, queryFilter.Formula)
	}
	if !logicalExpressionHasEqField(*queryFilter.Formula, grammar.ModelStringPattern("$aasdesc#globalAssetId")) {
		t.Fatalf("expected merged formula to include global asset ID condition, got %#v", queryFilter.Formula)
	}

	readFormula, ok := queryFilter.FormulasByRight[grammar.RightsEnumREAD]
	if !ok {
		t.Fatalf("expected READ formula in query filter")
	}
	if !logicalExpressionHasEqField(readFormula, descriptorIDField) {
		t.Fatalf("expected READ formula to preserve existing field %q, got %#v", descriptorIDField, readFormula)
	}
	if !logicalExpressionHasEqField(readFormula, grammar.ModelStringPattern("$aasdesc#globalAssetId")) {
		t.Fatalf("expected READ formula to include global asset ID condition, got %#v", readFormula)
	}
}

func TestBuildBasicDiscoveryAssetLinkQuery_UsesBDRootAlias(t *testing.T) {
	t.Parallel()

	b := false
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{
		Condition: &grammar.LogicalExpression{Boolean: &b},
	})

	query := buildBasicDiscoveryAssetLinkQueryWithAccess(ctx, []model.AssetLink{{Name: "name", Value: "value"}}, false)
	if query.Condition == nil || len(query.Condition.And) != 1 {
		t.Fatalf("expected one basic-discovery asset-link condition, got %#v", query.Condition)
	}
	match := query.Condition.And[0].Or[0].Match
	if len(match) == 0 || match[0].Eq[0].Field == nil || *match[0].Eq[0].Field != "$aasdesc#specificAssetIds[].value" {
		t.Fatalf("expected query to filter $aasdesc#specificAssetIds[].value, got %#v", query.Condition)
	}
}

func logicalExpressionHasEqField(expr grammar.LogicalExpression, field grammar.ModelStringPattern) bool {
	for _, value := range expr.Eq {
		if valueHasField(value, field) {
			return true
		}
	}

	for _, child := range expr.And {
		if logicalExpressionHasEqField(child, field) {
			return true
		}
	}

	for _, child := range expr.Or {
		if logicalExpressionHasEqField(child, field) {
			return true
		}
	}

	if expr.Not != nil {
		return logicalExpressionHasEqField(*expr.Not, field)
	}

	return false
}

func valueHasField(value grammar.Value, field grammar.ModelStringPattern) bool {
	if value.Field != nil && *value.Field == field {
		return true
	}
	if value.StrCast != nil {
		return valueHasField(*value.StrCast, field)
	}
	if value.BoolCast != nil {
		return valueHasField(*value.BoolCast, field)
	}
	if value.DateTimeCast != nil {
		return valueHasField(*value.DateTimeCast, field)
	}
	if value.NumCast != nil {
		return valueHasField(*value.NumCast, field)
	}
	if value.TimeCast != nil {
		return valueHasField(*value.TimeCast, field)
	}

	return false
}
