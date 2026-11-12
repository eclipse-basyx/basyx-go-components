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
	"errors"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// RequireAuthenticationForAsyncRoutes rejects unauthenticated requests to an
// asynchronous route root and its descendants with 401, so every asynchronous
// job handle on those routes belongs to an authenticated principal. Only the
// route root below the context path matches, at a path segment boundary; other
// routes are left to the authorization chain.
//
// Parameters:
//   - contextPath: Configured context path the API router is mounted at.
//   - routeRoot: Asynchronous route root relative to the context path, such as "/packages-async".
//
// Returns:
//   - func(http.Handler) http.Handler: Middleware for the security claims chain.
func RequireAuthenticationForAsyncRoutes(contextPath string, routeRoot string) func(http.Handler) http.Handler {
	root := joinBasePath(contextPath, routeRoot)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if isRouteOrDescendant(request.URL.Path, root) && !IsAuthenticated(request.Context()) {
				_ = common.WriteErrorResponse(writer, errors.New("access denied"), http.StatusUnauthorized, "Middleware", "Rules", "Denied")
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func isRouteOrDescendant(requestPath string, root string) bool {
	return requestPath == root || strings.HasPrefix(requestPath, root+"/")
}
