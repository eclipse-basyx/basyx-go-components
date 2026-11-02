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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

const (
	superpathAASID      = "urn:example:aas:product-1"
	superpathSubmodelID = "urn:example:submodel:nameplate-1"
)

func superpathRequestPath() string {
	return "/shells/" + common.EncodeString(superpathAASID) + "/submodels/" + common.EncodeString(superpathSubmodelID)
}

func superpathRule(object map[string]string, formula any, rights ...string) map[string]any {
	rule := environmentRule(object, rights...)
	if formula != nil {
		rule["FORMULA"] = formula
	}
	return rule
}

func fieldFormula(operator, field, value string) map[string]any {
	return map[string]any{operator: []any{map[string]string{"$field": field}, map[string]string{"$strVal": value}}}
}

func publicSubmodelsRule() map[string]any {
	return superpathRule(map[string]string{"IDENTIFIABLE": `$sm("*")`}, fieldFormula("$regex", "$sm#id", "^urn:example:submodel:.+$"), "READ", "UPDATE", "DELETE")
}

func serveSuperpath(t *testing.T, method string, resolver ReBACResolver, rules ...map[string]any) context.Context {
	t.Helper()
	router := chi.NewRouter()
	noop := func(http.ResponseWriter, *http.Request) {}
	router.Get("/shells/{aasIdentifier}", noop)
	router.Get("/shells/{aasIdentifier}/submodels/{submodelIdentifier}", noop)
	router.Put("/shells/{aasIdentifier}/submodels/{submodelIdentifier}", noop)
	router.Delete("/shells/{aasIdentifier}/submodels/{submodelIdentifier}", noop)
	payload, err := json.Marshal(map[string]any{"AllAccessPermissionRules": map[string]any{"rules": rules}})
	require.NoError(t, err)
	model, err := ParseAccessModel(payload, router, "")
	require.NoError(t, err)
	cfg := &common.Config{}
	cfg.ABAC.Enabled = true
	req := httptest.NewRequest(method, superpathRequestPath(), nil)
	ctx := common.ContextWithConfig(req.Context(), cfg)
	ctx = context.WithValue(ctx, ClaimsKey, Claims{"role": "test", "sub": "alice", "iss": "issuer"})
	ctx = context.WithValue(ctx, authenticatedKey, true)
	var result context.Context
	handler := ABACMiddleware(ABACSettings{Enabled: true, Model: model, ReBAC: resolver})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result = r.Context()
		w.WriteHeader(http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req.WithContext(ctx))
	require.Equal(t, http.StatusOK, response.Code)
	return result
}

func aasFormulaSQL(ctx context.Context, t *testing.T) string {
	t.Helper()
	return formulaSQLForRoot(ctx, t, grammar.CollectorRootAAS, "aas", "aas")
}

func TestSuperpathShellContextUsesShellRulesOnly(t *testing.T) {
	openShells := superpathRule(map[string]string{"IDENTIFIABLE": `$aas("*")`}, nil, "READ")
	ctx := serveSuperpath(t, http.MethodGet, nil, openShells, publicSubmodelsRule())
	shellCtx, err := SuperpathShellContext(ctx, superpathAASID, grammar.RightsEnumREAD)
	require.NoError(t, err)
	require.True(t, HasUnrestrictedFormulaForRight(shellCtx, grammar.RightsEnumREAD))
	require.NotContains(t, aasFormulaSQL(shellCtx, t), "submodel")

	productShells := superpathRule(map[string]string{"IDENTIFIABLE": `$aas("*")`}, fieldFormula("$eq", "$aas#idShort", "ExampleAAS"), "READ")
	ctx = serveSuperpath(t, http.MethodGet, nil, productShells, publicSubmodelsRule())
	shellCtx, err = SuperpathShellContext(ctx, superpathAASID, grammar.RightsEnumREAD)
	require.NoError(t, err)
	sql := aasFormulaSQL(shellCtx, t)
	require.Contains(t, sql, "ExampleAAS")
	require.NotContains(t, sql, "urn:example:submodel")
	require.Nil(t, AuthorizedQueryFromContext(shellCtx))
}

func TestSuperpathShellContextRequiresShellGrantForRight(t *testing.T) {
	for _, test := range []struct {
		name    string
		rule    map[string]any
		right   grammar.RightsEnum
		allowed bool
	}{
		{"no shell rule", nil, grammar.RightsEnumREAD, false},
		{"other shell", superpathRule(map[string]string{"IDENTIFIABLE": `$aas("urn:other")`}, nil, "READ"), grammar.RightsEnumREAD, false},
		{"concrete shell", superpathRule(map[string]string{"IDENTIFIABLE": `$aas("` + superpathAASID + `")`}, nil, "READ"), grammar.RightsEnumREAD, true},
		{"shell route", superpathRule(map[string]string{"ROUTE": "/shells/*"}, nil, "READ"), grammar.RightsEnumREAD, true},
		{"read does not grant update", superpathRule(map[string]string{"IDENTIFIABLE": `$aas("*")`}, nil, "READ"), grammar.RightsEnumUPDATE, false},
		{"update", superpathRule(map[string]string{"IDENTIFIABLE": `$aas("*")`}, nil, "UPDATE"), grammar.RightsEnumUPDATE, true},
		{"superpath route is no shell grant", superpathRule(map[string]string{"ROUTE": "/shells/*/submodels/*"}, nil, "READ"), grammar.RightsEnumREAD, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rules := []map[string]any{publicSubmodelsRule()}
			if test.rule != nil {
				rules = append(rules, test.rule)
			}
			ctx := serveSuperpath(t, http.MethodGet, nil, rules...)
			shellCtx, err := SuperpathShellContext(ctx, superpathAASID, test.right)
			if !test.allowed {
				require.True(t, common.IsErrDenied(err), "expected denial, got %v", err)
				return
			}
			require.NoError(t, err)
			require.True(t, HasUnrestrictedFormulaForRight(shellCtx, test.right))
		})
	}
}

func TestSuperpathShellContextKeepsReBACShellGrant(t *testing.T) {
	grants := NewReBACGrantSet(grammar.RightsEnumREAD)
	require.NoError(t, grants.AllowResources(SemanticResourceAAS, []string{grantedAASUUID}, grammar.RightsEnumREAD))
	resolver := &fakeReBACResolver{covered: true, grants: grants}
	ctx := serveSuperpath(t, http.MethodGet, resolver, publicSubmodelsRule())
	shellCtx, err := SuperpathShellContext(ctx, superpathAASID, grammar.RightsEnumREAD)
	require.NoError(t, err)
	sql := aasFormulaSQL(shellCtx, t)
	require.Contains(t, sql, grantedAASUUID)
	require.NotContains(t, sql, "urn:example:submodel")
}

func TestSuperpathShellContextPreservesInternalAndDisabledContexts(t *testing.T) {
	cfg := &common.Config{}
	ctx := common.ContextWithConfig(t.Context(), cfg)
	ctx = context.WithValue(ctx, ClaimsKey, Claims{"role": "test"})
	unchanged, err := SuperpathShellContext(ctx, superpathAASID, grammar.RightsEnumREAD)
	require.NoError(t, err)
	require.Equal(t, ctx, unchanged)
	cfg.ABAC.Enabled = true
	_, err = SuperpathShellContext(ctx, superpathAASID, grammar.RightsEnumREAD)
	require.True(t, common.IsErrDenied(err))
	internal := common.ContextWithConfig(t.Context(), cfg)
	unchanged, err = SuperpathShellContext(internal, superpathAASID, grammar.RightsEnumREAD)
	require.NoError(t, err)
	require.Equal(t, internal, unchanged)
}
