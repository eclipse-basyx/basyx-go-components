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

package rebac

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestElementManagementTargetsRejectInvalidIDShortPaths(t *testing.T) {
	t.Parallel()

	coordinator := &Coordinator{}
	var elementBase *accessBase
	for _, base := range coordinator.accessBases([]ResourceKind{KindSubmodel}) {
		if base.element {
			elementBase = &base
		}
	}
	require.NotNil(t, elementBase)

	for target, idShortPath := range map[string]string{
		"/request/%2541":               "%41",
		"/request/Coll.x%2Fattachment": "Coll.x%2Fattachment",
		"/request/%25":                 "%25",
		"/request/Coll%20Prop":         "Coll Prop",
	} {
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add(paramSubmodel, "c3VibW9kZWw")
		routeContext.URLParams.Add(paramPath, idShortPath)
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))

		_, found := elementBase.target(request)
		require.False(t, found, "idShortPath %q must not resolve a management target", idShortPath)
	}
}
