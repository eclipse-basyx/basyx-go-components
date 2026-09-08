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
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestAASXFileServerRoutesHaveRightsMappings(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	router.Get("/packages", nil)
	router.Post("/packages", nil)
	router.Get("/packages/{packageId}", nil)
	router.Put("/packages/{packageId}", nil)
	router.Delete("/packages/{packageId}", nil)
	router.Post("/packages-async", nil)
	router.Get("/packages-async/status/{handleId}", nil)
	router.Get("/packages-async/result/{handleId}", nil)

	model := &AccessModel{apiRouter: router}
	tests := []struct {
		method string
		path   string
		rights []grammar.RightsEnum
	}{
		{http.MethodGet, "/packages", []grammar.RightsEnum{grammar.RightsEnumREAD}},
		{http.MethodPost, "/packages", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
		{http.MethodGet, "/packages/package-1", []grammar.RightsEnum{grammar.RightsEnumREAD}},
		{http.MethodPut, "/packages/package-1", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
		{http.MethodDelete, "/packages/package-1", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
		{http.MethodPost, "/packages-async", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
		{http.MethodGet, "/packages-async/status/handle-1", []grammar.RightsEnum{grammar.RightsEnumREAD}},
		{http.MethodGet, "/packages-async/result/handle-1", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			t.Parallel()
			alternatives, mapped, routeFound := model.mapMethodAndPathToRights(EvalInput{
				Method: test.method,
				Path:   test.path,
			})
			require.True(t, routeFound)
			require.True(t, mapped)
			require.Equal(t, [][]grammar.RightsEnum{test.rights}, alternatives)
		})
	}
}
