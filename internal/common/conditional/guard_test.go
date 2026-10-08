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

package conditional

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

const submodelPattern = "/submodels/{submodelIdentifier}"

func identity(value string) (string, error) {
	return value, nil
}

func serve(t *testing.T, options Options, method string, pattern string, path string, header http.Header, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	if options.DecodeIdentifier == nil {
		options.DecodeIdentifier = identity
	}
	router := chi.NewRouter()
	router.Method(method, pattern, NewGuard(options).Wrap(pattern, handler))
	request := httptest.NewRequest(method, path, nil)
	for key, values := range header {
		request.Header[key] = values
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func observedJSONHandler(revision int64, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stateFromContext(r.Context()).observe(revision)
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
	}
}

func TestGetReturnsRepresentationETagAndNotModified(t *testing.T) {
	response := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", nil, observedJSONHandler(5, `{"id":"sm1"}`))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, `{"id":"sm1"}`, response.Body.String())
	etag := response.Header().Get("ETag")
	require.Equal(t, quote(representationTag(concurrencyValidator(Ref(KindSubmodel, "sm1"), 5), []byte(`{"id":"sm1"}`))), etag)

	notModified := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", http.Header{"If-None-Match": {etag}}, observedJSONHandler(5, `{"id":"sm1"}`))
	require.Equal(t, http.StatusNotModified, notModified.Code)
	require.Empty(t, notModified.Body.String())
	require.Equal(t, etag, notModified.Header().Get("ETag"))

	otherView := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", http.Header{"If-None-Match": {etag}}, observedJSONHandler(5, `{"id":"sm1","x":1}`))
	require.Equal(t, http.StatusOK, otherView.Code)
	require.NotEqual(t, etag, otherView.Header().Get("ETag"))
}

func TestGetWithStaleIfMatchFails(t *testing.T) {
	stale := ConcurrencyETag(Ref(KindSubmodel, "sm1"), 4)
	response := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", http.Header{"If-Match": {stale}}, observedJSONHandler(5, `{}`))
	require.Equal(t, http.StatusPreconditionFailed, response.Code)
	require.Empty(t, response.Header().Get("ETag"))
	require.Contains(t, response.Body.String(), "COMMON-CONDREQ-IFMATCH")
}

func TestGetWithInconsistentObservationsSendsNoETag(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		state := stateFromContext(r.Context())
		state.observe(5)
		state.observe(6)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}
	response := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", nil, handler)
	require.Equal(t, http.StatusOK, response.Code)
	require.Empty(t, response.Header().Get("ETag"))
	require.Equal(t, `{}`, response.Body.String())
}

func TestBinaryGetIsStreamedWithURLBoundETag(t *testing.T) {
	pattern := submodelPattern + "/submodel-elements/{idShortPath}/attachment"
	handler := func(w http.ResponseWriter, r *http.Request) {
		stateFromContext(r.Context()).observe(5)
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "png")
	}
	response := serve(t, Options{}, http.MethodGet, pattern, "/submodels/sm1/submodel-elements/file/attachment", nil, handler)
	require.Equal(t, "png", response.Body.String())
	etag := response.Header().Get("ETag")
	require.Regexp(t, `^"5-[0-9a-f]{8}-[0-9a-f]{16}"$`, etag)

	notModified := serve(t, Options{}, http.MethodGet, pattern, "/submodels/sm1/submodel-elements/file/attachment", http.Header{"If-None-Match": {etag}}, handler)
	require.Equal(t, http.StatusNotModified, notModified.Code)
	require.Empty(t, notModified.Body.String())
}

func TestWriteResponsesCarryTheNewETagExceptPut(t *testing.T) {
	newETag := ConcurrencyETag(Ref(KindSubmodel, "sm1"), 9)
	handler := func(w http.ResponseWriter, r *http.Request) {
		stateFromContext(r.Context()).recordCommit(false, newETag)
		w.WriteHeader(http.StatusNoContent)
	}
	patch := serve(t, Options{}, http.MethodPatch, submodelPattern, "/submodels/sm1", nil, handler)
	require.Equal(t, newETag, patch.Header().Get("ETag"))

	put := serve(t, Options{}, http.MethodPut, submodelPattern, "/submodels/sm1", nil, handler)
	require.Empty(t, put.Header().Get("ETag"))
}

func TestReplacedPreconditionErrorsAreAnsweredWith412(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		stateFromContext(r.Context()).setFailure(errIfMatchFailed())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `[{"text":"wrapped"}]`)
	}
	response := serve(t, Options{}, http.MethodPut, submodelPattern, "/submodels/sm1", http.Header{"If-Match": {`"1-x"`}}, handler)
	require.Equal(t, http.StatusPreconditionFailed, response.Code)
	require.NotContains(t, response.Body.String(), "wrapped")
}

func TestCreateOnlyConflictIsAnsweredWith412(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}
	response := serve(t, Options{}, http.MethodPut, submodelPattern, "/submodels/sm1", http.Header{"If-None-Match": {"*"}}, handler)
	require.Equal(t, http.StatusPreconditionFailed, response.Code)
}

func TestCollectionsEvaluateConditionsWithoutAnETag(t *testing.T) {
	called := false
	handler := func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}
	failed := serve(t, Options{}, http.MethodPost, "/submodels", "/submodels", http.Header{"If-Match": {`"1-x"`}}, handler)
	require.Equal(t, http.StatusPreconditionFailed, failed.Code)
	require.False(t, called)

	passed := serve(t, Options{}, http.MethodPost, "/submodels", "/submodels", http.Header{"If-Match": {"*"}}, handler)
	require.Equal(t, http.StatusCreated, passed.Code)
	require.True(t, called)
}

func TestUndecodableIdentifiersDisableConditionalHandling(t *testing.T) {
	options := Options{DecodeIdentifier: func(string) (string, error) { return "", io.EOF }}
	response := serve(t, options, http.MethodGet, submodelPattern, "/submodels/not-base64", http.Header{"If-Match": {`"1-x"`}}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestDPPIdentifiersArePathDecoded(t *testing.T) {
	var target ResourceRef
	handler := func(w http.ResponseWriter, r *http.Request) {
		target = *stateFromContext(r.Context()).target
		w.WriteHeader(http.StatusOK)
	}
	serve(t, Options{}, http.MethodGet, "/v1/dpps/{dppId}", "/v1/dpps/urn%3Adpp%2F1", nil, handler)
	require.Equal(t, Ref(KindDPP, "urn:dpp/1"), target)
}

func TestJSONBasedFileDownloadsAreStreamed(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		stateFromContext(r.Context()).observe(5)
		w.Header().Set("Content-Type", "application/aasx+json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "zip")
		flushed, ok := w.(*responseWriter)
		require.True(t, ok)
		require.Nil(t, flushed.buffer, "file downloads must not be buffered")
	}
	response := serve(t, Options{}, http.MethodGet, "/packages/{packageId}", "/packages/p1", nil, handler)
	require.Equal(t, "zip", response.Body.String())
	require.Regexp(t, `^"5-[0-9a-f]{8}-[0-9a-f]{16}"$`, response.Header().Get("ETag"))
}

func TestOversizedJSONRepresentationsAreStreamedWithoutETag(t *testing.T) {
	chunk := strings.Repeat("x", 1<<20)
	handler := func(w http.ResponseWriter, r *http.Request) {
		stateFromContext(r.Context()).observe(5)
		w.Header().Set("Content-Type", "application/json")
		for range 17 {
			_, _ = io.WriteString(w, chunk)
		}
	}
	response := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", nil, handler)
	require.Equal(t, http.StatusOK, response.Code)
	require.Empty(t, response.Header().Get("ETag"))
	require.Equal(t, 17<<20, response.Body.Len())

	tagged := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", http.Header{"If-Match": {`"5-00000000"`}}, handler)
	require.Equal(t, http.StatusPreconditionFailed, tagged.Code)
	wildcard := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", http.Header{"If-Match": {"*"}}, handler)
	require.Equal(t, http.StatusOK, wildcard.Code)
	require.Equal(t, 17<<20, wildcard.Body.Len())

	notModified := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", http.Header{"If-None-Match": {"*"}}, handler)
	require.Equal(t, http.StatusNotModified, notModified.Code)
	require.Zero(t, notModified.Body.Len())
	otherTag := serve(t, Options{}, http.MethodGet, submodelPattern, "/submodels/sm1", http.Header{"If-None-Match": {`"5-00000000"`}}, handler)
	require.Equal(t, http.StatusOK, otherTag.Code)
	require.Equal(t, 17<<20, otherTag.Body.Len())
}
