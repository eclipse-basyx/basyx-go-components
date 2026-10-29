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

package aol

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestDispatchUsesOriginalTokenAndFreshRouteContext(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/submodels/{id}", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer caller", r.Header.Get("Authorization"))
		require.Equal(t, "urn:sm", DecodeID(chi.URLParam(r, "id")))
		require.Equal(t, "session", IdentityFromContext(r.Context()).SessionID)
		_, err := w.Write([]byte(`{"value":42}`))
		require.NoError(t, err)
	})
	ctx := ContextWithIdentity(t.Context(), Identity{Token: "Bearer caller", SessionID: "session"})
	ctx = context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
	d := Dispatcher{Router: router, MaxBytes: 1024}
	result, err := d.Do(ctx, http.MethodGet, "/submodels/"+EncodeID("urn:sm"), nil, nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"value":42}`, string(result.Body))
}

func TestDispatchRejectsOversizeAndPreservesHTTPFailure(t *testing.T) {
	d := Dispatcher{MaxBytes: 8, Router: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403); _, _ = w.Write([]byte(`denied`)) })}
	_, err := d.Do(t.Context(), "GET", "/shells", nil, nil)
	var failure *Error
	require.ErrorAs(t, err, &failure)
	require.Equal(t, 403, failure.Status)
	d.Router = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`0123456789`)) })
	_, err = d.Do(t.Context(), "GET", "/shells", json.RawMessage(nil), nil)
	require.ErrorContains(t, err, "LIMIT")
}

func TestElementRejectsRouteTraversalBeforeDispatch(t *testing.T) {
	reader := Reader{Dispatch: Dispatcher{Router: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid path reached router") }), MaxBytes: 1024}}
	for _, path := range []string{"../../security/abac", "Temperature/$metadata", "Temperature?x=1", "", "Temperature..Value"} {
		_, err := reader.Element(t.Context(), "urn:sm", path)
		require.Error(t, err)
	}
}
