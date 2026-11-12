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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequireAuthenticationForAsyncRoutesMatchesOnlyTheRouteRootBelowTheContextPath(t *testing.T) {
	cases := []struct {
		name        string
		contextPath string
		path        string
		rejected    bool
	}{
		{name: "route root", contextPath: "", path: "/upload-async", rejected: true},
		{name: "route descendant", contextPath: "", path: "/upload-async/status/handle", rejected: true},
		{name: "route root below context path", contextPath: "/api/v3", path: "/api/v3/upload-async", rejected: true},
		{name: "descendant below context path", contextPath: "/api/v3/", path: "/api/v3/upload-async/result/handle", rejected: true},
		{name: "route root without context path", contextPath: "/api/v3", path: "/upload-async", rejected: false},
		{name: "longer path segment", contextPath: "", path: "/submodels/abc/submodel-elements/upload-asyncManual", rejected: false},
		{name: "nested segment of another route", contextPath: "", path: "/submodels/abc/upload-async", rejected: false},
		{name: "segment in context path", contextPath: "/upload-async", path: "/upload-async/shells", rejected: false},
		{name: "longer route root", contextPath: "", path: "/upload-asyncs", rejected: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.rejected, asyncAuthenticationRejects(t, testCase.contextPath, testCase.path, false))
			require.False(t, asyncAuthenticationRejects(t, testCase.contextPath, testCase.path, true))
		})
	}
}

func asyncAuthenticationRejects(t *testing.T, contextPath string, path string, authenticated bool) bool {
	t.Helper()
	handlerCalled := false
	handler := RequireAuthenticationForAsyncRoutes(contextPath, "/upload-async")(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, path, nil)
	request = request.WithContext(context.WithValue(request.Context(), authenticatedKey, authenticated))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code == http.StatusUnauthorized {
		require.False(t, handlerCalled)
		return true
	}
	require.True(t, handlerCalled)
	return false
}
