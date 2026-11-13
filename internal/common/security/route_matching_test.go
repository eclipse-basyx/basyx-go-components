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
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestCanonicalAuthorizationPathKeepsSeparatorsOfTheRouter(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		requestPath string
		want        string
	}{
		{"/v1/dpps/urn:x%2Felements%2Fy", "/v1/dpps/urn:x%2Felements%2Fy"},
		{"/v1/dpps/urn:x%2felements%2fy", "/v1/dpps/urn:x%2Felements%2Fy"},
		{"/submodels/abc/submodel-elements/List%5B0%5D", "/submodels/abc/submodel-elements/List[0]"},
		{"/submodels/abc/submodel-elements/%41", "/submodels/abc/submodel-elements/A"},
		{"/v1/dpps/a%25b", "/v1/dpps/a%25b"},
		{"/v1/dpps/a%252Fb", "/v1/dpps/a%252Fb"},
		{"/v1/dpps/urn:x/elements/y", "/v1/dpps/urn:x/elements/y"},
		{"/v1/dpps/urn:m\xc3\xbcller%2Felements%2Fy", "/v1/dpps/urn:m\xc3\xbcller%2Felements%2Fy"},
		{"/v1/dpps/urn:x%2Felements%2Fy{|}", "/v1/dpps/urn:x%2Felements%2Fy{|}"},
		{"/submodels/", "/submodels/"},
		{"/", "/"},
	} {
		t.Run(test.requestPath, func(t *testing.T) {
			t.Parallel()
			got, err := canonicalAuthorizationPath(mustParseRequestURL(t, test.requestPath))
			if err != nil {
				t.Fatalf("canonicalAuthorizationPath(%q) error = %v", test.requestPath, err)
			}
			if got != test.want {
				t.Fatalf("canonicalAuthorizationPath(%q) = %q, want %q", test.requestPath, got, test.want)
			}
		})
	}
}

func TestCanonicalAuthorizationPathRejectsAmbiguousPaths(t *testing.T) {
	t.Parallel()
	for _, requestPath := range []string{
		"/submodels/./x",
		"/submodels/x/..",
		"/v1/dpps/x/elements/../..",
		"/v1/dpps/x/elements/%2E%2E",
		"/v1/dpps/x/elements/%2e",
		"/submodels//x",
		"//submodels",
		"/submodels/x%00",
		"/submodels/x%0Ay",
		"/submodels/x%7F",
	} {
		t.Run(requestPath, func(t *testing.T) {
			t.Parallel()
			if got, err := canonicalAuthorizationPath(mustParseRequestURL(t, requestPath)); err == nil {
				t.Fatalf("canonicalAuthorizationPath(%q) = %q, want an error", requestPath, got)
			}
		})
	}
}

func TestRouteWildcardsDoNotSplitEncodedUnits(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/v1/dpps/*F*", "/v1/dpps/a%2Fb", false},
		{"/v1/dpps/*2F*", "/v1/dpps/a%2Fb", false},
		{"/v1/dpps/*25admin*", "/v1/dpps/a%25admin", false},
		{"/v1/dpps/*/elements/*", "/v1/dpps/urn:x%2Felements%2Fy", false},
		{"/v1/dpps/*/elements/*", "/v1/dpps/urn:x%2Fp1/elements/a", true},
		{"/v1/dpps/*", "/v1/dpps/urn:x%2Fp1", true},
		{"/v1/dpps/urn:x%2Fp1", "/v1/dpps/urn:x%2Fp1", true},
		{"/v1/dpps/urn:x%2fp1", "/v1/dpps/urn:x%2Fp1", true},
		{"/v1/dpps/urn:x%2Fp1", "/v1/dpps/urn:x/p1", false},
		{"/v1/dpps/a%25b", "/v1/dpps/a%25b", true},
		{"/v1/dpps/a%2Fb", "/v1/dpps/a%252Fb", false},
		{"/v1/dpps/a%252Fb", "/v1/dpps/a%252Fb", true},
		{"/v1/dpps/a%252Fb", "/v1/dpps/a%2Fb", false},
		{"*", "/v1/dpps/a%2Fb/elements/c", true},
		{"/submodels/*/submodel-elements/*/attachment", "/submodels/abc/submodel-elements/Coll.x%2Fattachment", false},
		{"/submodels/*/submodel-elements/*/attachment", "/submodels/abc/submodel-elements/Coll.x/attachment", true},
	} {
		t.Run(test.pattern+" "+test.path, func(t *testing.T) {
			t.Parallel()
			if got := matchRouteACL(test.pattern, test.path); got != test.want {
				t.Fatalf("matchRouteACL(%q, %q) = %v, want %v", test.pattern, test.path, got, test.want)
			}
		})
	}
}

func TestMappedRouteWildcardsStayWithinOneSegment(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/submodels/*", "/submodels/abc", true},
		{"/submodels/*", "/submodels/a%2Fb", true},
		{"/submodels/*", "/submodels/abc/submodel-elements", false},
		{"/submodels/abc/**", "/submodels/abc/submodel-elements/x", true},
		{"/submodels/abc/submodel-elements/Coll.*", "/submodels/abc/submodel-elements/Coll.x", true},
		{"/submodels/abc/submodel-elements/Coll.*", "/submodels/abc/submodel-elements/Coll.%2F", true},
		{"/submodels/abc/submodel-elements/Coll.*F", "/submodels/abc/submodel-elements/Coll.a%2F", false},
	} {
		t.Run(test.pattern+" "+test.path, func(t *testing.T) {
			t.Parallel()
			if got := matchRouteANT(test.pattern, test.path); got != test.want {
				t.Fatalf("matchRouteANT(%q, %q) = %v, want %v", test.pattern, test.path, got, test.want)
			}
		})
	}
}

func TestRoutePatternValidationRejectsAmbiguousRoutes(t *testing.T) {
	t.Parallel()
	for _, route := range []string{
		"/v1/dpps/%41",
		"/v1/dpps/a%",
		"/v1/dpps/a%2",
		"/v1/dpps/a%G1",
		"/v1/dpps//x",
		"/v1/dpps/./x",
		"/v1/dpps/x/..",
	} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			if _, err := parseSemanticRouteTestModel(route, ""); err == nil {
				t.Fatalf("route %q was accepted", route)
			}
		})
	}
	for _, route := range []string{"/v1/dpps/a%2fb", "/v1/dpps/a%25b", "/v1/dpps/*/", "*", "/"} {
		if _, err := parseSemanticRouteTestModel(route, ""); err != nil {
			t.Fatalf("route %q was rejected: %v", route, err)
		}
	}
}

func TestStoredPolicyRoutesAreValidatedOnLoad(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	policy, err := MaterializeABACPolicy([]byte(routeTestPolicy("/v1/dpps/a%2Fb")), router, "")
	if err != nil {
		t.Fatalf("MaterializeABACPolicy() error = %v", err)
	}
	if _, err := AccessModelFromMaterializedRules(policy.PolicyID, policy.Rules, router, ""); err != nil {
		t.Fatalf("valid stored policy was rejected: %v", err)
	}

	tampered := make([]MaterializedABACRule, len(policy.Rules))
	copy(tampered, policy.Rules)
	tampered[0].MaterializedRuleJSON = bytes.Replace(tampered[0].MaterializedRuleJSON, []byte("a%2Fb"), []byte("a%41b"), 1)
	if bytes.Equal(tampered[0].MaterializedRuleJSON, policy.Rules[0].MaterializedRuleJSON) {
		t.Fatal("test setup did not change the stored route")
	}
	if _, err := AccessModelFromMaterializedRules(policy.PolicyID, tampered, router, ""); err == nil {
		t.Fatal("stored policy with an invalid route was accepted")
	}
}

func TestRoutesWithSurroundingWhitespaceDoNotLoad(t *testing.T) {
	t.Parallel()
	router := chi.NewRouter()
	for _, objectJSON := range []string{
		`{"ROUTE":"/submodels/*/submodel-elements/Coll "}`,
		`{"ROUTE":" /submodels/*/submodel-elements/Coll"}`,
		`{"ROUTE":"/submodels/*/submodel-elements/Coll	"}`,
		`{"ROUTE":{"Route":"/submodels/*/submodel-elements/Coll "}}`,
		`{"ROUTE":{"Route":" /submodels/*/submodel-elements/Coll"}}`,
	} {
		policy := []byte(routeTestPolicyWithObject(objectJSON))
		if _, err := ParseAccessModel(policy, router, ""); err == nil {
			t.Errorf("ParseAccessModel() accepted %s", objectJSON)
		}
		if _, err := MaterializeABACPolicy(policy, router, ""); err == nil {
			t.Errorf("MaterializeABACPolicy() accepted %s", objectJSON)
		}
	}
}

func TestStoredRoutesWithSurroundingWhitespaceDoNotLoad(t *testing.T) {
	t.Parallel()
	router := chi.NewRouter()
	policy, err := MaterializeABACPolicy([]byte(routeTestPolicy("/submodels/*/submodel-elements/Coll")), router, "")
	if err != nil {
		t.Fatalf("MaterializeABACPolicy() error = %v", err)
	}
	stored := make([]MaterializedABACRule, len(policy.Rules))
	copy(stored, policy.Rules)
	stored[0].MaterializedRuleJSON = bytes.Replace(stored[0].MaterializedRuleJSON, []byte("/Coll\""), []byte("/Coll \""), 1)
	if bytes.Equal(stored[0].MaterializedRuleJSON, policy.Rules[0].MaterializedRuleJSON) {
		t.Fatal("test setup did not change the stored route")
	}
	if _, err := AccessModelFromMaterializedRules(policy.PolicyID, stored, router, ""); err == nil {
		t.Fatal("stored route with surrounding whitespace was accepted")
	}
}

func TestABACMiddlewareMatchesTheCanonicalPath(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		route       string
		requestPath string
		want        int
	}{
		{"element reader cannot read a passport whose id contains /elements/", "/v1/dpps/*/elements/*", "/v1/dpps/urn:x%2Felements%2Fy", http.StatusForbidden},
		{"element reader cannot read such a passport when the id is sent with raw non-ASCII bytes", "/v1/dpps/*/elements/*", "/v1/dpps/urn:m\xc3\xbcller%2Felements%2Fy", http.StatusForbidden},
		{"element reader cannot read such a passport when the id is sent with raw reserved bytes", "/v1/dpps/*/elements/*", "/v1/dpps/urn:x%2Felements%2Fy{", http.StatusForbidden},
		{"element reader reads an element of a passport whose id contains a slash", "/v1/dpps/*/elements/*", "/v1/dpps/urn:x%2Fp1/elements/a", http.StatusNoContent},
		{"rule for a/b grants a/b", "/v1/dpps/a%2Fb", "/v1/dpps/a%2Fb", http.StatusNoContent},
		{"rule for a/b does not grant the literal text a%2Fb", "/v1/dpps/a%2Fb", "/v1/dpps/a%252Fb", http.StatusForbidden},
		{"rule for a%b grants a%b", "/v1/dpps/a%25b", "/v1/dpps/a%25b", http.StatusNoContent},
		{"attachment rule does not grant an element whose idShort contains /attachment", "/submodels/*/submodel-elements/*/attachment", "/submodels/abc/submodel-elements/Coll.x%2Fattachment", http.StatusForbidden},
		{"attachment rule grants an attachment", "/submodels/*/submodel-elements/*/attachment", "/submodels/abc/submodel-elements/Coll.x/attachment", http.StatusNoContent},
		{"dot segments are rejected", "*", "/v1/dpps/x/elements/../..", http.StatusBadRequest},
		{"encoded dot segments are rejected", "*", "/v1/dpps/x/elements/%2E%2E", http.StatusBadRequest},
		{"empty segments are rejected", "*", "/v1/dpps//x", http.StatusBadRequest},
		{"control characters are rejected", "*", "/v1/dpps/x%00", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			router := routeTestRouter(t, test.route)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "http://localhost"+test.requestPath, nil)
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("GET %s with ROUTE %q = %d, want %d: %s", test.requestPath, test.route, recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func routeTestRouter(t *testing.T, route string) *chi.Mux {
	t.Helper()
	router := chi.NewRouter()
	model, err := ParseAccessModel([]byte(routeTestPolicy(route)), router, "")
	if err != nil {
		t.Fatalf("ParseAccessModel() error = %v", err)
	}
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ClaimsKey, Claims{"role": "viewer"})))
		})
	}, ABACMiddleware(ABACSettings{Enabled: true, Model: model}))
	registerDPPTestRoutes(router)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	router.Method(http.MethodGet, "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", handler)
	router.Method(http.MethodGet, "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", handler)
	return router
}

func routeTestPolicy(route string) string {
	routeJSON, _ := json.Marshal(route)
	return routeTestPolicyWithObject(`{"ROUTE":` + string(routeJSON) + `}`)
}

func routeTestPolicyWithObject(objectJSON string) string {
	return `{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [{"name":"role","attributes":[{"CLAIM":"role"}]}],
			"DEFOBJECTS": [{"name":"route","objects":[` + objectJSON + `]}],
			"DEFACLS": [{"name":"read","acl":{"USEATTRIBUTES":"role","RIGHTS":["READ"],"ACCESS":"ALLOW"}}],
			"DEFFORMULAS": [{"name":"viewer","formula":{"$eq":[{"$attribute":{"CLAIM":"role"}},{"$strVal":"viewer"}]}}],
			"rules": [{"USEACL":"read","USEOBJECTS":["route"],"USEFORMULA":"viewer"}]
		}
	}`
}

func mustParseRequestURL(t *testing.T, requestPath string) *url.URL {
	t.Helper()
	parsed, err := url.Parse("http://localhost" + requestPath)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", requestPath, err)
	}
	return parsed
}
