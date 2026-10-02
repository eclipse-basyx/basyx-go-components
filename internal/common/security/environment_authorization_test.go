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

func environmentRule(object map[string]string, rights ...string) map[string]any {
	return map[string]any{
		"ACL":     map[string]any{"ATTRIBUTES": []any{map[string]string{"CLAIM": "role"}}, "RIGHTS": rights, "ACCESS": "ALLOW"},
		"OBJECTS": []any{object}, "FORMULA": map[string]bool{"$boolean": true},
	}
}

func serveEnvironment(t *testing.T, method, path, base string, resolver ReBACResolver, rules ...map[string]any) (context.Context, int) {
	t.Helper()
	router := chi.NewRouter()
	router.Get("/serialization", func(http.ResponseWriter, *http.Request) {})
	router.Post("/upload", func(http.ResponseWriter, *http.Request) {})
	payload, err := json.Marshal(map[string]any{"AllAccessPermissionRules": map[string]any{"rules": rules}})
	require.NoError(t, err)
	model, err := ParseAccessModel(payload, router, base)
	require.NoError(t, err)
	cfg := &common.Config{}
	cfg.ABAC.Enabled = true
	req := httptest.NewRequest(method, base+path, nil)
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
	return result, response.Code
}

func TestEnvironmentIdentifiableGrantsStayWithinResourceType(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path, right := "/serialization", "READ"
		if method == http.MethodPost {
			path, right = "/upload", "CREATE"
		}
		for _, allowed := range []SemanticResourceKind{SemanticResourceAAS, SemanticResourceSM, SemanticResourceCD} {
			for _, identifier := range []string{"*", "urn:allowed"} {
				t.Run(method+"/"+string(allowed)+"/"+identifier, func(t *testing.T) {
					object := map[string]string{"IDENTIFIABLE": "$" + string(allowed) + "(\"" + identifier + "\")"}
					ctx, status := serveEnvironment(t, method, path, "/api", nil, environmentRule(object, right))
					require.Equal(t, http.StatusOK, status)
					require.True(t, isFalseFormula(GetQueryFilter(ctx).Formula), "entry permission is not unrestricted data access")
					for _, target := range []struct {
						kind         SemanticResourceKind
						root         grammar.CollectorRoot
						table, alias string
					}{
						{SemanticResourceAAS, grammar.CollectorRootAAS, "aas", "aas"},
						{SemanticResourceSM, grammar.CollectorRootSM, "submodel", "submodel"},
						{SemanticResourceCD, grammar.CollectorRootCD, "concept_description", "concept_description"},
					} {
						bound, err := EnvironmentResourceContext(ctx, target.kind)
						require.NoError(t, err)
						bound = SelectFormulaForRight(bound, grammar.RightsEnum(right))
						sql := formulaSQLForRoot(bound, t, target.root, target.table, target.alias)
						if target.kind != allowed {
							require.Contains(t, sql, "FALSE")
							continue
						}
						if identifier == "*" {
							require.True(t, HasUnrestrictedFormulaForRight(bound, grammar.RightsEnum(right)))
						} else {
							require.Contains(t, sql, identifier)
						}
						if method == http.MethodPost {
							update := SelectFormulaForRight(bound, grammar.RightsEnumUPDATE)
							require.True(t, isFalseFormula(GetQueryFilter(update).Formula))
						}
					}
				})
			}
		}
	}
}

func TestEnvironmentRequiresCorrectRightsObjectsAndClaims(t *testing.T) {
	for _, test := range []struct{ method, path, kind, object, right string }{
		{"POST", "/upload", "IDENTIFIABLE", `$sm("*")`, "READ"},
		{"GET", "/serialization", "IDENTIFIABLE", `$sm("*")`, "UPDATE"},
		{"GET", "/serialization", "DESCRIPTOR", `$smdesc("*")`, "READ"},
		{"GET", "/serialization", "ROUTE", "/submodels", "READ"},
	} {
		_, status := serveEnvironment(t, test.method, test.path, "", nil, environmentRule(map[string]string{test.kind: test.object}, test.right))
		require.Equal(t, http.StatusForbidden, status, test)
	}
	rule := environmentRule(map[string]string{"IDENTIFIABLE": `$sm("*")`}, "READ")
	rule["FORMULA"] = map[string]any{"$eq": []any{map[string]any{"$attribute": map[string]string{"CLAIM": "role"}}, map[string]string{"$strVal": "other"}}}
	_, status := serveEnvironment(t, "GET", "/serialization", "", nil, rule)
	require.Equal(t, http.StatusForbidden, status)
}

func TestEnvironmentRouteGrantRemainsAnAlternative(t *testing.T) {
	rule := environmentRule(map[string]string{"IDENTIFIABLE": `$sm("urn:only")`}, "READ")
	rule["OBJECTS"] = []any{map[string]string{"IDENTIFIABLE": `$sm("urn:only")`}, map[string]string{"ROUTE": "/serialization"}}
	ctx, status := serveEnvironment(t, "GET", "/serialization", "", nil, rule)
	require.Equal(t, http.StatusOK, status)
	for _, target := range []SemanticResourceKind{SemanticResourceAAS, SemanticResourceSM, SemanticResourceCD} {
		bound, err := EnvironmentResourceContext(ctx, target)
		require.NoError(t, err)
		require.True(t, HasUnrestrictedFormulaForRight(bound, grammar.RightsEnumREAD))
	}
}

func TestEnvironmentWildcardDoesNotSuppressOtherReBACGrants(t *testing.T) {
	grants := NewReBACGrantSet(grammar.RightsEnumREAD)
	require.NoError(t, grants.AllowResources(SemanticResourceAAS, []string{grantedAASUUID}, grammar.RightsEnumREAD))
	resolver := &fakeReBACResolver{covered: true, grants: grants}
	ctx, status := serveEnvironment(t, "GET", "/serialization", "", resolver, environmentRule(map[string]string{"IDENTIFIABLE": `$sm("*")`}, "READ"))
	require.Equal(t, http.StatusOK, status)
	require.EqualValues(t, 1, resolver.calls.Load())
	aasCtx, err := EnvironmentResourceContext(ctx, SemanticResourceAAS)
	require.NoError(t, err)
	require.Contains(t, formulaSQLForRoot(aasCtx, t, grammar.CollectorRootAAS, "aas", "aas"), grantedAASUUID)
	cdCtx, err := EnvironmentResourceContext(ctx, SemanticResourceCD)
	require.NoError(t, err)
	cdSQL := formulaSQLForRoot(cdCtx, t, grammar.CollectorRootCD, "concept_description", "concept_description")
	require.Contains(t, cdSQL, "FALSE")
	require.NotContains(t, cdSQL, grantedAASUUID)
	smCtx, err := EnvironmentResourceContext(ctx, SemanticResourceSM)
	require.NoError(t, err)
	require.True(t, HasUnrestrictedFormulaForRight(smCtx, grammar.RightsEnumREAD))
}

func TestEnvironmentBindingReplacesStaleViewsAndKeepsRightsIndependent(t *testing.T) {
	create := environmentRule(map[string]string{"IDENTIFIABLE": `$sm("created")`}, "CREATE")
	update := environmentRule(map[string]string{"IDENTIFIABLE": `$sm("updated")`}, "UPDATE")
	ctx, status := serveEnvironment(t, "POST", "/upload", "", nil, create, update)
	require.Equal(t, http.StatusOK, status)
	smCtx, err := EnvironmentResourceContext(ctx, SemanticResourceSM)
	require.NoError(t, err)
	for right, id := range map[grammar.RightsEnum]string{grammar.RightsEnumCREATE: "created", grammar.RightsEnumUPDATE: "updated"} {
		selected := SelectFormulaForRight(smCtx, right)
		sql := formulaSQLForRoot(selected, t, grammar.CollectorRootSM, "submodel", "submodel")
		require.Contains(t, sql, id)
		if right == grammar.RightsEnumCREATE {
			require.NotContains(t, sql, "updated")
		} else {
			require.NotContains(t, sql, "created")
		}
	}
	stale, err := WithAuthorizedQuery(SelectFormulaForRight(smCtx, grammar.RightsEnumCREATE), SemanticResourceSM, grammar.Query{})
	require.NoError(t, err)
	bound, err := EnvironmentResourceContext(stale, SemanticResourceAAS)
	require.NoError(t, err)
	require.Nil(t, AuthorizedQueryFromContext(bound))
	_, selected := SelectedFormulaRight(bound)
	require.False(t, selected)
	rebuilt, err := WithAuthorizedQuery(bound, SemanticResourceAAS, grammar.Query{})
	require.NoError(t, err)
	require.Contains(t, formulaSQLForRoot(rebuilt, t, grammar.CollectorRootAAS, "aas", "aas"), "FALSE")
	GetQueryFilter(smCtx).FormulasByRight[grammar.RightsEnumUPDATE] = boolExpression(true)
	again, err := EnvironmentResourceContext(ctx, SemanticResourceSM)
	require.NoError(t, err)
	require.False(t, HasUnrestrictedFormulaForRight(again, grammar.RightsEnumUPDATE))
}

func TestEnvironmentBindingPreservesInternalAndDisabledContexts(t *testing.T) {
	cfg := &common.Config{}
	ctx := common.ContextWithConfig(t.Context(), cfg)
	ctx = context.WithValue(ctx, ClaimsKey, Claims{"role": "test"})
	unchanged, err := EnvironmentResourceContext(ctx, SemanticResourceSM)
	require.NoError(t, err)
	require.Equal(t, ctx, unchanged)
	cfg.ABAC.Enabled = true
	_, err = EnvironmentResourceContext(ctx, SemanticResourceSM)
	require.Error(t, err)
	startup := common.ContextWithConfig(t.Context(), cfg)
	unchanged, err = EnvironmentResourceContext(startup, SemanticResourceSM)
	require.NoError(t, err)
	require.Equal(t, startup, unchanged)
}

func TestEnvironmentAdapterOnlyPreparesRegisteredAggregateRoutes(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/serialization", func(http.ResponseWriter, *http.Request) {})
	router.Post("/upload", func(http.ResponseWriter, *http.Request) {})
	router.Get("/submodels", func(http.ResponseWriter, *http.Request) {})
	model := &AccessModel{apiRouter: router, basePath: "/api"}
	session := newAuthorizationSession(model, nil, nil, grammar.DefaultSimplifyOptions())
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/serialization"}, {http.MethodPost, "/upload"},
	} {
		path := "/api" + request.path
		aggregate := session.evaluateEnvironment(request.method, path, path)
		require.NotNil(t, aggregate)
		alternatives, mapped, found := model.mapMethodAndPathToRights(EvalInput{Method: request.method, Path: path})
		require.True(t, mapped && found)
		require.Equal(t, collectRelevantRights(alternatives), aggregate.rights)
		require.Len(t, aggregate.evaluations, 3)
	}
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/api/upload"}, {http.MethodPost, "/api/serialization"},
		{http.MethodGet, "/api/submodels"}, {http.MethodGet, "/api/serialization/"},
		{http.MethodGet, "/serialization"},
	} {
		require.Nil(t, session.evaluateEnvironment(request.method, request.path, request.path), request)
	}
	model.apiRouter = chi.NewRouter()
	require.Nil(t, session.evaluateEnvironment(http.MethodGet, "/api/serialization", "/api/serialization"))
}
