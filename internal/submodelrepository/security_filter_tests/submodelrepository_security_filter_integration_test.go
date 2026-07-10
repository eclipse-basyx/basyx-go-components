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

// nolint:all Is only a test file
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

var (
	securityFilterBaseURL          = testenv.LocalhostURLFromEnv("BASYX_IT_API_PORT", 6004)
	securityFilterKeycloakTokenURL = testenv.LocalhostURLFromEnv("BASYX_IT_KEYCLOAK_PORT", 8080) + "/realms/basyx/protocol/openid-connect/token"
)

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		DefaultExpectedStatus: http.StatusOK,
		TokenProvider: testenv.NewPasswordGrantTokenProvider(
			securityFilterKeycloakTokenURL,
			"basyx-ui",
			10*time.Second,
		),
	})
	t.Run("HiddenAndNonexistentSMECursorsAreIndistinguishable", func(t *testing.T) {
		assertEqualCursorResponses(t, "/submodel-elements")
		assertEqualCursorResponses(t, "/submodel-elements/$path")
	})
}

func assertEqualCursorResponses(t *testing.T, suffix string) {
	t.Helper()
	const submodelIdentifier = "dXJuOmV4YW1wbGU6c3VibW9kZWw6bmFtZXBsYXRlLWZpbHRlcg"
	const hiddenCursor = "QVJlc3RyaWN0ZWQ"
	const nonexistentCursor = "Qk5vbmV4aXN0ZW50"

	hiddenResponse := getAnonymousCursorResponse(t, submodelIdentifier, suffix, hiddenCursor)
	nonexistentResponse := getAnonymousCursorResponse(t, submodelIdentifier, suffix, nonexistentCursor)
	require.Equal(t, nonexistentResponse, hiddenResponse)
}

func getAnonymousCursorResponse(t *testing.T, submodelIdentifier string, suffix string, cursor string) any {
	t.Helper()
	request, requestErr := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		securityFilterBaseURL+"/submodels/"+submodelIdentifier+suffix+"?cursor="+cursor,
		nil,
	)
	require.NoError(t, requestErr)

	response, responseErr := http.DefaultClient.Do(request)
	require.NoError(t, responseErr)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)

	var payload any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	return payload
}

func TestMain(m *testing.M) {
	runtime := testenv.NewComposeRuntimeOrExit("submodelrepository-security-filter", []testenv.PortBinding{
		{Name: "api", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "db", EnvVar: "BASYX_IT_DB_PORT"},
		{Name: "keycloak", EnvVar: "BASYX_IT_KEYCLOAK_PORT"},
	})
	securityFilterBaseURL = runtime.LocalhostURL("api")
	securityFilterKeycloakTokenURL = runtime.LocalhostURL("keycloak") + "/realms/basyx/protocol/openid-connect/token"
	securityEnv := testenv.PrepareSecurityEnvOrExit("security_env", map[string]string{
		"http://localhost:8080": runtime.LocalhostURL("keycloak"),
	})

	code := testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "docker_compose/docker_compose.yml",
		ProjectName:     runtime.ProjectName,
		Env:             runtime.EnvWith("BASYX_IT_SECURITY_ENV=" + securityEnv),
		PreDownBeforeUp: true,
		HealthURL:       securityFilterBaseURL + "/health",
		HealthTimeout:   150 * time.Second,
	})
	_ = os.RemoveAll(securityEnv)
	os.Exit(code)
}
