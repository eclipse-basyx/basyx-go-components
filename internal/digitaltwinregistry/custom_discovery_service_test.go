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
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
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

func TestAuthorizedBasicDiscoveryGlobalAssetIDQueryUsesVisibleBDField(t *testing.T) {
	t.Parallel()

	policyField := grammar.ModelStringPattern("$aasdesc#globalAssetId")
	publicReadable := grammar.StandardString("public-visible-global")
	policy := grammar.LogicalExpression{Eq: grammar.ComparisonItems{
		{Field: &policyField},
		{StrVal: &publicReadable},
	}}
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	ctx = auth.WithQueryFilter(ctx, &auth.QueryFilter{
		Formula: &policy,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: policy,
		},
	})
	caller := buildBasicDiscoveryGlobalAssetIDQuery([]string{"global-asset"})
	if caller.Condition == nil || len(caller.Condition.And) != 1 ||
		len(caller.Condition.And[0].Eq) != 2 || caller.Condition.And[0].Eq[0].Field == nil ||
		*caller.Condition.And[0].Eq[0].Field != "$bd#globalAssetId" {
		t.Fatalf("expected a basic-discovery globalAssetId condition, got %#v", caller.Condition)
	}
	ctx, err := auth.WithAuthorizedQuery(ctx, auth.SemanticResourceBD, caller)
	if err != nil {
		t.Fatalf("create authorized discovery query: %v", err)
	}
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootBD)
	if err != nil {
		t.Fatalf("create basic-discovery collector: %v", err)
	}
	dataset, err := auth.AddFormulaQueryFromContext(
		ctx,
		goqu.Dialect("postgres").
			From("aas_identifier").
			Join(
				goqu.T("aas_descriptor"),
				goqu.On(goqu.I("aas_descriptor.id").Eq(goqu.I("aas_identifier.aasid"))),
			),
		collector,
	)
	if err != nil {
		t.Fatalf("compile authorized discovery query: %v", err)
	}
	sql, args, err := dataset.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("render authorized discovery query: %v", err)
	}
	if strings.Contains(sql, "NULL") {
		t.Fatalf("basic-discovery globalAssetId was neutralized as an unavailable semantic field:\n%s", sql)
	}
	if strings.Count(sql, `"aas_descriptor"."global_asset_id"`) < 2 {
		t.Fatalf("expected policy and caller globalAssetId predicates, got:\n%s\nargs: %#v", sql, args)
	}
	for _, expected := range []any{"public-visible-global", "global-asset"} {
		found := false
		for _, arg := range args {
			if arg == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected SQL args to contain %q, got %#v", expected, args)
		}
	}
}
