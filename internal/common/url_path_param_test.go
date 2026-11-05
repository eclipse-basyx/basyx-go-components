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

//nolint:all - package name is not meaningless
package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestURLPathParamDecodesEveryClientEncodingOnce(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		segment string
		want    string
	}{
		{name: "plain path", segment: "Coll.plain", want: "Coll.plain"},
		{name: "unencoded semicolon", segment: "Coll.a;b", want: "Coll.a;b"},
		{name: "encoded semicolon", segment: "Coll.a%3Bb", want: "Coll.a;b"},
		{name: "encoded comma", segment: "Coll.a%2Cb", want: "Coll.a,b"},
		{name: "encoded colon", segment: "Coll.a%3Ab", want: "Coll.a:b"},
		{name: "encoded slash", segment: "Coll.a%2Fb", want: "Coll.a/b"},
		{name: "over-encoded dot", segment: "Coll%2Eplain", want: "Coll.plain"},
		{name: "space in canonical form", segment: "Coll.a%20b", want: "Coll.a b"},
		{name: "space next to encoded semicolon", segment: "Coll.a%20b%3Bc", want: "Coll.a b;c"},
		{name: "list index", segment: "List%5B0%5D", want: "List[0]"},
		{name: "encoded percent in canonical form", segment: "a%2525", want: "a%25"},
		{name: "encoded percent next to encoded semicolon", segment: "a%25%3B", want: "a%;"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, testCase.want, routedURLPathParam(t, "/elements/"+testCase.segment))
		})
	}
}

func routedURLPathParam(t *testing.T, target string) string {
	t.Helper()
	routed := false
	var value string
	router := chi.NewRouter()
	router.Get("/elements/{idShortPath}", func(_ http.ResponseWriter, r *http.Request) {
		routed = true
		value = URLPathParam(r, "idShortPath")
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	require.True(t, routed, "request %q was not routed", target)
	return value
}
