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

package common

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

// ConfigureAPIRouter applies common API router behavior.
// It registers standardized 404/405 handlers.
func ConfigureAPIRouter(r *chi.Mux, component string) {
	AddDefaultRouterErrorHandlers(r, component)
}

// AddDefaultRouterErrorHandlers attaches standardized 404/405 responses to the router.
// The component name is used in correlation code generation.
func AddDefaultRouterErrorHandlers(r *chi.Mux, component string) {
	componentID := normalizeComponentID(component)

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeRouterError(w, component, componentID, http.StatusNotFound, "resource not found", "NOTFOUND")
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeRouterError(w, component, componentID, http.StatusMethodNotAllowed, "method not allowed", "METHODNOTALLOWED")
	})
}

func writeRouterError(w http.ResponseWriter, component, componentID string, status int, message, errorType string) {
	resp := NewErrorResponse(
		errors.New(message),
		status,
		component,
		"Router",
		fmt.Sprintf("%s-ROUTER-%s", componentID, errorType),
	)
	if err := model.EncodeJSONResponse(resp.Body, &resp.Code, w); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func normalizeComponentID(component string) string {
	var b strings.Builder
	b.Grow(len(component))

	for _, r := range component {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			_, _ = b.WriteRune(unicode.ToUpper(r))
		}
	}

	if b.Len() == 0 {
		return "COMPONENT"
	}

	return b.String()
}
