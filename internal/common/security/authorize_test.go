package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/go-chi/chi/v5"
)

func TestABACMiddleware_UnknownRouteReturnsNotFound(t *testing.T) {
	router := api.NewRouter()
	model := &AccessModel{
		apiRouter: router,
		rctx:      api.NewRouteContext(),
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
		rctx:      api.NewRouteContext(),
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
		rctx:      api.NewRouteContext(),
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
