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
	"fmt"
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

func TestBuildSingleStatementAASDescriptorListQueryReusesSQLShape(t *testing.T) {
	build := func(
		limit int32,
		cursor string,
		assetType string,
		identifiable string,
		createdFrom time.Time,
		updatedFrom time.Time,
	) (string, []interface{}) {
		t.Helper()
		ds, err := buildSingleStatementAASDescriptorListQuery(
			contextWithABACDisabled(t),
			limit,
			cursor,
			"Instance",
			assetType,
			identifiable,
			createdFrom,
			updatedFrom,
		)
		if err != nil {
			t.Fatalf("buildSingleStatementAASDescriptorListQuery returned error: %v", err)
		}
		sql, args, err := ds.Prepared(true).ToSQL()
		if err != nil {
			t.Fatalf("ToSQL returned error: %v", err)
		}
		return sql, args
	}

	firstSQL, firstArgs := build(
		2,
		"urn:example:cursor:first",
		"urn:example:asset-type:first",
		"urn:example:aas:first",
		time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC),
	)
	secondSQL, secondArgs := build(
		20,
		"urn:example:cursor:second",
		"urn:example:asset-type:second",
		"urn:example:aas:second",
		time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.February, 2, 0, 0, 0, 0, time.UTC),
	)

	if firstSQL != secondSQL {
		t.Fatalf("expected reusable SQL shape, got:\nfirst:  %s\nsecond: %s", firstSQL, secondSQL)
	}
	if fmt.Sprint(firstArgs) == fmt.Sprint(secondArgs) {
		t.Fatalf("expected different runtime arguments, got: %#v", firstArgs)
	}
}

func TestBuildSingleStatementAASDescriptorListQueryCorrelatesMultiKeyMaskFlags(t *testing.T) {
	tests := []struct {
		name             string
		fragment         grammar.FragmentStringPattern
		conditionField   grammar.ModelStringPattern
		joinedKeyPattern string
		partitionPattern string
	}{
		{
			name:             "specific asset ID",
			fragment:         "$aasdesc#specificAssetIds[].name",
			conditionField:   "$aasdesc#specificAssetIds[].externalSubjectId.keys[].value",
			joinedKeyPattern: `LEFT JOIN "specific_asset_id_external_subject_id_reference_key" AS "external_subject_reference_key"`,
			partitionPattern: `OVER (PARTITION BY "specific_asset_id"."id")`,
		},
		{
			name:             "submodel descriptor",
			fragment:         "$aasdesc#submodelDescriptors[].semanticId",
			conditionField:   "$aasdesc#submodelDescriptors[].semanticId.keys[].value",
			joinedKeyPattern: `LEFT JOIN "submodel_descriptor_semantic_id_reference_key" AS "aasdesc_submodel_descriptor_semantic_id_reference_key"`,
			partitionPattern: `OVER (PARTITION BY "submodel_descriptor"."descriptor_id")`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := auth.MergeQueryFilter(contextWithABACDisabled(t), grammar.Query{
				FilterConditions: []grammar.SubFilter{
					{Fragment: &test.fragment, Condition: pointerToDescriptorExpression(descriptorFieldEquals(test.conditionField, "MASK_MATCH"))},
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

			if !strings.Contains(sql, "BOOL_OR(") {
				t.Fatalf("expected mask flag to aggregate matching reference keys: %s", sql)
			}
			if !strings.Contains(sql, test.joinedKeyPattern) {
				t.Fatalf("expected mask query to join the reference keys: %s", sql)
			}
			if !strings.Contains(sql, test.partitionPattern) {
				t.Fatalf("expected mask flags to aggregate per parent: %s", sql)
			}
		})
	}
}

func pointerToDescriptorExpression(expression grammar.LogicalExpression) *grammar.LogicalExpression {
	return &expression
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
		Filters: auth.FragmentFilters{fragment: auth.NewFragmentFilterPredicate(condition, true)},
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
			fragment: auth.NewFragmentFilterPredicate(descriptorFieldEquals(keyValue, "VISIBLE_KEY"), false),
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
