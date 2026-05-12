package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
	ctx := context.WithValue(req.Context(), claimsKey, Claims{"sub": "tester", "scope": ""})
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
	ctx := context.WithValue(req.Context(), claimsKey, Claims{"sub": "tester", "scope": ""})
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
	ctx := context.WithValue(req.Context(), claimsKey, Claims{"sub": "tester", "scope": ""})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
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
