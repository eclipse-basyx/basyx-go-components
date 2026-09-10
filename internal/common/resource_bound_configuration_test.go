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
	"encoding/json"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResourceBoundConfigurationIsOptIn(t *testing.T) {
	cfg := &Config{}
	require.NoError(t, validateResourceBoundConfig(cfg))
	require.False(t, ResourceBoundEnabled(cfg))
	cfg.Security.AuthorizationMode = AuthorizationResourceBoundFirst
	require.Error(t, validateResourceBoundConfig(cfg))
	cfg.ReBAC.BootstrapOwner = AccessPrincipal{Issuer: "https://issuer.example", Subject: "owner"}
	require.NoError(t, validateResourceBoundConfig(cfg))
	require.Equal(t, "default", cfg.ReBAC.PolicyScope)
	require.True(t, ResourceBoundEnabled(cfg))
	cfg.Security.AuthorizationMode = "invalid"
	require.Error(t, validateResourceBoundConfig(cfg))
	cfg.Security.AuthorizationMode = AuthorizationResourceBoundFirst
	cfg.ReBAC.BootstrapOwner.Subject = " \t"
	require.ErrorContains(t, validateResourceBoundConfig(cfg), "CONFIG-REBAC-OWNER")
}

func TestResourceBoundEnvironmentOverrides(t *testing.T) {
	t.Setenv("SECURITY_AUTHORIZATION_MODE", AuthorizationResourceBoundFirst)
	t.Setenv("REBAC_POLICY_SCOPE", "bridges")
	t.Setenv("REBAC_BOOTSTRAP_OWNER_ISSUER", "https://issuer.example")
	t.Setenv("REBAC_BOOTSTRAP_OWNER_SUBJECT", "owner")
	cfg := &Config{}
	applyResourceBoundEnvOverrides(cfg)
	require.NoError(t, validateResourceBoundConfig(cfg))
	require.Equal(t, "bridges", cfg.ReBAC.PolicyScope)
	require.True(t, ResourceBoundEnabled(cfg))
}

func TestResourceBoundOpenAPIMatchesAvailableResources(t *testing.T) {
	input := []byte(`openapi: 3.0.3
paths:
  /shells/{aasIdentifier}: {}
  /submodels: {}
  /submodels/{submodelIdentifier}: {}
  /submodels/{submodelIdentifier}/submodel-elements/{idShortPath}: {}
`)
	output, err := injectResourceBoundAPI(input)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(output, &doc))
	paths := doc["paths"].(map[string]any)
	require.Contains(t, paths, "/submodels/$access/policy")
	require.Contains(t, paths, "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$access/grants/{grantId}")
	require.NotContains(t, paths, "/shells/$access")
	capability := paths["/shells/{aasIdentifier}/$access/capabilities"].(map[string]any)["get"].(map[string]any)
	capabilityJSON := string(mustJSON(t, capability))
	require.Contains(t, capabilityJSON, "AASAccessCapabilities")
	require.Contains(t, capabilityJSON, "no-store")
	require.NotContains(t, capabilityJSON, "ETag")
	require.NotContains(t, capabilityJSON, "If-Match")
	operation := paths["/submodels/{submodelIdentifier}/$access/owners"].(map[string]any)["put"].(map[string]any)
	require.Contains(t, operation["responses"], "428")
	require.Contains(t, operation["responses"], "412")
	require.Contains(t, operation["responses"], "413")
	require.Contains(t, operation["responses"], "405")
	require.Contains(t, string(output), "If-Match")
	grantOperation := paths["/submodels/{submodelIdentifier}/$access/grants"].(map[string]any)["post"].(map[string]any)
	require.Contains(t, string(mustJSON(t, grantOperation)), "Location")
	require.Contains(t, string(output), "uniqueItems: true")
	require.Contains(t, string(output), `pattern: .*\S.*`)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func TestResourceBoundCORSAllowsAccessPreconditions(t *testing.T) {
	cfg := &Config{Security: SecurityConfig{AuthorizationMode: AuthorizationResourceBoundFirst}, CorsConfig: CorsConfig{AllowedOrigins: []string{"https://client.example"}, AllowedMethods: []string{"GET", "PUT"}}}
	router := chi.NewRouter()
	AddCors(router, cfg)
	router.Get("/access", func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("ETag", "revision"); w.WriteHeader(200) })
	preflight := httptest.NewRequest("OPTIONS", "/access", nil)
	preflight.Header.Set("Origin", "https://client.example")
	preflight.Header.Set("Access-Control-Request-Method", "PUT")
	preflight.Header.Set("Access-Control-Request-Headers", "If-Match")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, preflight)
	requireHeaderValues(t, response.Header().Get("Access-Control-Allow-Headers"), "If-Match")
	request := httptest.NewRequest("GET", "/access", nil)
	request.Header.Set("Origin", "https://client.example")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	requireHeaderValues(t, response.Header().Get("Access-Control-Expose-Headers"), "ETag")
}
