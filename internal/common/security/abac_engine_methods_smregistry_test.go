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
	"github.com/go-chi/chi/v5"
)

func TestMapMethodAndPathToRights_SubmodelDescriptorQueryRoute(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	router.Method(http.MethodPost, "/query/submodel-descriptors", handler)
	model := &AccessModel{apiRouter: router, basePath: ""}

	rights, mapped, routeFound := model.mapMethodAndPathToRights(EvalInput{
		Method: http.MethodPost,
		Path:   "/query/submodel-descriptors",
	})

	if !routeFound {
		t.Fatalf("expected route %s %s to exist", http.MethodPost, "/query/submodel-descriptors")
	}
	if !mapped {
		t.Fatalf("expected route %s %s to have ABAC mapping", http.MethodPost, "/query/submodel-descriptors")
	}
	assertRightsAlternative(t, rights, []grammar.RightsEnum{grammar.RightsEnumREAD})
}
