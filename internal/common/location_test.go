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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextualizeAPIResourceLocationPreservesEscapedSegments(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(
		http.MethodPost,
		"http://example.com/api/submodels/c20/submodel-elements/Ops%2FAdd/invoke-async",
		nil,
	)

	location := ContextualizeAPIResourceLocation(
		request,
		"/submodels/c20/submodel-elements/Ops%2FAdd/operation-status/handle-1",
		"/submodels/",
	)

	require.Equal(
		t,
		"http://example.com/api/submodels/c20/submodel-elements/Ops%2FAdd/operation-status/handle-1",
		location,
	)
}

func TestContextualizeAPIResourceLocationUsesEscapedExternalBaseURL(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(
		http.MethodPost,
		"http://internal.example/internal/submodels/c20/submodel-elements/Ops%2FAdd/invoke-async",
		nil,
	)
	cfg := &Config{}
	cfg.General.ExternalURL = "https://public.example/aas%20environment"
	request = request.WithContext(ContextWithConfig(request.Context(), cfg))

	location := ContextualizeAPIResourceLocation(
		request,
		"/submodels/c20/submodel-elements/Ops%2FAdd/operation-status/handle-1",
		"/submodels/",
	)

	require.Equal(
		t,
		"https://public.example/aas%20environment/submodels/c20/submodel-elements/Ops%2FAdd/operation-status/handle-1",
		location,
	)
}
