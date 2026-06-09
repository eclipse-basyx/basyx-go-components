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

	model := mustParseAASRegistryAccessModel(t, matchedRuleModelJSON)

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
}

func TestABACMiddlewareStoresMatchedRuleIDOnAllowDecision(t *testing.T) {
	t.Parallel()

	model := mustParseAASRegistryAccessModel(t, matchedRuleModelJSON)
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

	router := chi.NewRouter()
	ctrl := apis.NewAssetAdministrationShellRegistryAPIAPIController(nil, "/*")
	for _, rt := range ctrl.Routes() {
		router.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	model, err := ParseAccessModel([]byte(modelJSON), router, "")
	if err != nil {
		t.Fatalf("parse model failed: %v", err)
	}
	return model
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
