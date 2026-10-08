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

package rebacintegration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAccessChangesInvalidateRepresentationETags checks that a caller whose
// view of a resource changes does not get 304 for the old view, although the
// resource revision did not change.
func TestAccessChangesInvalidateRepresentationETags(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "etag-view", property("a", "first"), property("b", "second"))
	elements := submodelURL + "/submodels/" + enc(identifier) + "/submodel-elements"
	addGrants(t, "alice", elementAccess(submodelURL, identifier, "a"), userGrant(t, "viewer", "bob"))

	narrow := call(t, "bob", http.MethodGet, elements, nil, nil)
	expectStatus(t, http.StatusOK, narrow, "element-only list")
	narrowTag := narrow.header.Get("ETag")
	require.NotEmpty(t, narrowTag)
	expectStatus(t, http.StatusNotModified, call(t, "bob", http.MethodGet, elements, nil, map[string]string{"If-None-Match": narrowTag}), "unchanged view")

	full := call(t, "alice", http.MethodGet, elements, nil, nil)
	require.NotEqual(t, narrowTag, full.header.Get("ETag"), "different views have different entity tags")

	addGrants(t, "alice", elementAccess(submodelURL, identifier, "b"), userGrant(t, "viewer", "bob"))
	widened := call(t, "bob", http.MethodGet, elements, nil, map[string]string{"If-None-Match": narrowTag})
	expectStatus(t, http.StatusOK, widened, "a changed view is never answered with 304")
	require.ElementsMatch(t, []string{"a", "b"}, resultIDShorts(t, widened))
	require.NotEqual(t, narrowTag, widened.header.Get("ETag"))
}
