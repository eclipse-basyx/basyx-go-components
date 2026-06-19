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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	api "github.com/go-chi/chi/v5"
)

func TestSecurityMiddleware_RejectsAnonymousGetWhenACLRequiresSubClaim(t *testing.T) {
	router := api.NewRouter()
	model, err := ParseAccessModel([]byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [
				{
					"name": "anonymous",
					"attributes": [
						{ "GLOBAL": "ANONYMOUS" }
					]
				},
				{
					"name": "sub_claim",
					"attributes": [
						{ "CLAIM": "sub" }
					]
				}
			],
			"DEFOBJECTS": [
				{
					"name": "description_api",
					"objects": [
						{ "ROUTE": "/description" }
					]
				},
				{
					"name": "aas_registry_api",
					"objects": [
						{ "ROUTE": "/shell-descriptors" }
					]
				}
			],
			"DEFACLS": [
				{
					"name": "anonymous_access",
					"acl": {
						"USEATTRIBUTES": "anonymous",
						"RIGHTS": [ "READ" ],
						"ACCESS": "ALLOW"
					}
				},
				{
					"name": "read_access",
					"acl": {
						"USEATTRIBUTES": "sub_claim",
						"RIGHTS": [ "READ" ],
						"ACCESS": "ALLOW"
					}
				}
			],
			"DEFFORMULAS": [
				{
					"name": "always_true",
					"formula": { "$boolean": true }
				}
			],
			"rules": [
				{
					"USEACL": "anonymous_access",
					"USEOBJECTS": [ "description_api" ],
					"USEFORMULA": "always_true"
				},
				{
					"USEACL": "read_access",
					"USEOBJECTS": [ "aas_registry_api" ],
					"USEFORMULA": "always_true"
				}
			]
		}
	}`), router, "")
	if err != nil {
		t.Fatalf("ParseAccessModel() error = %v", err)
	}

	oidc := &OIDC{
		verifiers: map[string]issuerVerifier{},
		settings: OIDCSettings{
			AllowAnonymous: true,
		},
	}
	router.Use(
		oidc.Middleware,
		ABACMiddleware(ABACSettings{
			Enabled: true,
			Model:   model,
		}),
	)
	router.Get("/shell-descriptors", func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler must not be called without a token")
	})
	router.Get("/description", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := FromContext(r)["sub"]; ok {
			t.Fatal("anonymous claims must not contain sub")
		}
		w.WriteHeader(http.StatusOK)
	})

	protectedReq := httptest.NewRequest(http.MethodGet, "/shell-descriptors", nil)
	protectedRec := httptest.NewRecorder()

	router.ServeHTTP(protectedRec, protectedReq)

	if protectedRec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d without token, got %d", http.StatusForbidden, protectedRec.Code)
	}

	anonymousReq := httptest.NewRequest(http.MethodGet, "/description", nil)
	anonymousRec := httptest.NewRecorder()

	router.ServeHTTP(anonymousRec, anonymousReq)

	if anonymousRec.Code != http.StatusOK {
		t.Fatalf("expected GLOBAL ANONYMOUS access status %d, got %d", http.StatusOK, anonymousRec.Code)
	}
}

func TestABACMiddleware_UnknownRouteReturnsNotFound(t *testing.T) {
	router := api.NewRouter()
	model := &AccessModel{
		apiRouter: router,
		basePath:  "",
	}

	router.Use(ABACMiddleware(ABACSettings{
		Enabled: true,
		Model:   model,
	}))
	router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	ctx := context.WithValue(req.Context(), ClaimsKey, Claims{"sub": "tester", "scope": ""})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestABACMiddleware_MethodNotAllowedReturnsMethodNotAllowed(t *testing.T) {
	router := api.NewRouter()
	model := &AccessModel{
		apiRouter: router,
		basePath:  "",
	}

	router.Use(ABACMiddleware(ABACSettings{
		Enabled: true,
		Model:   model,
	}))
	router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/description", nil)
	ctx := context.WithValue(req.Context(), ClaimsKey, Claims{"sub": "tester", "scope": ""})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestABACMiddleware_KnownMappedRouteWithoutMatchingRuleReturnsForbidden(t *testing.T) {
	router := api.NewRouter()
	model := &AccessModel{
		apiRouter: router,
		basePath:  "",
	}

	router.Use(ABACMiddleware(ABACSettings{
		Enabled: true,
		Model:   model,
	}))
	router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/description", nil)
	ctx := context.WithValue(req.Context(), ClaimsKey, Claims{"sub": "tester", "scope": ""})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestABACMiddleware_DeniedSensitivePrefixReturnsNotFound(t *testing.T) {
	router := api.NewRouter()
	model := &AccessModel{
		apiRouter: router,
		basePath:  "",
	}

	router.Use(ABACMiddleware(ABACSettings{
		Enabled:                true,
		Model:                  model,
		DenyAsNotFoundPrefixes: []string{"/security/abac"},
	}))
	router.Get("/security/abac/policy-versions", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/security/abac/policy-versions", nil)
	ctx := context.WithValue(req.Context(), ClaimsKey, Claims{"sub": "tester", "scope": ""})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestABACMiddleware_DeniedSensitivePrefixDoesNotHideOtherRoutes(t *testing.T) {
	router := api.NewRouter()
	model := &AccessModel{
		apiRouter: router,
		basePath:  "",
	}

	router.Use(ABACMiddleware(ABACSettings{
		Enabled:                true,
		Model:                  model,
		DenyAsNotFoundPrefixes: []string{"/security/abac"},
	}))
	router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/description", nil)
	ctx := context.WithValue(req.Context(), ClaimsKey, Claims{"sub": "tester", "scope": ""})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestPathMatchesPrefixUsesPathBoundaries(t *testing.T) {
	t.Parallel()

	if !pathMatchesPrefix("/security/abac/policy-versions", "/security/abac") {
		t.Fatal("expected nested management path to match")
	}
	if !pathMatchesPrefix("/api/v3/security/abac/policy-versions", "/api/v3/security/abac") {
		t.Fatal("expected context-path management path to match")
	}
	if pathMatchesPrefix("/security/abacus", "/security/abac") {
		t.Fatal("expected similar prefix without path boundary not to match")
	}
}

func TestABACMiddleware_ModelProviderNilFailsClosed(t *testing.T) {
	handler := ABACMiddleware(ABACSettings{
		Enabled:       true,
		ModelProvider: emptyModelProvider{},
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/description", nil)
	ctx := context.WithValue(req.Context(), ClaimsKey, Claims{"sub": "tester"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected fail-closed status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

type emptyModelProvider struct{}

func (emptyModelProvider) ActiveAccessModel() *AccessModel {
	return nil
}

func TestHasUnrestrictedFormulaForRight_ReturnsTrueForBooleanTrue(t *testing.T) {
	t.Parallel()

	b := true
	ctx := context.WithValue(context.Background(), filterKey, &QueryFilter{
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: {Boolean: &b},
		},
	})

	if !HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumREAD) {
		t.Fatalf("expected READ formula to be unrestricted")
	}
}

func TestHasUnrestrictedFormulaForRight_ReturnsFalseWhenMissingOrFalse(t *testing.T) {
	t.Parallel()

	bFalse := false
	ctx := context.WithValue(context.Background(), filterKey, &QueryFilter{
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD:   {Boolean: &bFalse},
			grammar.RightsEnumCREATE: {},
		},
	})

	if HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumREAD) {
		t.Fatalf("expected READ formula to be restricted")
	}
	if HasUnrestrictedFormulaForRight(ctx, grammar.RightsEnumCREATE) {
		t.Fatalf("expected CREATE formula without boolean literal to be restricted")
	}
	if HasUnrestrictedFormulaForRight(context.Background(), grammar.RightsEnumREAD) {
		t.Fatalf("expected nil query filter context to be restricted")
	}
}

func TestShouldEnforceFormula_AppliesMergedQueryWhenABACDisabled(t *testing.T) {
	t.Parallel()

	ctx := common.ContextWithConfig(context.Background(), &common.Config{})
	queryExpr := boolExpression(true)

	mergedCtx := MergeQueryFilter(ctx, grammar.Query{Condition: &queryExpr})

	shouldEnforce, err := ShouldEnforceFormula(mergedCtx)
	if err != nil {
		t.Fatalf("ShouldEnforceFormula returned error: %v", err)
	}
	if !shouldEnforce {
		t.Fatalf("expected merged user query to be enforced when ABAC is disabled")
	}
}

func TestShouldEnforceFormula_InconsistentQueryFilterErrorDoesNotMentionABACEnabled(t *testing.T) {
	t.Parallel()

	queryExpr := boolExpression(true)
	ctx := common.ContextWithConfig(context.Background(), &common.Config{})
	ctx = WithQueryFilter(ctx, &QueryFilter{Formula: &queryExpr})

	shouldEnforce, err := ShouldEnforceFormula(ctx)
	if err == nil {
		t.Fatalf("expected inconsistent QueryFilter error")
	}
	if !shouldEnforce {
		t.Fatalf("expected fail-closed enforcement decision")
	}
	if strings.Contains(err.Error(), "ABAC is enabled") {
		t.Fatalf("error should describe QueryFilter state, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Formula is set but FormulasByRight is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
