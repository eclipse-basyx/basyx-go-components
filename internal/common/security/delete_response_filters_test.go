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

package auth

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestDeleteIgnoresResponseFiltersOnEveryMappedRoute(t *testing.T) {
	model := deleteResponseFilterModel(t, true, "READ", "DELETE")
	parameter := regexp.MustCompile(`\{[^{}]+\}`)
	for _, route := range MappedRoutes() {
		if route.Method != http.MethodDelete {
			continue
		}
		t.Run(route.Pattern, func(t *testing.T) {
			result := model.AuthorizeWithFilterWithOptions(EvalInput{
				Method: http.MethodDelete,
				Path:   parameter.ReplaceAllString(route.Pattern, "resource"),
				Claims: Claims{"role": "test"},
			}, grammar.DefaultSimplifyOptions())
			require.True(t, result.Allowed, result.Reason)
			require.NotNil(t, result.QueryFilter)
			require.Empty(t, result.QueryFilter.Filters)
			assertFormulaByRightBoolean(t, result.QueryFilter, grammar.RightsEnumDELETE, true)
			require.Empty(t, reBACPendingRights(result, []grammar.RightsEnum{grammar.RightsEnumDELETE}))
			for _, alternative := range result.alternatives {
				require.Empty(t, alternative.filters)
			}
		})
	}
}

func TestDeleteStillRequiresItsRightAndFormula(t *testing.T) {
	for _, test := range []struct {
		name    string
		formula bool
		rights  []string
	}{
		{"formula denies", false, []string{"READ", "DELETE"}},
		{"right missing", true, []string{"READ"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := deleteResponseFilterModel(t, test.formula, test.rights...)
			allowed, _, _ := model.AuthorizeWithFilter(EvalInput{
				Method: http.MethodDelete,
				Path:   "/submodels/resource",
				Claims: Claims{"role": "test"},
			})
			require.False(t, allowed)
		})
	}
}

func TestDeleteDoesNotRemoveReadFiltersFromTheSameRule(t *testing.T) {
	model := deleteResponseFilterModel(t, true, "READ", "DELETE")
	for _, method := range []string{http.MethodDelete, http.MethodGet, http.MethodDelete, http.MethodGet} {
		allowed, _, filter := model.AuthorizeWithFilter(EvalInput{
			Method: method,
			Path:   "/submodels/resource",
			Claims: Claims{"role": "test"},
		})
		require.True(t, allowed)
		require.NotNil(t, filter)
		require.Equal(t, method == http.MethodGet, len(filter.Filters) > 0)
	}
}

func TestSuperpathDeleteStillRejectsForeignFormulaGrants(t *testing.T) {
	shells := superpathRule(map[string]string{"IDENTIFIABLE": `$aas("*")`}, nil, "UPDATE")
	route := superpathRule(map[string]string{"ROUTE": "/shells/*"}, fieldFormula("$eq", "$aas#id", superpathAASID), "DELETE")
	ctx, status := serveSuperpathRoute(t, http.MethodDelete, superpathRequestPath(), "", nil, shells, route)
	require.Equal(t, http.StatusForbidden, status)
	require.Nil(t, ctx)
}

func TestSuperpathDeleteOmitsMasksFromDerivedAuthorizations(t *testing.T) {
	shells := superpathRule(map[string]string{"IDENTIFIABLE": `$aas("*")`}, fieldFormula("$eq", "$aas#id", superpathAASID), "READ", "UPDATE")
	shells["FILTER"] = map[string]any{"FRAGMENT": "$aas#idShort", "CONDITION": fieldFormula("$eq", "$sm#id", superpathSubmodelID)}
	submodels := publicSubmodelsRule()
	submodels["FILTER"] = map[string]any{"FRAGMENT": "$sm#idShort", "CONDITION": fieldFormula("$eq", "$aas#id", superpathAASID)}
	ctx := serveSuperpath(t, http.MethodDelete, nil, shells, submodels)
	require.Empty(t, GetQueryFilter(ctx).Filters)
	shellCtx, err := SuperpathShellContext(ctx, superpathAASID, grammar.RightsEnumUPDATE)
	require.NoError(t, err)
	require.Empty(t, GetQueryFilter(shellCtx).Filters)
	require.Contains(t, aasFormulaSQL(shellCtx, t), superpathAASID)
	require.NotContains(t, formulaSQLForRoot(ctx, t, grammar.CollectorRootSM, "submodel", "submodel"), `"aas"`)
	session := AuthorizationSessionFromContext(shellCtx)
	view := session.semanticReadView(SemanticResourceSM, SemanticResourceSM)
	require.NotEmpty(t, view.alternatives)
	for _, alternative := range view.alternatives {
		require.Empty(t, alternative.filters)
	}
	read := session.evaluate(http.MethodGet, "/shells/"+common.EncodeString(superpathAASID))
	require.True(t, read.Allowed)
	require.Empty(t, read.QueryFilter.Filters)
}

func deleteResponseFilterModel(t *testing.T, formula bool, rights ...string) *AccessModel {
	t.Helper()
	router := chi.NewRouter()
	for _, route := range MappedRoutes() {
		if route.Method == http.MethodDelete {
			router.Method(http.MethodDelete, route.Pattern, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			router.Method(http.MethodGet, route.Pattern, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		}
	}
	rule := superpathRule(map[string]string{"ROUTE": "/*"}, map[string]bool{"$boolean": formula}, rights...)
	rule["FILTERLIST"] = []any{
		map[string]any{"FRAGMENT": "$sm#idShort", "CONDITION": fieldFormula("$eq", "$aas#id", superpathAASID)},
		map[string]any{"FRAGMENT": "$aas#idShort", "CONDITION": map[string]bool{"$boolean": false}},
	}
	payload, err := json.Marshal(map[string]any{"AllAccessPermissionRules": map[string]any{"rules": []any{rule}}})
	require.NoError(t, err)
	model, err := ParseAccessModel(payload, router, "")
	require.NoError(t, err)
	return model
}
