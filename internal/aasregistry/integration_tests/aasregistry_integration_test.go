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
// Author: Martin Stemmer ( Fraunhofer IESE ), Christian Koort ( Fraunhofer IESE )

//nolint:all
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

var aasRegistryBaseURL = testenv.LocalURLFromEnv("BASYX_IT_API_PORT", 6004)
var aasRegistryIntegrationTestDSN = getAASRegistryIntegrationTestDSN()

func getAASRegistryIntegrationTestDSN() string {
	if dsn := os.Getenv("AASREGISTRY_INTEGRATION_TEST_DSN"); dsn != "" {
		return dsn
	}
	return testenv.PostgresURLFromEnv("BASYX_IT_DB_PORT", 6432, "basyxTestDB")
}

func deleteAllAASDescriptors(t *testing.T, runner *testenv.JSONSuiteRunner, stepNumber int) {
	response, err := runner.RunStep(testenv.JSONSuiteStep{
		Method:         http.MethodGet,
		Endpoint:       aasRegistryBaseURL + "/shell-descriptors",
		ExpectedStatus: http.StatusOK,
	}, stepNumber)
	require.NoError(t, err)

	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Body), &list))

	for _, item := range list.Result {
		enc := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		_, err := runner.RunStep(testenv.JSONSuiteStep{
			Method:         http.MethodDelete,
			Endpoint:       fmt.Sprintf("%s/shell-descriptors/%s", aasRegistryBaseURL, enc),
			ExpectedStatus: http.StatusNoContent,
		}, stepNumber)
		require.NoError(t, err)
	}
}

func postJSONResponse(endpoint string, body string) (map[string]any, int, http.Header, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("failed to read response body: %v", readErr)
	}

	if strings.TrimSpace(string(responseBody)) == "" {
		return nil, resp.StatusCode, resp.Header.Clone(), nil
	}

	var payload map[string]any
	if unmarshalErr := json.Unmarshal(responseBody, &payload); unmarshalErr != nil {
		return nil, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("failed to unmarshal response body: %v", unmarshalErr)
	}

	return payload, resp.StatusCode, resp.Header.Clone(), nil
}

func putJSONResponse(endpoint string, body string) (map[string]any, int, http.Header, error) {
	req, err := http.NewRequest(http.MethodPut, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("failed to read response body: %v", readErr)
	}

	if strings.TrimSpace(string(responseBody)) == "" {
		return nil, resp.StatusCode, resp.Header.Clone(), nil
	}

	var payload map[string]any
	if unmarshalErr := json.Unmarshal(responseBody, &payload); unmarshalErr != nil {
		return nil, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("failed to unmarshal response body: %v", unmarshalErr)
	}

	return payload, resp.StatusCode, resp.Header.Clone(), nil
}

func cleanupAASDescriptor(t *testing.T, baseURL string, aasID string) {
	t.Helper()

	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	endpoint := fmt.Sprintf("%s/shell-descriptors/%s", baseURL, aasIdentifier)

	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		t.Fatalf("failed to create cleanup request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to execute cleanup request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		cleanupBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			t.Fatalf("cleanup failed with status %d and unreadable body: %v", resp.StatusCode, readErr)
		}

		t.Fatalf("cleanup failed with status %d: %s", resp.StatusCode, string(cleanupBody))
	}
}

func buildAASDescriptorPayload(aasID string) string {
	return fmt.Sprintf(
		`{"id":"%s","assetKind":"Type","assetType":"integration-test","endpoints":[{"interface":"AAS-3.0","protocolInformation":{"href":"https://example.com/aas/%s","endpointProtocol":"https"}}]}`,
		aasID,
		base64.RawURLEncoding.EncodeToString([]byte(aasID)),
	)
}

func buildSubmodelDescriptorPayload(submodelID string, tag string) string {
	return fmt.Sprintf(
		`{"id":"%s","endpoints":[{"interface":"AAS-3.0","protocolInformation":{"href":"https://example.com/submodels/%s","endpointProtocol":"https"}}],"extensions":[{"name":"tag","valueType":"xs:string","value":"%s"}]}`,
		submodelID,
		base64.RawURLEncoding.EncodeToString([]byte(submodelID)),
		tag,
	)
}

func assertLocationHeaderMatches(t *testing.T, expectedLocation string, actualLocation string) {
	t.Helper()

	require.NotEmpty(t, actualLocation)

	expectedURL, err := url.Parse(expectedLocation)
	require.NoError(t, err)

	actualURL, err := url.Parse(actualLocation)
	require.NoError(t, err)

	require.Equal(t, expectedURL.Scheme, actualURL.Scheme)
	require.Equal(t, expectedURL.Path, actualURL.Path)
	require.Equal(t, expectedURL.RawQuery, actualURL.RawQuery)
	require.Equal(t, expectedURL.Port(), actualURL.Port())

	expectedHost := strings.ToLower(expectedURL.Hostname())
	actualHost := strings.ToLower(actualURL.Hostname())
	if expectedHost == actualHost {
		return
	}

	allowedLoopbackHosts := []string{"localhost", "127.0.0.1"}
	require.Contains(t, allowedLoopbackHosts, expectedHost)
	require.Contains(t, allowedLoopbackHosts, actualHost)
}

func TestLocationHeadersForCreateEndpointsAASRegistry(t *testing.T) {
	baseURL := aasRegistryBaseURL

	t.Run("PostAssetAdministrationShellDescriptorSetsLocation", func(t *testing.T) {
		aasID := fmt.Sprintf("https://example.com/ids/aas/location-post-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupAASDescriptor(t, baseURL, aasID) })

		aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
		endpoint := baseURL + "/shell-descriptors"

		_, statusCode, headers, err := postJSONResponse(endpoint, buildAASDescriptorPayload(aasID))
		require.NoError(t, err, "POST AAS descriptor request failed")
		require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for POST AAS descriptor")
		assertLocationHeaderMatches(t, endpoint+"/"+aasIdentifier, headers.Get("Location"))
	})

	t.Run("PutAssetAdministrationShellDescriptorByIdSetsLocationOnlyOnCreate", func(t *testing.T) {
		aasID := fmt.Sprintf("https://example.com/ids/aas/location-put-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupAASDescriptor(t, baseURL, aasID) })

		aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
		endpoint := fmt.Sprintf("%s/shell-descriptors/%s", baseURL, aasIdentifier)

		_, createStatusCode, createHeaders, createErr := putJSONResponse(endpoint, buildAASDescriptorPayload(aasID))
		require.NoError(t, createErr, "Initial PUT AAS descriptor request failed")
		require.Equal(t, http.StatusCreated, createStatusCode, "Expected 201 Created for initial PUT AAS descriptor")
		assertLocationHeaderMatches(t, endpoint, createHeaders.Get("Location"))

		_, updateStatusCode, updateHeaders, updateErr := putJSONResponse(endpoint, buildAASDescriptorPayload(aasID))
		require.NoError(t, updateErr, "Update PUT AAS descriptor request failed")
		require.Equal(t, http.StatusNoContent, updateStatusCode, "Expected 204 No Content for update PUT AAS descriptor")
		require.Empty(t, updateHeaders.Get("Location"), "Expected no Location header for update PUT AAS descriptor")
	})

	t.Run("PostSubmodelDescriptorThroughSuperpathSetsLocation", func(t *testing.T) {
		aasID := fmt.Sprintf("https://example.com/ids/aas/location-submodel-parent-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupAASDescriptor(t, baseURL, aasID) })

		aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

		_, aasStatusCode, _, aasErr := postJSONResponse(baseURL+"/shell-descriptors", buildAASDescriptorPayload(aasID))
		require.NoError(t, aasErr, "POST parent AAS descriptor request failed")
		require.Equal(t, http.StatusCreated, aasStatusCode, "Expected 201 Created for parent AAS descriptor")

		submodelID := fmt.Sprintf("urn:example:sm:location-post-%d", time.Now().UnixNano())
		submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
		endpoint := fmt.Sprintf("%s/shell-descriptors/%s/submodel-descriptors", baseURL, aasIdentifier)

		_, statusCode, headers, err := postJSONResponse(endpoint, buildSubmodelDescriptorPayload(submodelID, "v1"))
		require.NoError(t, err, "POST submodel descriptor request failed")
		require.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for POST submodel descriptor")
		assertLocationHeaderMatches(t, endpoint+"/"+submodelIdentifier, headers.Get("Location"))
	})

	t.Run("PutSubmodelDescriptorByIdThroughSuperpathSetsLocationOnlyOnCreate", func(t *testing.T) {
		aasID := fmt.Sprintf("https://example.com/ids/aas/location-submodel-put-parent-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupAASDescriptor(t, baseURL, aasID) })

		aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))

		_, aasStatusCode, _, aasErr := postJSONResponse(baseURL+"/shell-descriptors", buildAASDescriptorPayload(aasID))
		require.NoError(t, aasErr, "POST parent AAS descriptor request failed")
		require.Equal(t, http.StatusCreated, aasStatusCode, "Expected 201 Created for parent AAS descriptor")

		submodelID := fmt.Sprintf("urn:example:sm:location-put-%d", time.Now().UnixNano())
		submodelIdentifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
		endpoint := fmt.Sprintf("%s/shell-descriptors/%s/submodel-descriptors/%s", baseURL, aasIdentifier, submodelIdentifier)

		_, createStatusCode, createHeaders, createErr := putJSONResponse(endpoint, buildSubmodelDescriptorPayload(submodelID, "before"))
		require.NoError(t, createErr, "Initial PUT submodel descriptor request failed")
		require.Equal(t, http.StatusCreated, createStatusCode, "Expected 201 Created for initial PUT submodel descriptor")
		assertLocationHeaderMatches(t, endpoint, createHeaders.Get("Location"))

		_, updateStatusCode, updateHeaders, updateErr := putJSONResponse(endpoint, buildSubmodelDescriptorPayload(submodelID, "after"))
		require.NoError(t, updateErr, "Update PUT submodel descriptor request failed")
		require.Equal(t, http.StatusNoContent, updateStatusCode, "Expected 204 No Content for update PUT submodel descriptor")
		require.Empty(t, updateHeaders.Get("Location"), "Expected no Location header for update PUT submodel descriptor")
	})
}

func TestIntegration(t *testing.T) {
	isExternalCompose := os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1"
	checkDBOptions := testenv.CheckDBIsEmptyOptions{
		Driver: "pgx",
		DSN:    aasRegistryIntegrationTestDSN,
	}
	if isExternalCompose {
		checkDBOptions.ExcludedTables = []string{"aas_identifier"}
	}

	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ShouldSkipStep: func(step testenv.JSONSuiteStep) bool {
			if !isExternalCompose {
				return false
			}

			return strings.Contains(step.Context, "Empty Keys in Reference")
		},
		ActionHandlers: map[string]testenv.JSONStepAction{
			"DELETE_ALL_AAS_DESCRIPTORS": func(t *testing.T, runner *testenv.JSONSuiteRunner, _ testenv.JSONSuiteStep, stepNumber int) {
				deleteAllAASDescriptors(t, runner, stepNumber)
			},
			testenv.ActionCheckDBIsEmpty: testenv.NewCheckDBIsEmptyAction(checkDBOptions),
		},
	})
}

func TestMain(m *testing.M) {
	if os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1" {
		os.Exit(m.Run())
	}

	runtime := testenv.NewComposeRuntimeOrExit("aasregistry-it", []testenv.PortBinding{
		{Name: "api", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "db", EnvVar: "BASYX_IT_DB_PORT"},
	})
	aasRegistryBaseURL = runtime.LocalURL("api")
	aasRegistryIntegrationTestDSN = runtime.PostgresURL("db", "basyxTestDB")

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: "docker_compose/docker_compose.yml",
		ProjectName: runtime.ProjectName,
		Env:         runtime.Env(),
		HealthURL:   aasRegistryBaseURL + "/health",
	}))
}
