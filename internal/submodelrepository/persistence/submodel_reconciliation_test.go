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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"encoding/json"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/postgresstaging"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	"github.com/stretchr/testify/require"
)

func TestStreamReconciliationRowsIncludesNestedElements(t *testing.T) {
	submodel := readReconciliationJSON(t, `{
		"id":"sm",
		"modelType":"Submodel",
		"submodelElements":[{
			"idShort":"C",
			"modelType":"SubmodelElementCollection",
			"value":[{
				"idShort":"L",
				"modelType":"SubmodelElementList",
				"typeValueListElement":"Property",
				"value":[{"modelType":"Property","valueType":"xs:string","value":"nested"}]
			}]
		}]
	}`)
	rows := make([]submodelelements.ReconciliationElementRow, 0, 3)
	err := submodelelements.StreamReconciliationElementRows(
		t.Context(),
		nil,
		submodel.SubmodelElements(),
		func(row submodelelements.ReconciliationElementRow) error {
			rows = append(rows, row)
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, "C", rows[0].Path)
	require.Equal(t, "C.L", rows[1].Path)
	require.Equal(t, "C.L[0]", rows[2].Path)
	require.Equal(t, "C.L", rows[2].ParentPath)
	require.Equal(t, "C", rows[2].RootPath)
	require.Equal(t, 2, rows[2].Depth)
}

func TestStagedElementComparisonUsesTypedPostgreSQLEquality(t *testing.T) {
	query, _, err := classifiedStagedUpdateRows(goqu.Dialect(common.Dialect)).ToSQL()
	require.NoError(t, err)
	require.Contains(t, query, "IS NOT DISTINCT FROM")
	require.Contains(t, query, "::timestamptz")
	require.Contains(t, query, "::time")
	require.Contains(t, query, "::date")
	require.Contains(t, query, "::numeric")
	require.Contains(t, query, "::boolean")
	require.Contains(t, query, "::interval")
	require.NotContains(t, query, "BuildJSONPatch")
}

func TestStagedDeleteClassificationUsesRetainedIDAntiJoin(t *testing.T) {
	query, _, err := stagedDeleteCandidates(goqu.Dialect(common.Dialect)).ToSQL()
	require.NoError(t, err)
	require.Contains(t, query, `FROM "retained_target_rows"`)
	require.Contains(t, query, `"live"."submodel_id" = "sm"."id"`)
	require.NotContains(t, query, "LIKE")
}

func TestStagedReconciliationQueriesUseExplicitCrossJoins(t *testing.T) {
	dialect := goqu.Dialect(common.Dialect)
	queries := map[string]*goqu.SelectDataset{
		"metadata":          stagedReconciliationMetadata(dialect),
		"insert candidates": directInsertCandidates(dialect),
		"retained rows":     retainedStagedRows(dialect),
	}
	for name, dataset := range queries {
		t.Run(name, func(t *testing.T) {
			query, _, err := dataset.ToSQL()
			require.NoError(t, err)
			require.Contains(t, query, " CROSS JOIN ")
		})
	}
}

func TestStagedSubmodelVerificationQueryUsesExplicitCrossJoins(t *testing.T) {
	query, _, err := buildStagedSubmodelVerificationQuery(
		"sm",
		&stagedSubmodelTarget{stage: &postgresstaging.Stage{}},
	)
	require.NoError(t, err)
	require.NotContains(t, query, `, "target_submodel"`)
	require.Contains(t, query, `"target_metadata" AS "metadata" CROSS JOIN "target_submodel" AS "target"`)
	require.Contains(t, query, `"target_element_rows" AS "target" CROSS JOIN "target_submodel" AS "sm"`)
	require.Contains(t, query, `"submodel_element" AS "live" CROSS JOIN "target_submodel" AS "sm"`)
}

func TestStagedJSONConstantsDoNotCreateUntypedBindParameters(t *testing.T) {
	dialect := goqu.Dialect(common.Dialect)
	expressions := map[string]any{
		"insert changes": allStagedElementChanges(),
		"semantic reference": currentSemanticReferenceJSON(
			dialect,
			submodelSemanticReferenceTables(),
			goqu.I("target.id"),
		),
	}
	for name, expression := range expressions {
		t.Run(name, func(t *testing.T) {
			query, args, err := dialect.Select(expression).Prepared(true).ToSQL()
			require.NoError(t, err)
			require.Empty(t, args)
			require.Contains(t, query, "jsonb_build_object")
		})
	}
}

func TestContextWithoutFragmentFiltersPreservesUpdateFormula(t *testing.T) {
	allowed := true
	formula := grammar.LogicalExpression{Boolean: &allowed}
	original := &auth.QueryFilter{
		Formula: &formula,
		Filters: auth.FragmentFilters{
			grammar.FragmentStringPattern("$sm#idShort"): {},
		},
	}
	ctx, err := contextWithoutFragmentFilters(auth.WithQueryFilter(t.Context(), original))
	require.NoError(t, err)
	filtered := auth.GetQueryFilter(ctx)
	require.NotNil(t, filtered)
	require.Equal(t, original.Formula, filtered.Formula)
	require.Nil(t, filtered.Filters)
	require.NotNil(t, original.Filters)
}

func readReconciliationJSON(t *testing.T, payload string) types.ISubmodel {
	t.Helper()
	var jsonable map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &jsonable))
	submodel, err := jsonization.SubmodelFromJsonable(jsonable)
	require.NoError(t, err)
	return submodel
}
