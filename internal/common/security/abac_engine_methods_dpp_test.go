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
// Author: Jannik Fried ( Fraunhofer IESE )

package auth

import (
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
)

func TestMapMethodAndPathToRights_DPPAPIRoutes(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	registerDPPTestRoutes(router)
	model := &AccessModel{apiRouter: router, basePath: ""}

	tests := []struct {
		name   string
		method string
		path   string
		want   []grammar.RightsEnum
	}{
		{
			name:   "read dpp by id maps to read",
			method: http.MethodGet,
			path:   "/v1/dpps/demo",
			want:   []grammar.RightsEnum{grammar.RightsEnumREAD},
		},
		{
			name:   "delete dpp by id maps to delete",
			method: http.MethodDelete,
			path:   "/v1/dpps/demo",
			want:   []grammar.RightsEnum{grammar.RightsEnumDELETE},
		},
		{
			name:   "patch dpp by id maps to update",
			method: http.MethodPatch,
			path:   "/v1/dpps/demo",
			want:   []grammar.RightsEnum{grammar.RightsEnumUPDATE},
		},
		{
			name:   "create dpp maps to create",
			method: http.MethodPost,
			path:   "/v1/dpps",
			want:   []grammar.RightsEnum{grammar.RightsEnumCREATE},
		},
		{
			name:   "read dpp by product id maps to read",
			method: http.MethodGet,
			path:   "/v1/dppsByProductId/product-001",
			want:   []grammar.RightsEnum{grammar.RightsEnumREAD},
		},
		{
			name:   "read dpp version by date maps to read",
			method: http.MethodGet,
			path:   "/v1/dppsByIdAndDate/demo",
			want:   []grammar.RightsEnum{grammar.RightsEnumREAD},
		},
		{
			name:   "read dpp ids by product ids maps to read",
			method: http.MethodPost,
			path:   "/v1/dppsByProductIds",
			want:   []grammar.RightsEnum{grammar.RightsEnumREAD},
		},
		{
			name:   "read data element maps to read",
			method: http.MethodGet,
			path:   "/v1/dpps/demo/elements/technicalData/manufacturerName",
			want:   []grammar.RightsEnum{grammar.RightsEnumREAD},
		},
		{
			name:   "patch data element maps to update",
			method: http.MethodPatch,
			path:   "/v1/dpps/demo/elements/technicalData/energyClass",
			want:   []grammar.RightsEnum{grammar.RightsEnumUPDATE},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rights, mapped, routeFound := model.mapMethodAndPathToRights(EvalInput{Method: tt.method, Path: tt.path})
			if !routeFound {
				t.Fatalf("expected route %s %s to exist", tt.method, tt.path)
			}
			if !mapped {
				t.Fatalf("expected route %s %s to have ABAC mapping", tt.method, tt.path)
			}
			assertRightsAlternative(t, rights, tt.want)
		})
	}
}

func registerDPPTestRoutes(router chi.Router) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	router.Method(http.MethodGet, "/v1/dpps/{dppId}", handler)
	router.Method(http.MethodDelete, "/v1/dpps/{dppId}", handler)
	router.Method(http.MethodPatch, "/v1/dpps/{dppId}", handler)
	router.Method(http.MethodPost, "/v1/dpps", handler)
	router.Method(http.MethodGet, "/v1/dppsByProductId/{productId}", handler)
	router.Method(http.MethodGet, "/v1/dppsByIdAndDate/{dppId}", handler)
	router.Method(http.MethodPost, "/v1/dppsByProductIds", handler)
	router.Method(http.MethodGet, "/v1/dpps/{dppId}/elements/*", handler)
	router.Method(http.MethodPatch, "/v1/dpps/{dppId}/elements/*", handler)
}

func assertRightsAlternative(t *testing.T, rights [][]grammar.RightsEnum, want []grammar.RightsEnum) {
	t.Helper()

	if len(rights) != 1 {
		t.Fatalf("expected exactly one rights alternative, got %d", len(rights))
	}
	if len(rights[0]) != len(want) {
		t.Fatalf("expected rights %v, got %v", want, rights[0])
	}
	for i := range want {
		if rights[0][i] != want[i] {
			t.Fatalf("expected rights %v, got %v", want, rights[0])
		}
	}
}
