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
	"regexp"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	apis "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
	"github.com/go-chi/chi/v5"
)

func TestDeterministicMatchedRuleIDStableForUnchangedRule(t *testing.T) {
	t.Parallel()

	rule := ruleWithFormulaName("always_true")
	first, err := deterministicMatchedRuleID(1, rule)
	if err != nil {
		t.Fatalf("deterministicMatchedRuleID returned error: %v", err)
	}
	second, err := deterministicMatchedRuleID(1, rule)
	if err != nil {
		t.Fatalf("deterministicMatchedRuleID returned error: %v", err)
	}

	if first != second {
		t.Fatalf("expected stable rule id, got %q and %q", first, second)
	}
	if !regexp.MustCompile(`^rule:1:[a-f0-9]{16}$`).MatchString(first) {
		t.Fatalf("unexpected rule id format: %q", first)
	}
}

func TestDeterministicMatchedRuleIDHashChangesWhenRuleContentChanges(t *testing.T) {
	t.Parallel()

	first, err := deterministicMatchedRuleID(1, ruleWithFormulaName("always_true"))
	if err != nil {
		t.Fatalf("deterministicMatchedRuleID returned error: %v", err)
	}
	second, err := deterministicMatchedRuleID(1, ruleWithFormulaName("is_admin"))
	if err != nil {
		t.Fatalf("deterministicMatchedRuleID returned error: %v", err)
	}

	if first == second {
		t.Fatalf("expected different rule ids after rule content changed, got %q", first)
	}
	if ruleIDHashPart(first) == ruleIDHashPart(second) {
		t.Fatalf("expected hash portion to change, got %q and %q", first, second)
	}
}

func TestAuthorizeWithFilterWithOptionsReturnsOrderedMatchedRuleIDs(t *testing.T) {
	t.Parallel()

	model := mustParseAASRegistryAccessModel(t, matchedRuleModelJSON).WithPolicyID("policy-db")

	result := model.AuthorizeWithFilterWithOptions(EvalInput{
		Method: http.MethodGet,
		Path:   "/shell-descriptors",
		Claims: Claims{"sub": "anonymous"},
	}, grammar.DefaultSimplifyOptions())

	if !result.Allowed || result.Reason != DecisionAllow {
		t.Fatalf("expected allow decision, got allowed=%v reason=%s", result.Allowed, result.Reason)
	}
	expected := strings.Join([]string{model.rules[0].id, model.rules[1].id}, ",")
	if result.MatchedRuleID != expected {
		t.Fatalf("expected matched rule ids %q, got %q", expected, result.MatchedRuleID)
	}
	if result.PolicyID != "policy-db" {
		t.Fatalf("expected policy id %q, got %q", "policy-db", result.PolicyID)
	}
}

func TestABACMiddlewareStoresMatchedRuleIDOnAllowDecision(t *testing.T) {
	t.Parallel()

	model := mustParseAASRegistryAccessModel(t, matchedRuleModelJSON).WithPolicyID("policy-db")
	var captured AuthorizationDecision
	handler := ABACMiddleware(ABACSettings{
		Enabled: true,
		Model:   model,
	})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var ok bool
		captured, ok = AuthorizationDecisionFromContext(r.Context())
		if !ok {
			t.Fatalf("expected authorization decision in request context")
		}
	}))

	request := httptest.NewRequest(http.MethodGet, "/shell-descriptors", nil)
	request = request.WithContext(context.WithValue(request.Context(), ClaimsKey, Claims{"sub": "anonymous"}))
	handler.ServeHTTP(httptest.NewRecorder(), request)

	expected := strings.Join([]string{model.rules[0].id, model.rules[1].id}, ",")
	if captured.Result != string(DecisionAllow) {
		t.Fatalf("expected allow result, got %q", captured.Result)
	}
	if captured.MatchedRuleID != expected {
		t.Fatalf("expected matched rule ids %q, got %q", expected, captured.MatchedRuleID)
	}
	if captured.PolicyID != "policy-db" {
		t.Fatalf("expected policy id %q, got %q", "policy-db", captured.PolicyID)
	}
}

func TestMaterializeABACPolicyBuildsCanonicalRowsAndReloadableModel(t *testing.T) {
	t.Parallel()

	router := aasRegistryRouter()
	materialized, err := MaterializeABACPolicy([]byte(matchedRuleModelJSON), router, "")
	if err != nil {
		t.Fatalf("materialize policy failed: %v", err)
	}

	if len(materialized.PolicyID) != 64 || materialized.PolicyID != materialized.ConfiguredPolicyHash {
		t.Fatalf("unexpected policy id/hash: %q %q", materialized.PolicyID, materialized.ConfiguredPolicyHash)
	}
	if len(materialized.RawPolicyHash) != 64 {
		t.Fatalf("expected raw file hash alias, got %q", materialized.RawPolicyHash)
	}
	if len(materialized.Rules) != 2 {
		t.Fatalf("expected two materialized rules, got %d", len(materialized.Rules))
	}
	if !regexp.MustCompile(`^rule:1:[a-f0-9]{16}$`).MatchString(materialized.Rules[0].MatchedRuleID) {
		t.Fatalf("unexpected matched rule id: %q", materialized.Rules[0].MatchedRuleID)
	}

	reloaded, err := AccessModelFromMaterializedRules(materialized.PolicyID, materialized.Rules, router, "")
	if err != nil {
		t.Fatalf("reload model failed: %v", err)
	}
	result := reloaded.AuthorizeWithFilterWithOptions(EvalInput{
		Method: http.MethodGet,
		Path:   "/shell-descriptors",
		Claims: Claims{"sub": "anonymous"},
	}, grammar.DefaultSimplifyOptions())

	if !result.Allowed {
		t.Fatalf("expected reloaded model to allow request, got %s", result.Reason)
	}
	if result.PolicyID != materialized.PolicyID {
		t.Fatalf("expected policy id %q, got %q", materialized.PolicyID, result.PolicyID)
	}
}

func TestMaterializedPolicyHashChangesWhenReferencedDefinitionChanges(t *testing.T) {
	t.Parallel()

	first, err := MaterializeABACPolicy([]byte(matchedRuleModelJSON), aasRegistryRouter(), "")
	if err != nil {
		t.Fatalf("materialize first policy failed: %v", err)
	}
	changed := strings.Replace(matchedRuleModelJSON, `{ "$boolean": true }`, `{ "$boolean": false }`, 1)
	second, err := MaterializeABACPolicy([]byte(changed), aasRegistryRouter(), "")
	if err != nil {
		t.Fatalf("materialize changed policy failed: %v", err)
	}

	if first.MaterializedPolicyHash == second.MaterializedPolicyHash {
		t.Fatalf("expected materialized policy hash to change")
	}
}

func ruleWithFormulaName(name string) grammar.AccessPermissionRule {
	acl := "read"
	objects := []string{"shells"}
	return grammar.AccessPermissionRule{
		USEACL:     &acl,
		USEOBJECTS: objects,
		USEFORMULA: &name,
	}
}

func ruleIDHashPart(id string) string {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return ""
	}
	return parts[2]
}

func mustParseAASRegistryAccessModel(t *testing.T, modelJSON string) *AccessModel {
	t.Helper()

	router := aasRegistryRouter()
	model, err := ParseAccessModel([]byte(modelJSON), router, "")
	if err != nil {
		t.Fatalf("parse model failed: %v", err)
	}
	return model
}

func aasRegistryRouter() *chi.Mux {
	router := chi.NewRouter()
	ctrl := apis.NewAssetAdministrationShellRegistryAPIAPIController(nil, "/*")
	for _, rt := range ctrl.Routes() {
		router.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	return router
}

const matchedRuleModelJSON = `{
  "AllAccessPermissionRules": {
    "DEFATTRIBUTES": [
      { "name": "anonymous", "attributes": [ { "GLOBAL": "ANONYMOUS" } ] }
    ],
    "DEFOBJECTS": [
      { "name": "shells", "objects": [ { "ROUTE": "/shell-descriptors" } ] }
    ],
    "DEFACLS": [
      { "name": "read", "acl": { "USEATTRIBUTES": "anonymous", "RIGHTS": ["READ"], "ACCESS": "ALLOW" } }
    ],
    "DEFFORMULAS": [
      { "name": "always_true", "formula": { "$boolean": true } }
    ],
    "rules": [
      { "USEACL": "read", "USEOBJECTS": ["shells"], "USEFORMULA": "always_true" },
      { "USEACL": "read", "USEOBJECTS": ["shells"], "FORMULA": { "$boolean": true } }
    ]
  }
}`
