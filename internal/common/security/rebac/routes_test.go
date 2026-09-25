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

package rebac

import (
	"strings"
	"testing"

	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

// coveredPrefixes are the route families of the services covered by PR 1.
var coveredPrefixes = []string{"/shells", "/submodels", "/concept-descriptions", "/query/shells", "/query/submodels", "/query/concept-descriptions"}

func TestEveryAuthorizableRouteIsClassified(t *testing.T) {
	t.Parallel()

	matrix := newRouteMatrix()
	for _, route := range auth.MappedRoutes() {
		if !hasCoveredPrefix(route.Pattern) {
			continue
		}
		_, covered := matrix.lookup(route.Method, route.Pattern)
		excluded := isExcludedRoute(route.Pattern)
		require.Truef(t, covered != excluded, "%s %s must be either covered or explicitly excluded", route.Method, route.Pattern)
	}
}

func TestExcludedRoutesAreNeverCovered(t *testing.T) {
	t.Parallel()

	for key := range newRouteMatrix() {
		_, pattern, _ := strings.Cut(key, " ")
		require.Falsef(t, isExcludedRoute(pattern), "%s is excluded from ReBAC", key)
	}
}

func TestSuperpathsRequireTheEnclosingShell(t *testing.T) {
	t.Parallel()

	for key, spec := range newRouteMatrix() {
		_, pattern, _ := strings.Cut(key, " ")
		isSuperpath := strings.HasPrefix(pattern, superpathShell+"/submodels/")
		require.Equalf(t, isSuperpath, spec.aasRelation != "", "%s", key)
	}
	deleteSpec, _ := newRouteMatrix().lookup("DELETE", superpathShell+"/submodels/{submodelIdentifier}")
	require.Equal(t, PermissionUpdate, deleteSpec.aasRelation, "removing a Submodel from a shell edits the shell")
	putSpec, _ := newRouteMatrix().lookup("PUT", superpathShell+"/submodels/{submodelIdentifier}")
	require.Equal(t, PermissionUpdate, putSpec.aasRelation, "superpath PUT writes a shell reference")
}

func hasCoveredPrefix(pattern string) bool {
	for _, prefix := range coveredPrefixes {
		if pattern == prefix || strings.HasPrefix(pattern, prefix+"/") {
			return true
		}
	}
	return false
}
