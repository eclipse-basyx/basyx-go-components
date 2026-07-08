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

//nolint:all
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var aasEnvNoRedirectClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func TestAASEnvironmentBulkOperationsAndDescription(t *testing.T) {
	t.Run("DescriptionProfilesMatchExpectedSet", func(t *testing.T) {
		resetDatabase(t)
		status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/description", nil)
		require.Equal(t, http.StatusOK, status)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))
		profiles := toStringSliceAASEnv(t, payload["profiles"])
		require.ElementsMatch(t, expectedAASEnvironmentDescriptionProfiles(), profiles)
	})

	t.Run("AASBulkSuccessAndFailureRollback", func(t *testing.T) {
		resetDatabase(t)
		deleteAllAASEnvDescriptors(t)

		aas1 := loadAASEnvJSONFixtureMap(t, "../../aasregistry/integration_tests/postBody/simple_ass_descriptor_1.json")
		aas2 := loadAASEnvJSONFixtureMap(t, "../../aasregistry/integration_tests/postBody/simple_ass_descriptor_2.json")

		handleCreate := startAASEnvBulkAndReadHandle(t, http.MethodPost, "/bulk/shell-descriptors", []any{aas1, aas2})
		awaitAASEnvBulkStatusRedirect(t, handleCreate)
		assertAASEnvBulkResultStatus(t, handleCreate, http.StatusNoContent)

		assertAASEnvAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/22335", http.StatusOK)
		assertAASEnvAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/34hzeaw", http.StatusOK)

		aas3 := loadAASEnvJSONFixtureMap(t, "../../aasregistry/integration_tests/postBody/simple_ass_descriptor_3.json")
		handleFail := startAASEnvBulkAndReadHandle(t, http.MethodPost, "/bulk/shell-descriptors", []any{aas3, aas1})
		awaitAASEnvBulkStatusRedirect(t, handleFail)
		body := assertAASEnvBulkResultStatus(t, handleFail, http.StatusBadRequest)
		assertAASEnvAtomicBulkFailureBody(t, body, 2)

		assertAASEnvAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/22335", http.StatusOK)
		assertAASEnvAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/3657le6d", http.StatusNotFound)
	})

	t.Run("SubmodelBulkSuccessAndFailureRollback", func(t *testing.T) {
		resetDatabase(t)
		deleteAllAASEnvSubmodelDescriptors(t)

		smd1 := loadAASEnvJSONFixtureMap(t, "../../smregistry/integration_tests/postBody/simple_submodel_descriptor_1.json")
		smd2 := loadAASEnvJSONFixtureMap(t, "../../smregistry/integration_tests/postBody/simple_submodel_descriptor_2.json")

		handleCreate := startAASEnvBulkAndReadHandle(t, http.MethodPost, "/bulk/submodel-descriptors", []any{smd1, smd2})
		awaitAASEnvBulkStatusRedirect(t, handleCreate)
		assertAASEnvBulkResultStatus(t, handleCreate, http.StatusNoContent)

		assertAASEnvSubmodelDescriptorStatus(t, "urn:example:sm:001", http.StatusOK)
		assertAASEnvSubmodelDescriptorStatus(t, "urn:example:sm:002", http.StatusOK)

		smd3 := loadAASEnvJSONFixtureMap(t, "../../smregistry/integration_tests/postBody/simple_submodel_descriptor_3.json")
		handleFail := startAASEnvBulkAndReadHandle(t, http.MethodPost, "/bulk/submodel-descriptors", []any{smd3, smd1})
		awaitAASEnvBulkStatusRedirect(t, handleFail)
		body := assertAASEnvBulkResultStatus(t, handleFail, http.StatusBadRequest)
		assertAASEnvAtomicBulkFailureBody(t, body, 2)

		assertAASEnvSubmodelDescriptorStatus(t, "urn:example:sm:001", http.StatusOK)
		assertAASEnvSubmodelDescriptorStatus(t, "urn:example:sm:003", http.StatusNotFound)
	})

	t.Run("SharedHandleStatusAndResultAcrossAASAndSubmodelBulk", func(t *testing.T) {
		resetDatabase(t)
		deleteAllAASEnvDescriptors(t)
		deleteAllAASEnvSubmodelDescriptors(t)

		aas4 := loadAASEnvJSONFixtureMap(t, "../../aasregistry/integration_tests/postBody/simple_ass_descriptor_4.json")
		smd3 := loadAASEnvJSONFixtureMap(t, "../../smregistry/integration_tests/postBody/simple_submodel_descriptor_3.json")

		aasHandle := startAASEnvBulkAndReadHandle(t, http.MethodPost, "/bulk/shell-descriptors", []any{aas4})
		smHandle := startAASEnvBulkAndReadHandle(t, http.MethodPost, "/bulk/submodel-descriptors", []any{smd3})

		awaitAASEnvBulkStatusRedirect(t, aasHandle)
		awaitAASEnvBulkStatusRedirect(t, smHandle)

		assertAASEnvBulkResultStatus(t, aasHandle, http.StatusNoContent)
		assertAASEnvBulkResultStatus(t, smHandle, http.StatusNoContent)

		assertAASEnvAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/w456kjw5k6z", http.StatusOK)
		assertAASEnvSubmodelDescriptorStatus(t, "urn:example:sm:003", http.StatusOK)
	})
}

func expectedAASEnvironmentDescriptionProfiles() []string {
	return []string{
		"https://basyx.org/aas/API/3/2/AssetAdministrationShellEnvironment/SSP-001",
		"https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-003",
		"https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-004",
		"https://basyx.org/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/SubmodelRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/SubmodelRegistryServiceSpecification/SSP-003",
		"https://basyx.org/aas/API/3/2/SubmodelRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRepositoryServiceSpecification/SSP-001",
		"https://basyx.org/aas/API/3/2/AssetAdministrationShellRepositoryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/SubmodelRepositoryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/SubmodelRepositoryServiceSpecification/SSP-005",
		"https://basyx.org/aas/API/3/2/SubmodelRepositoryService/1.0",
		"https://admin-shell.io/aas/API/3/2/ConceptDescriptionRepositoryServiceSpecification/SSP-001",
		"https://basyx.org/aas/API/3/2/ConceptDescriptionRepositoryService/1.0",
		"https://admin-shell.io/aas/API/3/2/DiscoveryServiceSpecification/SSP-001",
		"https://basyx.org/aas/API/3/2/DiscoveryServiceSpecification/SSP-001",
	}
}

func loadAASEnvJSONFixtureMap(t *testing.T, relativePath string) map[string]any {
	t.Helper()
	path := filepath.Clean(relativePath)
	bytesPayload, err := os.ReadFile(path)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(bytesPayload, &value))
	return value
}

func deleteAllAASEnvDescriptors(t *testing.T) {
	t.Helper()
	status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/shell-descriptors", nil)
	require.Equal(t, http.StatusOK, status)
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	for _, item := range list.Result {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		delStatus, _, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodDelete, aasEnvBaseURL+"/shell-descriptors/"+encoded, nil)
		require.Equal(t, http.StatusNoContent, delStatus)
	}
}

func deleteAllAASEnvSubmodelDescriptors(t *testing.T) {
	t.Helper()
	status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/submodel-descriptors", nil)
	require.Equal(t, http.StatusOK, status)
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	for _, item := range list.Result {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		delStatus, _, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodDelete, aasEnvBaseURL+"/submodel-descriptors/"+encoded, nil)
		require.Equal(t, http.StatusNoContent, delStatus)
	}
}

func assertAASEnvAASDescriptorStatus(t *testing.T, identifier string, expectedStatus int) {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(identifier))
	status, _, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/shell-descriptors/"+encoded, nil)
	require.Equal(t, expectedStatus, status)
}

func assertAASEnvSubmodelDescriptorStatus(t *testing.T, identifier string, expectedStatus int) {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(identifier))
	status, _, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/submodel-descriptors/"+encoded, nil)
	require.Equal(t, expectedStatus, status)
}

func startAASEnvBulkAndReadHandle(t *testing.T, method string, path string, payload any) string {
	t.Helper()
	status, _, headers := doAASEnvRequest(t, aasEnvNoRedirectClient, method, aasEnvBaseURL+path, payload)
	require.Equal(t, http.StatusAccepted, status)
	location := headers.Get("Location")
	require.NotEmpty(t, location)
	handleID := location[strings.LastIndex(location, "/")+1:]
	require.NotEmpty(t, handleID)
	return handleID
}

func awaitAASEnvBulkStatusRedirect(t *testing.T, handleID string) {
	t.Helper()
	statusURL := fmt.Sprintf("%s/bulk/status/%s", aasEnvBaseURL, handleID)
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		status, _, headers := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, statusURL, nil)
		if status == http.StatusFound {
			require.Contains(t, headers.Get("Location"), "/bulk/result/")
			return
		}
		require.Equal(t, http.StatusOK, status)
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("AASENV-BULK-STATUS-TIMEOUT handle=%s", handleID)
}

func assertAASEnvBulkResultStatus(t *testing.T, handleID string, expectedStatus int) map[string]any {
	t.Helper()
	status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, fmt.Sprintf("%s/bulk/result/%s", aasEnvBaseURL, handleID), nil)
	require.Equal(t, expectedStatus, status)
	if len(body) == 0 {
		return nil
	}
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	return parsed
}

func assertAASEnvAtomicBulkFailureBody(t *testing.T, body map[string]any, requestedCount int) {
	t.Helper()
	require.False(t, body["success"].(bool))
	require.EqualValues(t, requestedCount, body["processedCount"])
	require.EqualValues(t, 0, body["successfulCount"])
	require.EqualValues(t, requestedCount, body["failedCount"])
	details, ok := body["details"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, details)
}

func toStringSliceAASEnv(t *testing.T, value any) []string {
	t.Helper()
	values, ok := value.([]any)
	require.True(t, ok)
	result := make([]string, 0, len(values))
	for _, entry := range values {
		asString, ok := entry.(string)
		require.True(t, ok)
		result = append(result, asString)
	}
	return result
}

func doAASEnvRequest(t *testing.T, client *http.Client, method string, endpoint string, payload any) (int, []byte, http.Header) {
	t.Helper()

	var body io.Reader
	if payload != nil {
		marshaled, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(marshaled)
	}

	req, err := http.NewRequest(method, endpoint, body)
	require.NoError(t, err)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, respBody, resp.Header
}
