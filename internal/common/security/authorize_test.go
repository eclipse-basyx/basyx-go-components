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
