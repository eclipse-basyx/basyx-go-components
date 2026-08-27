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
	"database/sql"
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

func TestSubmodelElementPathReadReusesSQLShapeForDifferentPaths(t *testing.T) {
	t.Parallel()

	firstQuery := captureSubmodelElementPathReadQuery(t, "Motor.Nameplate.ManufacturerName")
	secondQuery := captureSubmodelElementPathReadQuery(t, "TechnicalData.Sections[12].MaximumRotationSpeed")

	require.Equal(t, firstQuery, secondQuery)
	require.NotContains(t, firstQuery, "Motor.Nameplate.ManufacturerName")
	require.NotContains(t, secondQuery, "TechnicalData.Sections[12].MaximumRotationSpeed")
}

func TestSubmodelElementPathListReusesSQLShapeForDifferentPaths(t *testing.T) {
	t.Parallel()

	firstQuery := captureSubmodelElementPathListQuery(t, "Motor.Nameplate.ManufacturerName")
	secondQuery := captureSubmodelElementPathListQuery(t, "TechnicalData.Sections[12].MaximumRotationSpeed")

	require.Equal(t, firstQuery, secondQuery)
	require.NotContains(t, firstQuery, "Motor.Nameplate.ManufacturerName")
	require.NotContains(t, secondQuery, "TechnicalData.Sections[12].MaximumRotationSpeed")
}

func TestSubmodelElementBatchReadReusesSQLShapeForDifferentRootCounts(t *testing.T) {
	t.Parallel()

	oneRootQuery := captureSubmodelElementBatchReadQuery(t, []int64{11})
	manyRootsQuery := captureSubmodelElementBatchReadQuery(t, []int64{11, 22, 33, 44})

	require.Equal(t, oneRootQuery, manyRootsQuery)
	require.Contains(t, oneRootQuery, "ANY($")
	require.Contains(t, oneRootQuery, "array_position($")
}

func TestSubmodelElementReferenceModelTypesReuseSQLShapeForDifferentRootCounts(t *testing.T) {
	t.Parallel()

	oneRootQuery, oneRootArgs, err := buildSubmodelElementModelTypesQuery(42, []int64{11})
	require.NoError(t, err)
	manyRootsQuery, manyRootsArgs, err := buildSubmodelElementModelTypesQuery(42, []int64{11, 22, 33, 44})
	require.NoError(t, err)

	require.Equal(t, oneRootQuery, manyRootsQuery)
	require.Len(t, oneRootArgs, 2)
	require.Len(t, manyRootsArgs, 2)
	require.Contains(t, oneRootQuery, "ANY($")
}

func TestSubmodelElementCursorCheckReusesSQLShapeForDifferentCursors(t *testing.T) {
	t.Parallel()

	firstQuery := captureSubmodelElementCursorQuery(t, "Motor|11")
	secondQuery := captureSubmodelElementCursorQuery(t, "TechnicalData|44")

	require.Equal(t, firstQuery, secondQuery)
	require.NotContains(t, firstQuery, "Motor")
	require.NotContains(t, secondQuery, "TechnicalData")
}

func captureSubmodelElementPathReadQuery(t *testing.T, idShortPath string) string {
	t.Helper()

	return captureSubmodelElementReadQuery(t, func(db *sql.DB) error {
		rows, err := readSubmodelElementRowsByPath(contextWithABACDisabled(t), db, 42, idShortPath, true, true)
		require.Empty(t, rows)
		return err
	})
}

func captureSubmodelElementPathListQuery(t *testing.T, idShortPath string) string {
	t.Helper()

	queries := make([]string, 0, 2)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actualQuery string) error {
		queries = append(queries, actualQuery)
		return nil
	})))
	require.NoError(t, err)

	mock.ExpectQuery("submodel lookup").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery("submodel element path list").
		WillReturnRows(sqlmock.NewRows([]string{"idshort_path"}).AddRow(idShortPath))

	paths, err := GetSubmodelElementPathsByPath(contextWithABACDisabled(t), db, "urn:example:submodel", idShortPath, "deep")
	require.NoError(t, err)
	require.Equal(t, []string{idShortPath}, paths)
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, queries, 2)

	return queries[1]
}

func captureSubmodelElementBatchReadQuery(t *testing.T, rootIDs []int64) string {
	t.Helper()

	return captureSubmodelElementReadQuery(t, func(db *sql.DB) error {
		rows, err := readSubmodelElementRowsByRootIDs(contextWithABACDisabled(t), db, 42, rootIDs, true, true, true)
		require.Empty(t, rows)
		return err
	})
}

func captureSubmodelElementReadQuery(t *testing.T, read func(db *sql.DB) error) string {
	t.Helper()

	var capturedQuery string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actualQuery string) error {
		capturedQuery = actualQuery
		return nil
	})))
	require.NoError(t, err)

	mock.ExpectQuery("capture query").WillReturnRows(sqlmock.NewRows([]string{"unused"}))
	require.NoError(t, read(db))
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotEmpty(t, capturedQuery)

	return capturedQuery
}

func captureSubmodelElementCursorQuery(t *testing.T, cursor string) string {
	t.Helper()

	return captureSubmodelElementReadQuery(t, func(db *sql.DB) error {
		query := goqu.Dialect("postgres").
			From(goqu.T("submodel_element").As("sme")).
			Select(goqu.I("sme.id"))
		_, err := submodelElementCursorExists(contextWithABACDisabled(t), db, query, cursor)
		return err
	})
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

func TestIsSubmodelElementPathAuthorizedUsesFormulaAndAncestorVisibility(t *testing.T) {
	t.Parallel()

	denied := false
	formula := grammar.LogicalExpression{Boolean: &denied}
	ctx := auth.WithQueryFilter(contextWithABACDisabled(t), &auth.QueryFilter{
		Formula: &formula,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumCREATE: formula,
		},
		Filters: auth.FragmentFilters{
			"$sme": auth.NewFragmentFilterPredicate(grammar.LogicalExpression{Boolean: &denied}, false),
		},
	})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
	})

	mock.ExpectQuery(`WITH RECURSIVE visible_sme_path_ancestors.*`).
		WillReturnRows(sqlmock.NewRows([]string{"visible"}))

	authorized, err := IsSubmodelElementPathAuthorized(ctx, db, 42, "Target.Child")
	require.NoError(t, err)
	require.False(t, authorized)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIsSubmodelElementPathAuthorizedSkipsQueryWithoutConstraints(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
	})

	baseCtx := contextWithABACDisabled(t)
	contexts := []struct {
		name string
		ctx  context.Context
	}{
		{name: "no query filter", ctx: baseCtx},
		{name: "empty query filter", ctx: auth.WithQueryFilter(baseCtx, &auth.QueryFilter{})},
	}
	for _, test := range contexts {
		authorized, authorizationErr := IsSubmodelElementPathAuthorized(test.ctx, db, 42, "Target.Child")
		require.NoError(t, authorizationErr, test.name)
		require.True(t, authorized, test.name)
	}
	require.NoError(t, mock.ExpectationsWereMet())
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

func TestSubmodelElementPageQueryShapeIsIndependentOfPageSize(t *testing.T) {
	t.Parallel()

	ctx := contextWithABACDisabled(t)
	oneQuery, oneArgs, err := buildSubmodelElementPageQuery(ctx, []int64{1}, false, "deep")
	require.NoError(t, err)
	manyQuery, manyArgs, err := buildSubmodelElementPageQuery(ctx, []int64{1, 2, 3}, false, "deep")
	require.NoError(t, err)

	require.Equal(t, oneQuery, manyQuery)
	require.Len(t, manyArgs, len(oneArgs))
	require.Contains(t, oneQuery, "ROW_NUMBER() OVER (PARTITION BY")
	require.Contains(t, oneQuery, "visible_submodel_roots")
	require.Contains(t, oneQuery, "selected_submodel_elements")
	require.Contains(t, oneQuery, "submodel_element_page_values")
	require.Contains(t, oneQuery, "UNION ALL")
	require.Contains(t, oneQuery, `SELECT "selected_root"."root_id" AS "element_id"`)
	require.NotContains(t, oneQuery, `"selected_sme"."id" = "selected_root"."root_id"`)
	require.Contains(t, oneQuery, `"selected_sme"."submodel_id" = "selected_root"."submodel_id"`)
	require.Contains(t, oneQuery, `"selected_sme"."root_sme_id"`)
	require.Contains(t, oneQuery, `"selected_sme"."id" != "selected_root"."root_id"`)
	require.Contains(t, oneQuery, `"property_element" AS "page_property"`)
	require.Contains(t, oneQuery, `"multilanguage_property_value" AS "page_multilanguage_value"`)
	require.Contains(t, oneQuery, `ORDER BY "page_multilanguage_value"."id"`)
	require.NotContains(t, oneQuery, `ORDER BY "page_multilanguage_value"."language"`)
	require.Contains(t, oneQuery, `IN ((SELECT "element_id" FROM "selected_submodel_elements"))`)
	require.Contains(t, oneQuery, `LEFT JOIN "submodel_element_page_values" AS "sme_value"`)
	require.NotContains(t, oneQuery, `NULL::jsonb AS "raw_value_payload"`)
}

func TestSubmodelElementCorePageSelectsOnlyRootsAndDirectChildren(t *testing.T) {
	t.Parallel()

	query, _, err := buildSubmodelElementPageQuery(contextWithABACDisabled(t), []int64{1}, false, "core")
	require.NoError(t, err)
	require.Contains(t, query, `"selected_sme"."parent_sme_id" = "selected_root"."root_id"`)
}

func TestSubmodelElementPageQueryHonorsBlobValueOption(t *testing.T) {
	t.Parallel()

	withoutValue, _, err := buildSubmodelElementPageQuery(contextWithABACDisabled(t), []int64{1}, false, "deep")
	require.NoError(t, err)
	withValue, _, err := buildSubmodelElementPageQuery(contextWithABACDisabled(t), []int64{1}, true, "deep")
	require.NoError(t, err)

	require.NotContains(t, withoutValue, `"page_blob"."value"`)
	require.Contains(t, withValue, `"page_blob"."value"`)
}

func TestAllSubmodelPathPageKeepsCompositeCursorAndStableOrder(t *testing.T) {
	t.Parallel()

	dialect := goqu.Dialect("postgres")
	visibleSubmodels := dialect.From("submodel").Select(
		goqu.I("id").As("submodel_id"),
		goqu.I("submodel_identifier").As("submodel_identifier"),
	)
	query, _, err := buildAllSubmodelElementPathsPageQuery(
		contextWithABACDisabled(t),
		visibleSubmodels,
		100,
		"urn:example:sm",
		"Collection.Property|42",
		"deep",
	)
	require.NoError(t, err)
	require.Contains(t, query, "WITH RECURSIVE")
	require.Contains(t, query, "authorized_submodel_paths")
	require.Contains(t, query, `SELECT 1 FROM "authorized_submodel_paths"`)
	require.Contains(t, query, `ORDER BY "authorized_path"."submodel_identifier" ASC, "authorized_path"."idshort_path" ASC, "authorized_path"."sme_id" ASC`)
	require.Contains(t, query, `"authorized_path"."sme_id" >`)
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
