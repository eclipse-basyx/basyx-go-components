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

package auth

import (
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	apis "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi"
	"github.com/go-chi/chi/v5"
)

func TestMapMethodAndPathToRights_SignedSubmodelWriteRoutesMatchUnsignedRights(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	ctrl := apis.NewSubmodelRepositoryAPIAPIController(nil, "", "")
	for _, rt := range ctrl.Routes() {
		router.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	model := &AccessModel{apiRouter: router, basePath: ""}

	tests := []struct {
		name   string
		method string
		path   string
		want   []grammar.RightsEnum
	}{
		{
			name:   "put signed maps to create+update",
			method: http.MethodPut,
			path:   "/submodels/demo/$signed",
			want:   []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE},
		},
		{
			name:   "patch signed maps to update",
			method: http.MethodPatch,
			path:   "/submodels/demo/$signed",
			want:   []grammar.RightsEnum{grammar.RightsEnumUPDATE},
		},
		{
			name:   "delete signed maps to delete",
			method: http.MethodDelete,
			path:   "/submodels/demo/$signed",
			want:   []grammar.RightsEnum{grammar.RightsEnumDELETE},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rights, mapped, routeFound := model.mapMethodAndPathToRights(EvalInput{Method: tt.method, Path: tt.path})
			if !routeFound {
				t.Fatalf("expected route %s %s to exist", tt.method, tt.path)
			}
			if !mapped {
				t.Fatalf("expected route %s %s to have ABAC mapping", tt.method, tt.path)
			}
			if len(rights) != 1 {
				t.Fatalf("expected exactly one rights alternative, got %d", len(rights))
			}
			if len(rights[0]) != len(tt.want) {
				t.Fatalf("expected rights %v, got %v", tt.want, rights[0])
			}
			for i := range tt.want {
				if rights[0][i] != tt.want[i] {
					t.Fatalf("expected rights %v, got %v", tt.want, rights[0])
				}
			}
		})
	}
}
