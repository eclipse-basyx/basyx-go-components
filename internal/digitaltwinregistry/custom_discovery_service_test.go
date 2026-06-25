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

func TestReplaceReadFormula_GlobalAssetIDLookupSkipsRouteSpecificAssetAuthorization(t *testing.T) {
	t.Parallel()

	originalFormula := publicReadableExternalSubjectExpression()
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{Condition: &originalFormula})

	query := buildAssetLinkQuery(ctx, []model.AssetLink{
		{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"},
	})
	if query.Condition == nil {
		t.Fatalf("expected globalAssetId lookup condition")
	}

	ctx = replaceReadFormula(ctx, *query.Condition)
	sql, args := renderBDReadFormulaSQL(t, ctx)

	for _, want := range []string{common.GlobalAssetIDAssetLinkName, "global-asset"} {
		if !containsArg(args, want) {
			t.Fatalf("expected globalAssetId lookup arg %q, got SQL: %s args: %#v", want, sql, args)
		}
	}
	if strings.Contains(sql, "external_subject_reference_key") || containsArg(args, "PUBLIC_READABLE") {
		t.Fatalf("did not expect globalAssetId lookup to require external subject authorization, got SQL: %s args: %#v", sql, args)
	}
}

func TestReplaceReadFormula_MixedGlobalAssetIDLookupKeepsSpecificAssetAuthorization(t *testing.T) {
	t.Parallel()

	originalFormula := publicReadableExternalSubjectExpression()
	ctx := auth.MergeQueryFilter(context.Background(), grammar.Query{Condition: &originalFormula})
	ctx = context.WithValue(ctx, auth.ClaimsKey, auth.Claims{"Edc-Bpn": "BPN_COMPANY_001"})

	query := buildAssetLinkQuery(ctx, []model.AssetLink{
		{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"},
		{Name: "customerPartId", Value: "4711"},
	})
	if query.Condition == nil {
		t.Fatalf("expected mixed asset-link lookup condition")
	}

	ctx = replaceReadFormula(ctx, *query.Condition)
	sql, args := renderBDReadFormulaSQL(t, ctx)

	for _, want := range []string{
		common.GlobalAssetIDAssetLinkName,
		"global-asset",
		"customerPartId",
		"4711",
		"BPN_COMPANY_001",
		"PUBLIC_READABLE",
	} {
		if !containsArg(args, want) {
			t.Fatalf("expected mixed lookup arg %q, got SQL: %s args: %#v", want, sql, args)
		}
	}
	if !strings.Contains(sql, "external_subject_reference_key") {
		t.Fatalf("expected non-global asset link to require external subject authorization, got SQL: %s", sql)
	}
}

func publicReadableExternalSubjectExpression() grammar.LogicalExpression {
	externalSubject := grammar.ModelStringPattern("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value")
	publicReadable := grammar.StandardString("PUBLIC_READABLE")
	return grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			{Field: &externalSubject},
			{StrVal: &publicReadable},
		},
	}
}

func renderBDReadFormulaSQL(t *testing.T, ctx context.Context) (string, []interface{}) {
	t.Helper()

	qf := auth.GetQueryFilter(ctx)
	if qf == nil || qf.Formula == nil {
		t.Fatalf("expected read formula in context")
	}

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootBD)
	if err != nil {
		t.Fatalf("failed to build collector: %v", err)
	}
	expr, _, err := qf.Formula.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("expected read formula to compile: %v", err)
	}

	sql, args, err := goqu.Dialect(common.Dialect).
		From(goqu.T(common.TblAASIdentifier)).
		LeftJoin(
			goqu.T(common.TblAASDescriptor),
			goqu.On(goqu.I(common.TblAASDescriptor+"."+common.ColAASID).Eq(goqu.I(common.TblAASIdentifier+".aasid"))),
		).
		Select(goqu.V(1)).
		Where(expr).
		Prepared(true).
		ToSQL()
	if err != nil {
		t.Fatalf("expected SQL generation to succeed: %v", err)
	}
	return sql, args
}

func containsArg(args []interface{}, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
