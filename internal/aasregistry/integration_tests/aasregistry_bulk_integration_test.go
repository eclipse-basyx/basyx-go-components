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

var aasNoRedirectClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func TestBulkOperationsAtomicity(t *testing.T) {
	t.Cleanup(func() {
		deleteAllAASDescriptorsHTTP(t)
	})

	t.Run("CreateBulkAASDescriptors_Success", func(t *testing.T) {
		deleteAllAASDescriptorsHTTP(t)

		aas1 := loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_1.json")
		aas2 := loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_2.json")

		handleID := startAASBulkAndReadHandle(t, http.MethodPost, "/bulk/shell-descriptors", []any{aas1, aas2})
		awaitAASBulkStatusRedirect(t, handleID)
		assertAASBulkResultStatus(t, handleID, http.StatusNoContent)

		assertAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/22335", http.StatusOK)
		assertAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/34hzeaw", http.StatusOK)
	})

	t.Run("CreateBulkAASDescriptors_FailureRollsBack", func(t *testing.T) {
		deleteAllAASDescriptorsHTTP(t)
		createAASDescriptor(t, loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_1.json"), http.StatusCreated)

		aas3 := loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_3.json")
		aas1 := loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_1.json")

		handleID := startAASBulkAndReadHandle(t, http.MethodPost, "/bulk/shell-descriptors", []any{aas3, aas1})
		awaitAASBulkStatusRedirect(t, handleID)
		body := assertAASBulkResultStatus(t, handleID, http.StatusBadRequest)
		assertAASAtomicBulkFailureBody(t, body, 2)

		assertAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/22335", http.StatusOK)
		assertAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/3657le6d", http.StatusNotFound)
	})

	t.Run("PutBulkAASDescriptors_Success", func(t *testing.T) {
		deleteAllAASDescriptorsHTTP(t)
		createAASDescriptor(t, loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_1.json"), http.StatusCreated)

		aas1Updated := loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_5.json")
		aas4 := loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_4.json")

		handleID := startAASBulkAndReadHandle(t, http.MethodPut, "/bulk/shell-descriptors", []any{aas1Updated, aas4})
		awaitAASBulkStatusRedirect(t, handleID)
		assertAASBulkResultStatus(t, handleID, http.StatusNoContent)

		updated := getAASDescriptor(t, "https://iese.fraunhofer.de/ids/aas/22335")
		require.Equal(t, "omega", updated["assetType"])
		assertAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/w456kjw5k6z", http.StatusOK)
	})

	t.Run("PutBulkAASDescriptors_FailureRollsBack", func(t *testing.T) {
		deleteAllAASDescriptorsHTTP(t)
		createAASDescriptor(t, loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_1.json"), http.StatusCreated)

		aas1Updated := loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_5.json")
		malformed := loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_malformed.json")

		handleID := startAASBulkAndReadHandle(t, http.MethodPut, "/bulk/shell-descriptors", []any{aas1Updated, malformed})
		awaitAASBulkStatusRedirect(t, handleID)
		body := assertAASBulkResultStatus(t, handleID, http.StatusBadRequest)
		assertAASAtomicBulkFailureBody(t, body, 2)

		current := getAASDescriptor(t, "https://iese.fraunhofer.de/ids/aas/22335")
		require.Equal(t, "gigachad", current["assetType"])
	})

	t.Run("DeleteBulkAASDescriptors_Success", func(t *testing.T) {
		deleteAllAASDescriptorsHTTP(t)
		createAASDescriptor(t, loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_1.json"), http.StatusCreated)
		createAASDescriptor(t, loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_2.json"), http.StatusCreated)

		handleID := startAASBulkAndReadHandle(t, http.MethodDelete, "/bulk/shell-descriptors", []any{
			"https://iese.fraunhofer.de/ids/aas/22335",
			"https://iese.fraunhofer.de/ids/aas/34hzeaw",
		})
		awaitAASBulkStatusRedirect(t, handleID)
		assertAASBulkResultStatus(t, handleID, http.StatusNoContent)

		assertAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/22335", http.StatusNotFound)
		assertAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/34hzeaw", http.StatusNotFound)
	})

	t.Run("DeleteBulkAASDescriptors_FailureRollsBack", func(t *testing.T) {
		deleteAllAASDescriptorsHTTP(t)
		createAASDescriptor(t, loadAASJSONFixtureMap(t, "postBody/simple_ass_descriptor_1.json"), http.StatusCreated)

		handleID := startAASBulkAndReadHandle(t, http.MethodDelete, "/bulk/shell-descriptors", []any{
			"https://iese.fraunhofer.de/ids/aas/22335",
			"https://iese.fraunhofer.de/ids/aas/not-found",
		})
		awaitAASBulkStatusRedirect(t, handleID)
		body := assertAASBulkResultStatus(t, handleID, http.StatusBadRequest)
		assertAASAtomicBulkFailureBody(t, body, 2)

		assertAASDescriptorStatus(t, "https://iese.fraunhofer.de/ids/aas/22335", http.StatusOK)
	})
}

func loadAASJSONFixtureMap(t *testing.T, relativePath string) map[string]any {
	t.Helper()
	bytesPayload, err := osReadAASFile(relativePath)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(bytesPayload, &value))
	return value
}

func osReadAASFile(relativePath string) ([]byte, error) {
	file := mustOpenAAS(relativePath)
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

func mustOpenAAS(relativePath string) io.ReadCloser {
	path := filepath.Clean(relativePath)
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	return file
}

func deleteAllAASDescriptorsHTTP(t *testing.T) {
	t.Helper()
	status, body, _ := doAASRequest(t, aasNoRedirectClient, http.MethodGet, aasRegistryBaseURL+"/shell-descriptors", nil)
	require.Equal(t, http.StatusOK, status)
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	for _, item := range list.Result {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		delStatus, _, _ := doAASRequest(t, aasNoRedirectClient, http.MethodDelete, aasRegistryBaseURL+"/shell-descriptors/"+encoded, nil)
		require.Equal(t, http.StatusNoContent, delStatus)
	}
}

func createAASDescriptor(t *testing.T, payload map[string]any, expectedStatus int) {
	t.Helper()
	status, _, _ := doAASRequest(t, aasNoRedirectClient, http.MethodPost, aasRegistryBaseURL+"/shell-descriptors", payload)
	require.Equal(t, expectedStatus, status)
}

func getAASDescriptor(t *testing.T, identifier string) map[string]any {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(identifier))
	status, body, _ := doAASRequest(t, aasNoRedirectClient, http.MethodGet, aasRegistryBaseURL+"/shell-descriptors/"+encoded, nil)
	require.Equal(t, http.StatusOK, status)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func assertAASDescriptorStatus(t *testing.T, identifier string, expectedStatus int) {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(identifier))
	status, _, _ := doAASRequest(t, aasNoRedirectClient, http.MethodGet, aasRegistryBaseURL+"/shell-descriptors/"+encoded, nil)
	require.Equal(t, expectedStatus, status)
}

func startAASBulkAndReadHandle(t *testing.T, method string, path string, payload any) string {
	t.Helper()
	status, _, headers := doAASRequest(t, aasNoRedirectClient, method, aasRegistryBaseURL+path, payload)
	require.Equal(t, http.StatusAccepted, status)
	location := headers.Get("Location")
	require.NotEmpty(t, location)
	handleID := location[strings.LastIndex(location, "/")+1:]
	require.NotEmpty(t, handleID)
	return handleID
}

func awaitAASBulkStatusRedirect(t *testing.T, handleID string) {
	t.Helper()
	statusURL := fmt.Sprintf("%s/bulk/status/%s", aasRegistryBaseURL, handleID)
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		status, _, headers := doAASRequest(t, aasNoRedirectClient, http.MethodGet, statusURL, nil)
		if status == http.StatusFound {
			require.Contains(t, headers.Get("Location"), "/bulk/result/")
			return
		}
		require.Equal(t, http.StatusOK, status)
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("AASREG-BULK-STATUS-TIMEOUT handle=%s", handleID)
}

func assertAASBulkResultStatus(t *testing.T, handleID string, expectedStatus int) map[string]any {
	t.Helper()
	status, body, _ := doAASRequest(t, aasNoRedirectClient, http.MethodGet, fmt.Sprintf("%s/bulk/result/%s", aasRegistryBaseURL, handleID), nil)
	require.Equal(t, expectedStatus, status)
	if len(body) == 0 {
		return nil
	}
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	return parsed
}

func assertAASAtomicBulkFailureBody(t *testing.T, body map[string]any, requestedCount int) {
	t.Helper()
	require.False(t, body["success"].(bool))
	require.EqualValues(t, requestedCount, body["processedCount"])
	require.EqualValues(t, 0, body["successfulCount"])
	require.EqualValues(t, requestedCount, body["failedCount"])
	details, ok := body["details"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, details)
}

func doAASRequest(t *testing.T, client *http.Client, method string, endpoint string, payload any) (int, []byte, http.Header) {
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
