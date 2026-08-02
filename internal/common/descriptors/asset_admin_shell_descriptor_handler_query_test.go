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
	stdsql "database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func descriptorFieldEquals(field grammar.ModelStringPattern, value string) grammar.LogicalExpression {
	literal := grammar.StandardString(value)
	return grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			{Field: &field},
			{StrVal: &literal},
		},
	}
}

func TestIsTransactionQueryerRecognizesDebugWrapper(t *testing.T) {
	tx := &stdsql.Tx{}
	wrapped := &descriptorDebugQueryer{db: tx}

	if !isTransactionQueryer(wrapped) {
		t.Fatal("expected a debug-wrapped transaction to retain transaction identity")
	}
}

func contextWithABACDisabled(t *testing.T) context.Context {
	t.Helper()

	cfg := &common.Config{}
	var cfgCtx context.Context
	handler := common.ConfigMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cfgCtx = r.Context()
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if cfgCtx == nil {
		t.Fatal("failed to create config-bearing context")
	}
	return cfgCtx
}

func TestBuildSingleStatementAASDescriptorListQueryUsesPagedInnerQuery(t *testing.T) {
	ctx := contextWithABACDisabled(t)
	ds, err := buildSingleStatementAASDescriptorListQuery(
		ctx,
		2,
		"",
		"",
		"",
		"",
		time.Time{},
		time.Time{},
	)
	if err != nil {
		t.Fatalf("buildSingleStatementAASDescriptorListQuery returned error: %v", err)
	}

	sql, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	for _, want := range []string{
		`FROM (SELECT`,
		`AS "aas_page"`,
		`LIMIT $`,
		`jsonb_build_object`,
		`"aas_descriptor"."descriptor_id"`,
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected SQL to contain %q, got: %s", want, sql)
		}
	}
	hasLimitArg := false
	for _, arg := range args {
		if v, ok := arg.(int64); ok && v == 2 {
			hasLimitArg = true
			break
		}
		if v, ok := arg.(int); ok && v == 2 {
			hasLimitArg = true
			break
		}
	}
	if !hasLimitArg {
		t.Fatalf("expected prepared args to contain limit 2, got: %#v", args)
	}
}

func TestBuildSingleStatementAASDescriptorListQueryAppliesSharedMaskCondition(t *testing.T) {
	field := grammar.ModelStringPattern("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value")
	lit := grammar.StandardString("PUBLIC_READABLE")
	cond := grammar.LogicalExpression{
		Eq: []grammar.Value{
			{Field: &field},
			{StrVal: &lit},
		},
	}

	fAssetKind := grammar.FragmentStringPattern("$aasdesc#assetKind")
	fAssetType := grammar.FragmentStringPattern("$aasdesc#assetType")
	fDescription := grammar.FragmentStringPattern("$aasdesc#description")

	ctx := auth.MergeQueryFilter(contextWithABACDisabled(t), grammar.Query{
		FilterConditions: []grammar.SubFilter{
			{Fragment: &fAssetKind, Condition: &cond},
			{Fragment: &fAssetType, Condition: &cond},
			{Fragment: &fDescription, Condition: &cond},
		},
	})

	ds, err := buildSingleStatementAASDescriptorListQuery(ctx, 2, "", "", "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("buildSingleStatementAASDescriptorListQuery returned error: %v", err)
	}
	sql, _, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if got := strings.Count(sql, "EXISTS ("); got != 1 {
		t.Fatalf("expected exactly 1 EXISTS for shared fragment condition, got %d: %s", got, sql)
	}
}

func TestBuildSingleStatementAASDescriptorListQueryCorrelatesSubmodelRouteFilters(t *testing.T) {
	fragment := grammar.FragmentStringPattern("$aasdesc#submodelDescriptors[]")
	supplementalKey := grammar.ModelStringPattern("$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value")
	externalSubjectKey := grammar.ModelStringPattern("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value")
	condition := grammar.LogicalExpression{
		And: []grammar.LogicalExpression{
			descriptorFieldEquals(supplementalKey, "PUBLIC_READABLE"),
			descriptorFieldEquals(externalSubjectKey, "PUBLIC_READABLE"),
		},
	}
	ctx := auth.WithQueryFilter(contextWithABACDisabled(t), &auth.QueryFilter{
		Filters: auth.FragmentFilters{fragment: condition},
		FilterMatch: auth.FragmentMatchModes{
			fragment: true,
		},
	})

	ds, err := buildSingleStatementAASDescriptorListQuery(ctx, 2, "", "", "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("buildSingleStatementAASDescriptorListQuery returned error: %v", err)
	}
	sql, _, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	for _, correlatedPath := range []string{
		`LEFT JOIN "submodel_descriptor_supplemental_semantic_id_reference"`,
		`"aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key"."value" = 'PUBLIC_READABLE'`,
		`EXISTS (SELECT 1 FROM "specific_asset_id"`,
	} {
		if !strings.Contains(sql, correlatedPath) {
			t.Fatalf("expected route filter to contain %q, got: %s", correlatedPath, sql)
		}
	}
}

func TestBuildSingleStatementAASDescriptorListQueryFiltersReferenceParentsByKeys(t *testing.T) {
	fragment := grammar.FragmentStringPattern("$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[]")
	keyValue := grammar.ModelStringPattern("$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value")
	ctx := auth.WithQueryFilter(contextWithABACDisabled(t), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			fragment: descriptorFieldEquals(keyValue, "VISIBLE_KEY"),
		},
	})

	ds, err := buildSingleStatementAASDescriptorListQuery(ctx, 2, "", "", "", "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("buildSingleStatementAASDescriptorListQuery returned error: %v", err)
	}
	sql, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if got := countArgument(args, "VISIBLE_KEY"); got != 2 {
		t.Fatalf("expected the key predicate on both parent and key queries, got %d occurrences: %s", got, sql)
	}
}

func TestBuildSingleStatementAASDescriptorListQueryAvoidsMaskLayersWithoutFilters(t *testing.T) {
	ds, err := buildSingleStatementAASDescriptorListQuery(
		contextWithABACDisabled(t),
		2,
		"",
		"",
		"",
		"",
		time.Time{},
		time.Time{},
	)
	if err != nil {
		t.Fatalf("buildSingleStatementAASDescriptorListQuery returned error: %v", err)
	}
	sql, _, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	for _, unwanted := range []string{`"flag_`, `SELECT DISTINCT`} {
		if strings.Contains(sql, unwanted) {
			t.Fatalf("expected the unfiltered query to avoid %q, got: %s", unwanted, sql)
		}
	}
}

func countArgument(args []interface{}, expected string) int {
	count := 0
	for _, arg := range args {
		if arg == expected {
			count++
		}
	}
	return count
}
