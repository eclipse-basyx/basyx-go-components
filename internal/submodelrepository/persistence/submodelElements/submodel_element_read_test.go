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

package submodelelements

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestBuildSubmodelElementReferenceBuildsKeyChainForNestedPathWithListIndex(t *testing.T) {
	t.Parallel()

	reference, err := buildSubmodelElementReference("sm-1", types.ModelTypeSubmodelElementList, "test.test[0]")
	require.NoError(t, err)

	keys := reference.Keys()
	require.Len(t, keys, 4)

	require.Equal(t, types.KeyTypesSubmodel, keys[0].Type())
	require.Equal(t, "sm-1", keys[0].Value())
	require.Equal(t, types.KeyTypesSubmodelElementCollection, keys[1].Type())
	require.Equal(t, "test", keys[1].Value())
	require.Equal(t, types.KeyTypesSubmodelElementCollection, keys[2].Type())
	require.Equal(t, "test", keys[2].Value())
	require.Equal(t, types.KeyTypesSubmodelElementList, keys[3].Type())
	require.Equal(t, "0", keys[3].Value())
}

func TestBuildSubmodelElementReferenceBuildsKeyChainForNestedDotPath(t *testing.T) {
	t.Parallel()

	reference, err := buildSubmodelElementReference("sm-1", types.ModelTypeProperty, "parent.child")
	require.NoError(t, err)

	keys := reference.Keys()
	require.Len(t, keys, 3)

	require.Equal(t, types.KeyTypesSubmodel, keys[0].Type())
	require.Equal(t, "sm-1", keys[0].Value())
	require.Equal(t, types.KeyTypesSubmodelElementCollection, keys[1].Type())
	require.Equal(t, "parent", keys[1].Value())
	require.Equal(t, types.KeyTypesProperty, keys[2].Type())
	require.Equal(t, "child", keys[2].Value())
}

func TestEscapeSQLLikePatternEscapesWildcardCharacters(t *testing.T) {
	t.Parallel()

	require.Equal(t, "A!_B", escapeSQLLikePattern("A_B"))
	require.Equal(t, "A!%B", escapeSQLLikePattern("A%B"))
	require.Equal(t, "A!!B", escapeSQLLikePattern("A!B"))
	require.Equal(t, "A!!B!_C!%", escapeSQLLikePattern("A!B_C%"))
}

func TestAddSMERowFilterQueriesWithoutMatchUsesContainingSubmodel(t *testing.T) {
	t.Parallel()

	var condition grammar.LogicalExpression
	err := json.Unmarshal([]byte(`{
		"$eq": [
			{"$field": "$sme#semanticId.keys[].value"},
			{"$strVal": "0112/2///61360_7#AAS011#001"}
		]
	}`), &condition)
	require.NoError(t, err)

	ctx := auth.WithQueryFilter(contextWithABACDisabled(t), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sme": auth.NewFragmentFilterPredicate(condition, false),
		},
	})
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.id"))
	filtered, err := addSMERowFilterQueries(ctx, dataset)
	require.NoError(t, err)
	sqlQuery, _, err := filtered.ToSQL()
	require.NoError(t, err)

	normalizedSQL := strings.ReplaceAll(sqlQuery, " ", "")
	require.Contains(t, normalizedSQL, `"submodel_element"."submodel_id"="sme"."submodel_id"`)
	require.NotContains(t, normalizedSQL, `"submodel_element"."id"="sme"."id"`)
}

func TestAddSMERowFilterQueriesMatchCorrelatesSamePathToCurrentElement(t *testing.T) {
	t.Parallel()

	fragment := grammar.FragmentStringPattern("$sme.a[].b[]")
	condition := unmarshalLogicalExpression(t, `{
		"$eq": [
			{"$field": "$sme.a[].b[]#value"},
			{"$strVal": "blue"}
		]
	}`)

	sqlQuery := buildSMERowFilterSQL(t, fragment, condition, true)
	normalizedSQL := strings.ReplaceAll(sqlQuery, " ", "")
	require.Contains(t, normalizedSQL, `"submodel_element"."id"="sme"."id"`)
	require.NotContains(t, normalizedSQL, `"submodel_element"."submodel_id"="sme"."submodel_id"`)
}

func TestAddSMERowFilterQueriesMatchCorrelatesDescendantPathToCurrentPath(t *testing.T) {
	t.Parallel()

	fragment := grammar.FragmentStringPattern("$sme.a[]")
	condition := unmarshalLogicalExpression(t, `{
		"$eq": [
			{"$field": "$sme.a[].b[]#value"},
			{"$strVal": "blue"}
		]
	}`)

	sqlQuery := buildSMERowFilterSQL(t, fragment, condition, true)
	require.Contains(t, sqlQuery, `"submodel_element"."idshort_path" ~`)
	require.Contains(t, sqlQuery, `"sme"."idshort_path"`)
	require.Contains(t, sqlQuery, "REPLACE")
	require.NotContains(t, strings.ReplaceAll(sqlQuery, " ", ""), `"submodel_element"."id"="sme"."id"`)
}

func TestAddSMERowFilterQueriesMatchEvaluatesUnrelatedPathAgainstContainingSubmodel(t *testing.T) {
	t.Parallel()

	fragment := grammar.FragmentStringPattern("$sme.a[].b[]")
	condition := unmarshalLogicalExpression(t, `{
		"$eq": [
			{"$field": "$sme.guard#value"},
			{"$strVal": "enabled"}
		]
	}`)

	sqlQuery := buildSMERowFilterSQL(t, fragment, condition, true)
	normalizedSQL := strings.ReplaceAll(sqlQuery, " ", "")
	require.Contains(t, normalizedSQL, `"submodel_element"."submodel_id"="sme"."submodel_id"`)
	require.NotContains(t, normalizedSQL, `"submodel_element"."id"="sme"."id"`)
}

func unmarshalLogicalExpression(t *testing.T, value string) grammar.LogicalExpression {
	t.Helper()

	var condition grammar.LogicalExpression
	require.NoError(t, json.Unmarshal([]byte(value), &condition))
	return condition
}

func buildSMERowFilterSQL(
	t *testing.T,
	fragment grammar.FragmentStringPattern,
	condition grammar.LogicalExpression,
	match bool,
) string {
	t.Helper()

	queryFilter := &auth.QueryFilter{
		Filters: auth.FragmentFilters{fragment: auth.NewFragmentFilterPredicate(condition, match)},
	}
	ctx := auth.WithQueryFilter(contextWithABACDisabled(t), queryFilter)
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.id"))
	filtered, err := addSMERowFilterQueries(ctx, dataset)
	require.NoError(t, err)
	sqlQuery, _, err := filtered.ToSQL()
	require.NoError(t, err)
	return sqlQuery
}

func TestAddSMERowFilterQueriesGuardsPathSpecificStructuralFragment(t *testing.T) {
	t.Parallel()

	deny := false
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sme.ARestricted": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &deny}, false),
		},
	})
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.id"))
	filtered, err := addSMERowFilterQueries(ctx, dataset)
	require.NoError(t, err)
	sqlQuery, _, err := filtered.ToSQL()
	require.NoError(t, err)

	require.Contains(t, sqlQuery, `"sme"."idshort_path"`)
	require.NotContains(t, sqlQuery, `"submodel_element"."idshort_path"`)
	require.Contains(t, sqlQuery, "ARestricted")
	require.Contains(t, sqlQuery, "NOT")
}

func TestAddSMEVisibleTreeQueryFiltersAncestorsBeforeLimit(t *testing.T) {
	t.Parallel()

	deny := false
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sme.ARestricted": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &deny}, false),
		},
	})
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path")).
		Order(goqu.I("sme.idshort_path").Asc()).
		Limit(2)

	filtered, err := addSMEVisibleTreeQuery(ctx, dataset, 42)
	require.NoError(t, err)
	sqlQuery, _, err := filtered.ToSQL()
	require.NoError(t, err)

	require.Contains(t, sqlQuery, "WITH RECURSIVE visible_sme_ids(id)")
	require.Contains(t, sqlQuery, `"visible_sme_child"."parent_sme_id" = "visible_sme_parent"."id"`)
	require.Contains(t, sqlQuery, `"visible_sme_root"."idshort_path"`)
	require.Contains(t, sqlQuery, `"visible_sme_child"."idshort_path"`)
	require.Contains(t, sqlQuery, `"sme"."id" IN ((SELECT "id" FROM "visible_sme_ids"))`)
	require.NotContains(t, sqlQuery, `"submodel_element"."idshort_path"`)
	require.Contains(t, sqlQuery, "LIMIT 2")
}

func TestAddSMEVisibleTreeQueryForLevelUsesRowFilterOnlyForCore(t *testing.T) {
	t.Parallel()

	deny := false
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sme.ARestricted": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &deny}, false),
		},
	})
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path"))

	filtered, err := addSMEVisibleTreeQueryForLevel(ctx, dataset, 42, "core")
	require.NoError(t, err)
	sqlQuery, _, err := filtered.ToSQL()
	require.NoError(t, err)

	require.NotContains(t, sqlQuery, "WITH RECURSIVE")
	require.Contains(t, sqlQuery, `"sme"."idshort_path"`)
	require.Contains(t, sqlQuery, "ARestricted")
}

func TestAddSMEVisibleSubtreeQueryForLevelRecursesOnlyForDeep(t *testing.T) {
	t.Parallel()

	allow := true
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sme": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &allow}, false),
		},
	})
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path"))

	coreQuery, err := addSMEVisibleSubtreeQueryForLevel(ctx, dataset, 42, "Target", "core")
	require.NoError(t, err)
	coreSQL, _, err := coreQuery.ToSQL()
	require.NoError(t, err)
	require.NotContains(t, coreSQL, "visible_sme_subtree_ids")

	deepQuery, err := addSMEVisibleSubtreeQueryForLevel(ctx, dataset, 42, "Target", "deep")
	require.NoError(t, err)
	deepSQL, _, err := deepQuery.ToSQL()
	require.NoError(t, err)
	require.Contains(t, deepSQL, "WITH RECURSIVE visible_sme_subtree_ids(id)")
	require.Contains(t, deepSQL, `"visible_sme_subtree_root"."idshort_path" = 'Target'`)
}

func TestAddSMEPathAncestorVisibilityQueryStartsAtTarget(t *testing.T) {
	t.Parallel()

	allow := true
	ctx := auth.WithQueryFilter(contextWithABACDisabled(t), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sme": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &allow}, false),
		},
	})
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path")).
		Where(goqu.I("sme.idshort_path").Eq("Target.Child"))

	filtered, err := addSMEPathAncestorVisibilityQuery(ctx, dataset, 42, "Target.Child")
	require.NoError(t, err)
	sqlQuery, _, err := filtered.ToSQL()
	require.NoError(t, err)

	require.Contains(t, sqlQuery, "WITH RECURSIVE visible_sme_path_ancestors(id,parent_sme_id)")
	require.Contains(t, sqlQuery, `"visible_sme_target"."idshort_path" = 'Target.Child'`)
	require.Contains(t, sqlQuery, `"visible_sme_ancestor"."id" = "visible_sme_child"."parent_sme_id"`)
	require.Contains(t, sqlQuery, `"visible_sme_path_ancestors"."parent_sme_id" IS NULL`)
	require.NotContains(t, sqlQuery, "visible_sme_ids")
}

func TestGetSubmodelElementByPathCombinesAuthorizationAndPayloadQuery(t *testing.T) {
	t.Parallel()

	var formula grammar.LogicalExpression
	err := json.Unmarshal([]byte(`{
		"$eq": [
			{"$field": "$sme#semanticId.keys[].value"},
			{"$strVal": "0112/2///61360_7#AAS011#001"}
		]
	}`), &formula)
	require.NoError(t, err)

	cfg := &common.Config{}
	cfg.ABAC.Enabled = true
	ctx := common.ContextWithConfig(context.Background(), cfg)
	allow := true
	ctx = auth.WithQueryFilter(ctx, &auth.QueryFilter{
		Formula: &formula,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: formula,
		},
		Filters: auth.FragmentFilters{
			"$sme": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &allow}, false),
		},
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
	})

	mock.ExpectQuery(
		`WITH RECURSIVE visible_sme_path_ancestors.*` +
			regexp.QuoteMeta(`"submodel_element"."submodel_id" = "sme"."submodel_id"`) +
			`.*sme_path_data`,
	).WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err = getSubmodelElementByIDShortOrPathWithSubmodelDBID(ctx, db, "submodel-id", 42, "Target", "deep", true)
	require.Error(t, err)
	require.Truef(t, common.IsErrNotFound(err), "expected not found, got %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSubmodelElementCursorExistsCodesRowsError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
	})

	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"id"}).
			AddRow(1).
			RowError(0, errors.New("cursor row failure")),
	)
	dataset := goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.id"))

	exists, err := submodelElementCursorExists(context.Background(), db, dataset, "Target|1")
	require.False(t, exists)
	require.ErrorContains(t, err, "SMREPO-CHECKSMECURSOR-ROWSERR")
	require.NoError(t, mock.ExpectationsWereMet())
}

func contextWithABACDisabled(t *testing.T) context.Context {
	t.Helper()

	var cfgCtx context.Context
	handler := common.ConfigMiddleware(&common.Config{})(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		cfgCtx = request.Context()
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	require.NotNil(t, cfgCtx)
	return cfgCtx
}

func TestNormalizeSMERowFiltersIgnoresOtherStructuralRoots(t *testing.T) {
	t.Parallel()

	allow := true
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sm":  auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &allow}, false),
			"$sme": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &allow}, false),
		},
	})

	filterCtx, fragments, err := normalizeSMERowFilters(ctx)
	require.NoError(t, err)
	require.Equal(t, []grammar.FragmentStringPattern{"$sme#idShort"}, fragments)
	require.NotContains(t, auth.GetQueryFilter(filterCtx).Filters, grammar.FragmentStringPattern("$sm"))
	require.NotContains(t, auth.GetQueryFilter(filterCtx).Filters, grammar.FragmentStringPattern("$sm#idShort"))
}

func TestNormalizeSMERowFiltersDoesNotMergeFieldMasks(t *testing.T) {
	t.Parallel()

	allow := true
	deny := false
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sme":         auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &allow}, false),
			"$sme#idShort": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &deny}, false),
		},
	})

	filterCtx, fragments, err := normalizeSMERowFilters(ctx)
	require.NoError(t, err)
	require.Equal(t, []grammar.FragmentStringPattern{"$sme#idShort"}, fragments)
	rowFilter := auth.GetQueryFilter(filterCtx).Filters["$sme#idShort"]
	require.NotNil(t, rowFilter.Condition)
	require.NotNil(t, rowFilter.Condition.Boolean)
	require.True(t, *rowFilter.Condition.Boolean)
}
