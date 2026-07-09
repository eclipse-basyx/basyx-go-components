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
	"sync"
	"unicode"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

var routerErrorComponents sync.Map

// ConfigureAPIRouter applies common API router behavior.
// It registers standardized 404/405 handlers.
func ConfigureAPIRouter(r *chi.Mux, component string) {
	AddDefaultRouterErrorHandlers(r, component)
}

// AddDefaultRouterErrorHandlers attaches standardized 404/405 responses to the router.
// The component name is used in correlation code generation.
func AddDefaultRouterErrorHandlers(r *chi.Mux, component string) {
	registerRouterErrorComponent(r, component)

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		WriteRouterNotFound(w, component)
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		WriteRouterMethodNotAllowed(w, component)
	})
}

// RouterErrorComponent returns the component configured for standardized router errors.
func RouterErrorComponent(r *chi.Mux) string {
	if r == nil {
		return ""
	}
	component, ok := routerErrorComponents.Load(r)
	if !ok {
		return ""
	}
	name, _ := component.(string)
	return name
}

// WriteRouterNotFound writes the standardized JSON 404 response used by API routers.
func WriteRouterNotFound(w http.ResponseWriter, component string) {
	writeRouterError(w, component, http.StatusNotFound, "resource not found", "NOTFOUND")
}

// WriteRouterMethodNotAllowed writes the standardized JSON 405 response used by API routers.
func WriteRouterMethodNotAllowed(w http.ResponseWriter, component string) {
	writeRouterError(w, component, http.StatusMethodNotAllowed, "method not allowed", "METHODNOTALLOWED")
}

func registerRouterErrorComponent(r *chi.Mux, component string) {
	if r == nil {
		return
	}
	routerErrorComponents.Store(r, component)
}

func writeRouterError(w http.ResponseWriter, component string, status int, message, errorType string) {
	componentID := normalizeComponentID(component)
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
