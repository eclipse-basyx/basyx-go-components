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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPathParamDecodesCanonicalEscapingExactlyOnce(t *testing.T) {
	req := requestWithChiParam("https://example.org/v1/dpps/literal%252Fid", "dppId", "literal%2Fid")

	if got := pathParam(req, "dppId"); got != "literal%2Fid" {
		t.Fatalf("pathParam() = %q, want literal %%2F to remain part of the identifier", got)
	}
}

func TestPathParamDecodesEscapedSlashExactlyOnce(t *testing.T) {
	req := requestWithChiParam("https://example.org/v1/dpps/literal%2Fid", "dppId", "literal%2Fid")

	if got := pathParam(req, "dppId"); got != "literal/id" {
		t.Fatalf("pathParam() = %q, want decoded slash", got)
	}
}

func TestElementIDPathParamPreservesCanonicalPercentEscape(t *testing.T) {
	req := requestWithChiParam("https://example.org/v1/dpps/dpp/elements/%2524", "*", "/%24")

	if got := elementIdPathParam(req); got != "%24" {
		t.Fatalf("elementIdPathParam() = %q, want literal %%24", got)
	}
}

func requestWithChiParam(target string, name string, value string) *http.Request {
	req := httptest.NewRequest("GET", target, nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add(name, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
}
