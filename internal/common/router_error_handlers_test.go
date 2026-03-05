package common

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestAddDefaultRouterErrorHandlers_NotFound(t *testing.T) {
	router := chi.NewRouter()
	AddDefaultRouterErrorHandlers(router, "Discovery Service")
	router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	var body []ErrorHandler
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected 1 error entry, got %d", len(body))
	}
	if body[0].Text != "resource not found" {
		t.Fatalf("expected error text %q, got %q", "resource not found", body[0].Text)
	}
	if !strings.Contains(body[0].CorrelationID, "DISCOVERYSERVICE-ROUTER-NOTFOUND") {
		t.Fatalf("expected correlation id to contain %q, got %q", "DISCOVERYSERVICE-ROUTER-NOTFOUND", body[0].CorrelationID)
	}
}

func TestAddDefaultRouterErrorHandlers_MethodNotAllowed(t *testing.T) {
	router := chi.NewRouter()
	AddDefaultRouterErrorHandlers(router, "Discovery Service")
	router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/description", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}

	var body []ErrorHandler
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected 1 error entry, got %d", len(body))
	}
	if body[0].Text != "method not allowed" {
		t.Fatalf("expected error text %q, got %q", "method not allowed", body[0].Text)
	}
	if !strings.Contains(body[0].CorrelationID, "DISCOVERYSERVICE-ROUTER-METHODNOTALLOWED") {
		t.Fatalf("expected correlation id to contain %q, got %q", "DISCOVERYSERVICE-ROUTER-METHODNOTALLOWED", body[0].CorrelationID)
	}
}
