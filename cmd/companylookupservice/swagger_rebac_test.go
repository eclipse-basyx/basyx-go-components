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

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestSwaggerNeverDocumentsReBAC(t *testing.T) {
	cfg := &common.Config{Swagger: common.SwaggerConfig{Enabled: true}}
	cfg.ReBAC.Enabled = true
	require.Contains(t, servedOpenAPISpec(t, cfg), "/security/rebac/", "rebac.enabled alone documents ReBAC")

	disableReBAC(cfg)
	spec := servedOpenAPISpec(t, cfg)
	require.NotContains(t, spec, "/security/rebac/", "the service offers no ReBAC management API")
	require.NotContains(t, spec, "/$access", "the service offers no $access sub-resources")
}

func servedOpenAPISpec(t *testing.T, cfg *common.Config) string {
	t.Helper()
	router := chi.NewRouter()
	require.NoError(t, common.AddSwaggerUIFromFS(router, openapiSpec, "openapi.yaml", "test", "/swagger", "/api-docs/openapi.yaml", cfg))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api-docs/openapi.yaml", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	return recorder.Body.String()
}
