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

	"github.com/eclipse-basyx/basyx-go-components/internal/aol"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func (m *Module) authorizeDraft(ctx context.Context, draft aol.Draft) error {
	targets := []struct{ kind, id string }{{"shells", draft.Shell["id"].(string)}}
	for _, sm := range draft.Submodels {
		targets = append(targets, struct{ kind, id string }{"submodels", sm["id"].(string)})
	}
	for _, target := range targets {
		for _, method := range []string{"POST", "GET", "DELETE"} {
			path := "/" + target.kind
			if method != "POST" {
				path += "/" + aol.EncodeID(target.id)
			}
			if err := m.authorizeAction(ctx, method, path); err != nil {
				return err
			}
		}
	}
	return nil
}
func (m *Module) authorizeAction(ctx context.Context, method, path string) error {
	if !m.security.Enabled {
		return aol.Failure("AOL-PLAN-PERMISSIONS", "creation requires an active authorization preflight", "Configure the secured MCP module.")
	}
	right := grammar.RightsEnumREAD
	if method == "POST" {
		right = grammar.RightsEnumCREATE
	}
	if method == "DELETE" {
		right = grammar.RightsEnumDELETE
	}
	guard := auth.ABACMiddleware(m.security)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filter := auth.GetQueryFilter(r.Context())
		if !auth.HasUnrestrictedFormulaForRight(r.Context(), right) || (filter != nil && len(filter.Filters) > 0) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	probe := aol.Dispatcher{Router: guard, MaxBytes: 4096, BasePath: m.reader.Dispatch.BasePath}
	if _, err := probe.Do(ctx, method, path, nil, nil); err != nil {
		return aol.Failure("AOL-PLAN-PERMISSIONS", "creation requires provable full CREATE, READ and DELETE permissions for every target", "Use an operator policy covering creation, verification and compensation; field-dependent grants are not sufficient.")
	}
	return nil
}
