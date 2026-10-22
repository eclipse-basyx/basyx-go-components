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

package mcp

import (
	"context"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/aol"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestCreationPreflightRequiresReadAndDeleteAsWellAsCreate(t *testing.T) {
	router := chi.NewRouter()
	for _, kind := range []string{"shells", "submodels"} {
		router.Post("/"+kind, func(http.ResponseWriter, *http.Request) { t.Fatal("preflight must not mutate") })
		router.Get("/"+kind+"/{id}", func(http.ResponseWriter, *http.Request) {})
		router.Delete("/"+kind+"/{id}", func(http.ResponseWriter, *http.Request) { t.Fatal("preflight must not delete") })
	}
	model, err := auth.ParseAccessModel([]byte(`{"AllAccessPermissionRules":{"rules":[{"ACL":{"ACCESS":"ALLOW","RIGHTS":["CREATE"],"ATTRIBUTES":[{"CLAIM":"sub"}]},"OBJECTS":[{"ROUTE":"/*"}],"FORMULA":{"$boolean":true}}]}}`), router, "")
	require.NoError(t, err)
	module := &Module{security: auth.ABACSettings{Enabled: true, Model: model}}
	ctx := context.WithValue(t.Context(), auth.ClaimsKey, auth.Claims{"sub": "creator"})
	require.Error(t, module.authorizeDraft(ctx, aol.Draft{Shell: map[string]any{"id": "urn:aas"}, Submodels: []map[string]any{{"id": "urn:sm"}}}))
}
