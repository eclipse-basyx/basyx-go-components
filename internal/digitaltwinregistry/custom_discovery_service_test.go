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

func TestBuildAssetLinkQuery_GlobalAssetIDUsesDescriptorValueAndTwinExternalSubjects(t *testing.T) {
	t.Parallel()

	ctx := restrictedReadContext()
	ctx = context.WithValue(ctx, auth.ClaimsKey, auth.Claims{"Edc-Bpn": "BPN_COMPANY_001"})

	query := buildAssetLinkQuery(ctx, []model.AssetLink{
		{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"},
	})
	if query.Condition == nil {
		t.Fatalf("expected globalAssetId condition")
	}

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootBD)
	if err != nil {
		t.Fatalf("failed to build collector: %v", err)
	}
	expr, _, err := query.Condition.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("expected globalAssetId query to compile: %v", err)
	}

	sql, _, err := goqu.Dialect(common.Dialect).
		From(goqu.T(common.TblAASIdentifier)).
		LeftJoin(
			goqu.T(common.TblAASDescriptor),
			goqu.On(goqu.I(common.TblAASDescriptor+"."+common.ColAASID).Eq(goqu.I(common.TblAASIdentifier+".aasid"))),
		).
		Select(goqu.V(1)).
		Where(expr).
		ToSQL()
	if err != nil {
		t.Fatalf("expected SQL generation to succeed: %v", err)
	}

	if !strings.Contains(sql, `"aas_descriptor"."global_asset_id" = 'global-asset'`) {
		t.Fatalf("expected direct global_asset_id comparison, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "EXISTS") || !strings.Contains(sql, `"external_subject_reference_key"`) {
		t.Fatalf("expected external-subject EXISTS authorization, got SQL: %s", sql)
	}
	if strings.Contains(sql, `"specific_asset_id"."name" = 'globalAssetId'`) {
		t.Fatalf("did not expect globalAssetId visibility to depend on generated specific_asset_id row, got SQL: %s", sql)
	}
}

func restrictedReadContext() context.Context {
	b := false
	return auth.MergeQueryFilter(context.Background(), grammar.Query{
		Condition: &grammar.LogicalExpression{Boolean: &b},
	})
}
