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

package openapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestSubmodelElementLocationPreservesEscapedSegments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		externalURL string
		want        string
	}{
		{
			name: "configured context path",
			want: "http://internal.example/api%20v3/submodels/c20/submodel-elements/Ops%2FAdd%20%3F%23%25",
		},
		{
			name:        "external base URL",
			externalURL: "https://public.example/aas%20environment/",
			want:        "https://public.example/aas%20environment/submodels/c20/submodel-elements/Ops%2FAdd%20%3F%23%25",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodPost, "http://internal.example/", nil)
			cfg := &common.Config{}
			cfg.ABAC.Enabled = false
			cfg.General.ExternalURL = test.externalURL
			request = request.WithContext(common.ContextWithConfig(request.Context(), cfg))
			controller := &SubmodelRepositoryAPIAPIController{contextPath: " /api%20v3/ "}

			location := controller.buildSubmodelElementLocationFromEncodedIdentifier(request, "c20", "Ops/Add ?#%")

			require.Equal(t, test.want, location)
		})
	}
}
