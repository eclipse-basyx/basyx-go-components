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

package history

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestMutationCoverageGuardAllowsCoveredContextPathMutation(t *testing.T) {
	configureMutationCoverageTest(t, ModeAPI)
	guard := NewMutationCoverageGuard()
	guard.ClassifyRoute("PutSubmodelById", http.MethodPut, "/submodels/{submodelIdentifier}")

	handlerCalled := false
	apiRouter := chi.NewRouter()
	apiRouter.Use(guard.Middleware)
	apiRouter.Put("/submodels/{submodelIdentifier}", func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		coverage, ok := MutationCoverageFromContext(r.Context())
		require.True(t, ok)
		require.Equal(t, http.MethodPut, coverage.Method)
		require.Equal(t, "/submodels/{submodelIdentifier}", coverage.Pattern)
		require.True(t, coverage.Versioned)
		w.WriteHeader(http.StatusNoContent)
	})
	router := chi.NewRouter()
	router.Mount("/context", apiRouter)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/context/submodels/c20tMQ", nil))

	require.True(t, handlerCalled)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestMutationCoverageGuardDoesNotMatchExtraLeadingPathSegments(t *testing.T) {
	configureMutationCoverageTest(t, ModeAPI)
	guard := NewMutationCoverageGuard()
	guard.Cover(http.MethodDelete, "/shells/{aasIdentifier}")
	handlerCalled := false
	handler := guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/lookup/shells/aas-1", nil))

	require.False(t, handlerCalled)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestMutationCoverageGuardAllowsExplicitExemption(t *testing.T) {
	configureMutationCoverageTest(t, ModeAudit)
	guard := NewMutationCoverageGuard()
	guard.ClassifyRoute("QuerySubmodels", http.MethodPost, "/query/submodels")

	handler := guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coverage, ok := MutationCoverageFromContext(r.Context())
		require.True(t, ok)
		require.False(t, coverage.Versioned)
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/query/submodels", nil))

	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestMutationCoverageGuardRejectsUnclassifiedMutation(t *testing.T) {
	configureMutationCoverageTest(t, ModeAPI)
	guard := NewMutationCoverageGuard()
	handlerCalled := false
	handler := guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/new-mutation", nil))

	require.False(t, handlerCalled)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "HISTORY-COVERAGE-UNCLASSIFIED")
}

func TestMutationCoverageGuardPreservesRouterMethodNotAllowedResponse(t *testing.T) {
	configureMutationCoverageTest(t, ModeAPI)
	router := chi.NewRouter()
	guard := NewMutationCoverageGuard(router)
	router.Use(guard.Middleware)
	router.Post("/known-mutation", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/known-mutation", nil))
	require.Equal(t, http.StatusMethodNotAllowed, recorder.Code)

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/known-mutation", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "HISTORY-COVERAGE-UNCLASSIFIED")
}

func TestMutationCoverageGuardAllowsMutationWhenHistoryIsOff(t *testing.T) {
	configureMutationCoverageTest(t, ModeOff)
	guard := NewMutationCoverageGuard()
	handlerCalled := false
	handler := guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/new-mutation", nil))

	require.True(t, handlerCalled)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestMutationCoverageGuardAllowsReadsWithoutClassification(t *testing.T) {
	configureMutationCoverageTest(t, ModeAudit)
	guard := NewMutationCoverageGuard()
	handlerCalled := false
	handler := guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/submodels", nil))

	require.True(t, handlerCalled)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func configureMutationCoverageTest(t *testing.T, mode string) {
	t.Helper()
	Configure(Config{Mode: mode, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
}
