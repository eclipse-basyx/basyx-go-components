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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	api "github.com/go-chi/chi/v5"
)

const testSubmodelRoute = "/submodels/{submodelIdentifier}"

type fakeReBACResolver struct {
	grants     *ReBACGrantSet
	err        error
	covered    bool
	management string
	calls      atomic.Int32
	lastRoute  ReBACRoute
	lastPend   []grammar.RightsEnum
}

func (f *fakeReBACResolver) Covers(ReBACRoute) bool { return f.covered }

func (f *fakeReBACResolver) IsManagementRoute(route ReBACRoute) bool {
	return f.management != "" && route.Pattern == f.management
}

func (f *fakeReBACResolver) Resolve(_ context.Context, request ReBACRequest) (*ReBACGrantSet, error) {
	f.calls.Add(1)
	f.lastRoute = request.Route
	f.lastPend = request.PendingRights
	return f.grants, f.err
}

type reBACMiddlewareResult struct {
	status  int
	body    string
	called  bool
	context context.Context
}

func serveWithReBAC(t *testing.T, settings ABACSettings, method string, target string, authenticated bool) reBACMiddlewareResult {
	t.Helper()
	router := api.NewRouter()
	if settings.Model == nil {
		settings.Model = &AccessModel{apiRouter: router}
	} else {
		settings.Model.apiRouter = router
	}
	settings.Enabled = true
	settings.DenyAsNotFoundPrefixes = []string{"/security/abac"}
	router.Use(ABACMiddleware(settings))
	result := reBACMiddlewareResult{}
	handler := func(w http.ResponseWriter, r *http.Request) {
		result.called = true
		result.context = r.Context()
		w.WriteHeader(http.StatusOK)
	}
	router.Get(testSubmodelRoute, handler)
	router.Get("/submodels/{submodelIdentifier}/$history", handler)
	router.Get("/security/abac/policy-versions", handler)
	router.Get("/security/rebac/status", handler)

	req := httptest.NewRequest(method, target, nil)
	claims := Claims{}
	if authenticated {
		claims = Claims{"iss": "https://issuer.example", "sub": "alice"}
	}
	ctx := context.WithValue(req.Context(), ClaimsKey, claims)
	ctx = context.WithValue(ctx, authenticatedKey, authenticated)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	result.status = rec.Code
	result.body = rec.Body.String()
	return result
}

func submodelReadGrants(t *testing.T) *ReBACGrantSet {
	t.Helper()
	return mustGrantSet(t, func(set *ReBACGrantSet) error {
		return set.AllowResources(SemanticResourceSM, []string{grantedSubmodelUUID}, grammar.RightsEnumREAD)
	}, grammar.RightsEnumREAD)
}

func TestReBACDenialLooksExactlyLikeABACDenial(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"/submodels/c20", "/security/abac/policy-versions"} {
		withoutReBAC := serveWithReBAC(t, ABACSettings{}, http.MethodGet, target, true)
		resolver := &fakeReBACResolver{covered: true, grants: NewReBACGrantSet(grammar.RightsEnumREAD)}
		withReBAC := serveWithReBAC(t, ABACSettings{ReBAC: resolver}, http.MethodGet, target, true)
		if withReBAC.called || withoutReBAC.called {
			t.Fatalf("%s: denied request reached the handler", target)
		}
		if withReBAC.status != withoutReBAC.status {
			t.Fatalf("%s: ReBAC deny status %d differs from ABAC deny status %d", target, withReBAC.status, withoutReBAC.status)
		}
		if withReBAC.body != withoutReBAC.body {
			t.Fatalf("%s: ReBAC deny body differs:\n%s\n%s", target, withReBAC.body, withoutReBAC.body)
		}
	}
}

func TestReBACIsNeverConsultedForAnonymousUnknownOrUncoveredRoutes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		method        string
		target        string
		authenticated bool
		covered       bool
		wantStatus    int
	}{
		{name: "anonymous", method: http.MethodGet, target: "/submodels/c20", covered: true, wantStatus: http.StatusForbidden},
		{name: "route not found", method: http.MethodGet, target: "/unknown", authenticated: true, covered: true, wantStatus: http.StatusNotFound},
		{name: "method not allowed", method: http.MethodDelete, target: "/submodels/c20", authenticated: true, covered: true, wantStatus: http.StatusMethodNotAllowed},
		{name: "uncovered history route", method: http.MethodGet, target: "/submodels/c20/$history", authenticated: true, wantStatus: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			resolver := &fakeReBACResolver{covered: test.covered, grants: submodelReadGrants(t)}
			result := serveWithReBAC(t, ABACSettings{ReBAC: resolver}, test.method, test.target, test.authenticated)
			if resolver.calls.Load() != 0 {
				t.Fatal("ReBAC must not be consulted")
			}
			if result.called || result.status != test.wantStatus {
				t.Fatalf("expected status %d without handler, got %d (called=%v)", test.wantStatus, result.status, result.called)
			}
		})
	}
}

func TestReBACUnavailableWhenNeededReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	resolver := &fakeReBACResolver{covered: true, err: errors.New("openfga timeout")}
	result := serveWithReBAC(t, ABACSettings{ReBAC: resolver}, http.MethodGet, "/submodels/c20", true)
	if result.called || result.status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without handler, got %d (called=%v)", result.status, result.called)
	}
}

func TestReBACIsSkippedWhenABACAllowsUnconditionally(t *testing.T) {
	t.Parallel()

	model, err := ParseAccessModel([]byte(`{"AllAccessPermissionRules":{"rules":[{
		"ACL":{"ATTRIBUTES":[{"CLAIM":"sub"}],"RIGHTS":["READ"],"ACCESS":"ALLOW"},
		"OBJECTS":[{"ROUTE":"/submodels/*"}],
		"FORMULA":{"$boolean":true}
	}]}}`), api.NewRouter(), "")
	if err != nil {
		t.Fatalf("parse model: %v", err)
	}
	resolver := &fakeReBACResolver{covered: true, err: errors.New("openfga down")}
	result := serveWithReBAC(t, ABACSettings{Model: model, ReBAC: resolver}, http.MethodGet, "/submodels/c20", true)
	if resolver.calls.Load() != 0 {
		t.Fatal("OpenFGA must not be consulted when ABAC allows unconditionally")
	}
	if !result.called || result.status != http.StatusOK {
		t.Fatalf("ABAC allow must be unaffected by an OpenFGA outage, got %d", result.status)
	}
	if ReBACGrantsFromContext(result.context) != nil {
		t.Fatal("ABAC-only allow must not carry ReBAC grants")
	}
}

func TestReBACGrantTurnsABACDenyIntoFailClosedAllow(t *testing.T) {
	t.Parallel()

	resolver := &fakeReBACResolver{covered: true, grants: submodelReadGrants(t)}
	result := serveWithReBAC(t, ABACSettings{ReBAC: resolver}, http.MethodGet, "/submodels/c20", true)
	if !result.called {
		t.Fatalf("ReBAC-granted request must reach the handler, got %d", result.status)
	}
	if resolver.lastRoute.Pattern != testSubmodelRoute || resolver.lastRoute.Params["submodelIdentifier"] != "c20" {
		t.Fatalf("resolver received wrong route: %#v", resolver.lastRoute)
	}
	if len(resolver.lastPend) != 1 || resolver.lastPend[0] != grammar.RightsEnumREAD {
		t.Fatalf("resolver received wrong pending rights: %v", resolver.lastPend)
	}
	queryFilter := GetQueryFilter(result.context)
	if queryFilter == nil || queryFilter.Formula == nil || queryFilter.Formula.Boolean == nil || *queryFilter.Formula.Boolean {
		t.Fatalf("ReBAC-granted request must keep a fail-closed ABAC filter: %#v", queryFilter)
	}
	if ReBACGrantsFromContext(result.context).IsEmpty() {
		t.Fatal("ReBAC grants must be published")
	}
	decision, ok := AuthorizationDecisionFromContext(result.context)
	if !ok || decision.MatchedRuleID != ReBACDecisionRuleID {
		t.Fatalf("ReBAC decision must be recorded for audit: %#v", decision)
	}
	session := AuthorizationSessionFromContext(result.context)
	if session == nil || session.outerAccess.decision != AccessViewRestricted {
		t.Fatalf("authorized-query outer view must be restricted, not denied or unrestricted")
	}
}

func TestReBACManagementRoutesBypassABACEvaluation(t *testing.T) {
	t.Parallel()

	resolver := &fakeReBACResolver{management: "/security/rebac/status"}
	result := serveWithReBAC(t, ABACSettings{ReBAC: resolver}, http.MethodGet, "/security/rebac/status", false)
	if !result.called {
		t.Fatalf("management routes authorize in their handlers, got %d", result.status)
	}
	if GetQueryFilter(result.context) != nil || ReBACGrantsFromContext(result.context) != nil {
		t.Fatal("management routes must not receive data authorization")
	}
	if route, ok := ReBACRouteFromContext(result.context); !ok || route.Pattern != "/security/rebac/status" {
		t.Fatalf("management routes must receive their matched route: %#v", route)
	}

	withoutReBAC := serveWithReBAC(t, ABACSettings{}, http.MethodGet, "/security/rebac/status", true)
	if withoutReBAC.called {
		t.Fatal("management routes must not bypass ABAC when ReBAC is disabled")
	}
}
