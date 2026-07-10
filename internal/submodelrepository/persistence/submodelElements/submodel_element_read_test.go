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
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
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

func TestAddSMERowFilterQueriesCorrelatesStructuralConditionToCurrentElement(t *testing.T) {
	t.Parallel()

	var condition grammar.LogicalExpression
	err := json.Unmarshal([]byte(`{
		"$eq": [
			{"$field": "$sme#semanticId.keys[].value"},
			{"$strVal": "0112/2///61360_7#AAS011#001"}
		]
	}`), &condition)
	require.NoError(t, err)

	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sme": condition,
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
	require.Contains(t, normalizedSQL, `"submodel_element"."id"="sme"."id"`)
	require.NotContains(t, normalizedSQL, `"submodel_element"."submodel_id"="sme"."submodel_id"`)
}

func TestAddSMERowFilterQueriesGuardsPathSpecificStructuralFragment(t *testing.T) {
	t.Parallel()

	deny := false
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sme.ARestricted": {Boolean: &deny},
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
			"$sme.ARestricted": {Boolean: &deny},
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

func TestNormalizeSMERowFiltersIgnoresOtherStructuralRoots(t *testing.T) {
	t.Parallel()

	allow := true
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$sm":  {Boolean: &allow},
			"$sme": {Boolean: &allow},
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
			"$sme":         {Boolean: &allow},
			"$sme#idShort": {Boolean: &deny},
		},
	})

	filterCtx, fragments, err := normalizeSMERowFilters(ctx)
	require.NoError(t, err)
	require.Equal(t, []grammar.FragmentStringPattern{"$sme#idShort"}, fragments)
	rowFilter := auth.GetQueryFilter(filterCtx).Filters["$sme#idShort"]
	require.NotNil(t, rowFilter.Boolean)
	require.True(t, *rowFilter.Boolean)
}
