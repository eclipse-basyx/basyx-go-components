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

func TestConfigureAPIRouter_StripsTrailingSlashToCollectionRoute(t *testing.T) {
	router := chi.NewRouter()
	ConfigureAPIRouter(router, "Discovery Service")
	router.Get("/lookup/shells", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("collection"))
	})
	router.Get("/lookup/shells/{aasIdentifier}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("item"))
	})

	req := httptest.NewRequest(http.MethodGet, "/lookup/shells/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "collection" {
		t.Fatalf("expected collection route response, got %q", rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("expected no redirect location header, got %q", location)
	}
}

func TestConfigureAPIRouter_CollectionTrailingSlashMethodNotAllowed(t *testing.T) {
	router := chi.NewRouter()
	ConfigureAPIRouter(router, "Submodel Registry Service")
	router.Get("/submodel-descriptors", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	router.Post("/submodel-descriptors", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	router.Put("/submodel-descriptors/{submodelIdentifier}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodDelete, "/submodel-descriptors/", nil)
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
	if !strings.Contains(body[0].CorrelationID, "SUBMODELREGISTRYSERVICE-ROUTER-METHODNOTALLOWED") {
		t.Fatalf("expected correlation id to contain %q, got %q", "SUBMODELREGISTRYSERVICE-ROUTER-METHODNOTALLOWED", body[0].CorrelationID)
	}
}
