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

const smRegistryBaseURL = "http://127.0.0.1:6004"

var smNoRedirectClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func TestBulkOperationsAtomicity(t *testing.T) {
	t.Cleanup(func() {
		deleteAllSubmodelDescriptorsHTTP(t)
	})

	t.Run("CreateBulkSubmodelDescriptors_Success", func(t *testing.T) {
		deleteAllSubmodelDescriptorsHTTP(t)

		smd1 := loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_1.json")
		smd2 := loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_2.json")

		handleID := startBulkAndReadHandle(t, http.MethodPost, "/bulk/submodel-descriptors", []any{smd1, smd2})
		awaitBulkStatusRedirect(t, handleID)
		assertBulkResultStatus(t, handleID, http.StatusNoContent)

		assertSubmodelDescriptorStatus(t, "urn:example:sm:001", http.StatusOK)
		assertSubmodelDescriptorStatus(t, "urn:example:sm:002", http.StatusOK)
	})

	t.Run("CreateBulkSubmodelDescriptors_FailureRollsBack", func(t *testing.T) {
		deleteAllSubmodelDescriptorsHTTP(t)
		createSubmodelDescriptor(t, loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_1.json"), http.StatusCreated)

		smd3 := loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_3.json")
		smd1 := loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_1.json")

		handleID := startBulkAndReadHandle(t, http.MethodPost, "/bulk/submodel-descriptors", []any{smd3, smd1})
		awaitBulkStatusRedirect(t, handleID)
		body := assertBulkResultStatus(t, handleID, http.StatusBadRequest)
		assertAtomicBulkFailureBody(t, body, 2)

		assertSubmodelDescriptorStatus(t, "urn:example:sm:001", http.StatusOK)
		assertSubmodelDescriptorStatus(t, "urn:example:sm:003", http.StatusNotFound)
	})

	t.Run("PutBulkSubmodelDescriptors_Success", func(t *testing.T) {
		deleteAllSubmodelDescriptorsHTTP(t)
		createSubmodelDescriptor(t, loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_2.json"), http.StatusCreated)

		smd2Updated := loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_2_updated.json")
		smd3 := loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_3.json")

		handleID := startBulkAndReadHandle(t, http.MethodPut, "/bulk/submodel-descriptors", []any{smd2Updated, smd3})
		awaitBulkStatusRedirect(t, handleID)
		assertBulkResultStatus(t, handleID, http.StatusNoContent)

		updated := getSubmodelDescriptor(t, "urn:example:sm:002")
		require.Equal(t, "v2-updated", updated["extensions"].([]any)[0].(map[string]any)["value"])
		assertSubmodelDescriptorStatus(t, "urn:example:sm:003", http.StatusOK)
	})

	t.Run("PutBulkSubmodelDescriptors_FailureRollsBack", func(t *testing.T) {
		deleteAllSubmodelDescriptorsHTTP(t)
		createSubmodelDescriptor(t, loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_2.json"), http.StatusCreated)

		smd2Updated := loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_2_updated.json")
		malformed := loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_malformed.json")

		handleID := startBulkAndReadHandle(t, http.MethodPut, "/bulk/submodel-descriptors", []any{smd2Updated, malformed})
		awaitBulkStatusRedirect(t, handleID)
		body := assertBulkResultStatus(t, handleID, http.StatusBadRequest)
		assertAtomicBulkFailureBody(t, body, 2)

		current := getSubmodelDescriptor(t, "urn:example:sm:002")
		require.Equal(t, "v2", current["extensions"].([]any)[0].(map[string]any)["value"])
	})

	t.Run("DeleteBulkSubmodelDescriptors_Success", func(t *testing.T) {
		deleteAllSubmodelDescriptorsHTTP(t)
		createSubmodelDescriptor(t, loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_1.json"), http.StatusCreated)
		createSubmodelDescriptor(t, loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_2.json"), http.StatusCreated)

		handleID := startBulkAndReadHandle(t, http.MethodDelete, "/bulk/submodel-descriptors", []any{"urn:example:sm:001", "urn:example:sm:002"})
		awaitBulkStatusRedirect(t, handleID)
		assertBulkResultStatus(t, handleID, http.StatusNoContent)

		assertSubmodelDescriptorStatus(t, "urn:example:sm:001", http.StatusNotFound)
		assertSubmodelDescriptorStatus(t, "urn:example:sm:002", http.StatusNotFound)
	})

	t.Run("DeleteBulkSubmodelDescriptors_FailureRollsBack", func(t *testing.T) {
		deleteAllSubmodelDescriptorsHTTP(t)
		createSubmodelDescriptor(t, loadJSONFixtureMap(t, "postBody/simple_submodel_descriptor_1.json"), http.StatusCreated)

		handleID := startBulkAndReadHandle(t, http.MethodDelete, "/bulk/submodel-descriptors", []any{"urn:example:sm:001", "urn:example:sm:not-found"})
		awaitBulkStatusRedirect(t, handleID)
		body := assertBulkResultStatus(t, handleID, http.StatusBadRequest)
		assertAtomicBulkFailureBody(t, body, 2)

		assertSubmodelDescriptorStatus(t, "urn:example:sm:001", http.StatusOK)
	})
}

func loadJSONFixtureMap(t *testing.T, relativePath string) map[string]any {
	t.Helper()
	bytesPayload, err := osReadFile(relativePath)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(bytesPayload, &value))
	return value
}

func osReadFile(relativePath string) ([]byte, error) {
	file := mustOpen(relativePath)
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

func mustOpen(relativePath string) io.ReadCloser {
	path := filepath.Clean(relativePath)
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	return file
}

func deleteAllSubmodelDescriptorsHTTP(t *testing.T) {
	t.Helper()
	status, body, _ := doRequest(t, smNoRedirectClient, http.MethodGet, smRegistryBaseURL+"/submodel-descriptors", nil)
	require.Equal(t, http.StatusOK, status)
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	for _, item := range list.Result {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		delStatus, _, _ := doRequest(t, smNoRedirectClient, http.MethodDelete, smRegistryBaseURL+"/submodel-descriptors/"+encoded, nil)
		require.Equal(t, http.StatusNoContent, delStatus)
	}
}

func createSubmodelDescriptor(t *testing.T, payload map[string]any, expectedStatus int) {
	t.Helper()
	status, _, _ := doRequest(t, smNoRedirectClient, http.MethodPost, smRegistryBaseURL+"/submodel-descriptors", payload)
	require.Equal(t, expectedStatus, status)
}

func getSubmodelDescriptor(t *testing.T, identifier string) map[string]any {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(identifier))
	status, body, _ := doRequest(t, smNoRedirectClient, http.MethodGet, smRegistryBaseURL+"/submodel-descriptors/"+encoded, nil)
	require.Equal(t, http.StatusOK, status)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func assertSubmodelDescriptorStatus(t *testing.T, identifier string, expectedStatus int) {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(identifier))
	status, _, _ := doRequest(t, smNoRedirectClient, http.MethodGet, smRegistryBaseURL+"/submodel-descriptors/"+encoded, nil)
	require.Equal(t, expectedStatus, status)
}

func startBulkAndReadHandle(t *testing.T, method string, path string, payload any) string {
	t.Helper()
	status, _, headers := doRequest(t, smNoRedirectClient, method, smRegistryBaseURL+path, payload)
	require.Equal(t, http.StatusAccepted, status)
	location := headers.Get("Location")
	require.NotEmpty(t, location)
	handleID := location[strings.LastIndex(location, "/")+1:]
	require.NotEmpty(t, handleID)
	return handleID
}

func awaitBulkStatusRedirect(t *testing.T, handleID string) {
	t.Helper()
	statusURL := fmt.Sprintf("%s/bulk/status/%s", smRegistryBaseURL, handleID)
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		status, _, headers := doRequest(t, smNoRedirectClient, http.MethodGet, statusURL, nil)
		if status == http.StatusFound {
			require.Contains(t, headers.Get("Location"), "/bulk/result/")
			return
		}
		require.Equal(t, http.StatusOK, status)
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("SMREG-BULK-STATUS-TIMEOUT handle=%s", handleID)
}

func assertBulkResultStatus(t *testing.T, handleID string, expectedStatus int) map[string]any {
	t.Helper()
	status, body, _ := doRequest(t, smNoRedirectClient, http.MethodGet, fmt.Sprintf("%s/bulk/result/%s", smRegistryBaseURL, handleID), nil)
	require.Equal(t, expectedStatus, status)
	if len(body) == 0 {
		return nil
	}
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	return parsed
}

func assertAtomicBulkFailureBody(t *testing.T, body map[string]any, requestedCount int) {
	t.Helper()
	require.False(t, body["success"].(bool))
	require.EqualValues(t, requestedCount, body["processedCount"])
	require.EqualValues(t, 0, body["successfulCount"])
	require.EqualValues(t, requestedCount, body["failedCount"])
	details, ok := body["details"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, details)
}

func doRequest(t *testing.T, client *http.Client, method string, endpoint string, payload any) (int, []byte, http.Header) {
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
