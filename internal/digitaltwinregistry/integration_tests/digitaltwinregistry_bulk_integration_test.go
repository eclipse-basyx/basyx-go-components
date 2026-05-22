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
package bench

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	dtrTokenURL = "http://127.0.0.1:8080/realms/basyx/protocol/openid-connect/token"
	dtrClientID = "basyx-ui"
)

var dtrNoRedirectClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func TestBulkAASOperationsAndDescription(t *testing.T) {
	token := fetchDTRToken(t, "admin", "pwd")
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Edc-Bpn":       "BPNL00000000015G",
	}

	t.Cleanup(func() {
		deleteAllDTRShellDescriptors(t, headers)
	})

	t.Run("BulkCreateSuccessAndRetryAfter", func(t *testing.T) {
		deleteAllDTRShellDescriptors(t, headers)

		desc1 := loadDTRFixtureMap(t, "postBody/aas_shell.json")
		desc2 := deepCopyMap(desc1)
		desc1["id"] = "urn:example:dtr:bulk-aas-1"
		desc2["id"] = "urn:example:dtr:bulk-aas-2"
		desc1["idShort"] = "dtrBulkAas1"
		desc2["idShort"] = "dtrBulkAas2"
		setNestedSubmodelID(desc1, "urn:example:dtr:bulk-sm-1")
		setNestedSubmodelID(desc2, "urn:example:dtr:bulk-sm-2")

		handleID := startDTRBulkAndReadHandle(t, http.MethodPost, "/bulk/shell-descriptors", []any{desc1, desc2}, headers)
		runningSeen := awaitDTRBulkStatus(t, handleID, headers)
		require.True(t, runningSeen, "expected to observe at least one running status response")
		assertDTRBulkResultStatus(t, handleID, http.StatusNoContent, headers)

		assertDTRShellStatus(t, "urn:example:dtr:bulk-aas-1", http.StatusOK, headers)
		assertDTRShellStatus(t, "urn:example:dtr:bulk-aas-2", http.StatusOK, headers)
	})

	t.Run("BulkCreateFailureRollsBack", func(t *testing.T) {
		deleteAllDTRShellDescriptors(t, headers)

		existing := loadDTRFixtureMap(t, "postBody/aas_shell.json")
		existing["id"] = "urn:example:dtr:bulk-conflict"
		existing["idShort"] = "dtrBulkConflict"
		setNestedSubmodelID(existing, "urn:example:dtr:bulk-conflict-sm")
		createDTRShellDescriptor(t, existing, http.StatusCreated, headers)

		descNew := deepCopyMap(existing)
		descNew["id"] = "urn:example:dtr:bulk-new"
		descNew["idShort"] = "dtrBulkNew"
		setNestedSubmodelID(descNew, "urn:example:dtr:bulk-new-sm")

		descConflict := deepCopyMap(existing)

		handleID := startDTRBulkAndReadHandle(t, http.MethodPost, "/bulk/shell-descriptors", []any{descNew, descConflict}, headers)
		_ = awaitDTRBulkStatus(t, handleID, headers)
		body := assertDTRBulkResultStatus(t, handleID, http.StatusBadRequest, headers)
		assertAtomicBulkFailureBody(t, body, 2)

		assertDTRShellStatus(t, "urn:example:dtr:bulk-conflict", http.StatusOK, headers)
		assertDTRShellStatus(t, "urn:example:dtr:bulk-new", http.StatusNotFound, headers)
	})

	t.Run("LookupByAssetLinkRemainsConstrainedWithABAC", func(t *testing.T) {
		deleteAllDTRShellDescriptors(t, headers)

		desc := loadDTRFixtureMap(t, "postBody/aas_shell.json")
		desc["id"] = "urn:example:dtr:lookup-constrained"
		desc["idShort"] = "dtrLookupConstrained"
		setNestedSubmodelID(desc, "urn:example:dtr:lookup-constrained-sm")
		createDTRShellDescriptor(t, desc, http.StatusCreated, headers)

		adminBPNHeaders := map[string]string{
			"Edc-Bpn": "BPN_COMPANY_001",
		}

		nonMatching := loadDTRFixtureArray(t, "postBody/num1AssetLink.json")
		nonMatchingBody := lookupShellsByAssetLink(t, nonMatching, adminBPNHeaders)
		assertLookupResultIDs(t, nonMatchingBody)

		matching := loadDTRFixtureArray(t, "postBody/num2AssetLink.json")
		matchingBody := lookupShellsByAssetLink(t, matching, adminBPNHeaders)
		assertLookupResultIDs(t, matchingBody, "urn:example:dtr:lookup-constrained")
	})
}

func fetchDTRToken(t *testing.T, user, password string) string {
	t.Helper()
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", dtrClientID)
	form.Set("username", user)
	form.Set("password", password)

	req, err := http.NewRequest(http.MethodPost, dtrTokenURL, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := dtrNoRedirectClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var tokenResp map[string]any
	require.NoError(t, json.Unmarshal(body, &tokenResp))
	token, ok := tokenResp["access_token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, token)
	return token
}

func deleteAllDTRShellDescriptors(t *testing.T, headers map[string]string) {
	t.Helper()
	status, body, _ := doDTRRequest(t, dtrNoRedirectClient, http.MethodGet, BaseURL+"/shell-descriptors?limit=300", nil, headers)
	require.Equal(t, http.StatusOK, status)
	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	for _, item := range list.Result {
		enc := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		delStatus, _, _ := doDTRRequest(t, dtrNoRedirectClient, http.MethodDelete, BaseURL+"/shell-descriptors/"+enc, nil, headers)
		require.Equal(t, http.StatusNoContent, delStatus)
	}
}

func createDTRShellDescriptor(t *testing.T, payload map[string]any, expectedStatus int, headers map[string]string) {
	t.Helper()
	status, _, _ := doDTRRequest(t, dtrNoRedirectClient, http.MethodPost, BaseURL+"/shell-descriptors", payload, headers)
	require.Equal(t, expectedStatus, status)
}

func assertDTRShellStatus(t *testing.T, identifier string, expectedStatus int, headers map[string]string) {
	t.Helper()
	enc := base64.RawURLEncoding.EncodeToString([]byte(identifier))
	status, _, _ := doDTRRequest(t, dtrNoRedirectClient, http.MethodGet, BaseURL+"/shell-descriptors/"+enc, nil, headers)
	require.Equal(t, expectedStatus, status)
}

func startDTRBulkAndReadHandle(t *testing.T, method string, path string, payload any, headers map[string]string) string {
	t.Helper()
	status, _, responseHeaders := doDTRRequest(t, dtrNoRedirectClient, method, BaseURL+path, payload, headers)
	require.Equal(t, http.StatusAccepted, status)
	location := responseHeaders.Get("Location")
	require.NotEmpty(t, location)
	handleID := location[strings.LastIndex(location, "/")+1:]
	require.NotEmpty(t, handleID)
	return handleID
}

func awaitDTRBulkStatus(t *testing.T, handleID string, headers map[string]string) bool {
	t.Helper()
	statusURL := fmt.Sprintf("%s/bulk/status/%s", BaseURL, handleID)
	deadline := time.Now().Add(10 * time.Second)
	seenRunning := false

	for time.Now().Before(deadline) {
		status, body, respHeaders := doDTRRequest(t, dtrNoRedirectClient, http.MethodGet, statusURL, nil, headers)
		if status == http.StatusFound {
			require.Contains(t, respHeaders.Get("Location"), "/bulk/result/")
			return seenRunning
		}
		require.Equal(t, http.StatusOK, status)
		seenRunning = true
		require.NotEmpty(t, respHeaders.Get("Retry-After"))

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(body, &parsed))
		_, exists := parsed["retryAfter"]
		require.False(t, exists, "retryAfter must be set as header only")
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("DTR-BULK-STATUS-TIMEOUT handle=%s", handleID)
	return false
}

func assertDTRBulkResultStatus(t *testing.T, handleID string, expectedStatus int, headers map[string]string) map[string]any {
	t.Helper()
	status, body, _ := doDTRRequest(t, dtrNoRedirectClient, http.MethodGet, fmt.Sprintf("%s/bulk/result/%s", BaseURL, handleID), nil, headers)
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

func loadDTRFixtureMap(t *testing.T, relativePath string) map[string]any {
	t.Helper()
	path := filepath.Clean(relativePath)
	bytesPayload, err := os.ReadFile(path)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(bytesPayload, &value))
	return value
}

func loadDTRFixtureArray(t *testing.T, relativePath string) []any {
	t.Helper()
	path := filepath.Clean(relativePath)
	bytesPayload, err := os.ReadFile(path)
	require.NoError(t, err)
	var value []any
	require.NoError(t, json.Unmarshal(bytesPayload, &value))
	return value
}

func deepCopyMap(input map[string]any) map[string]any {
	raw, _ := json.Marshal(input)
	var copied map[string]any
	_ = json.Unmarshal(raw, &copied)
	return copied
}

func setNestedSubmodelID(descriptor map[string]any, submodelID string) {
	submodels, ok := descriptor["submodelDescriptors"].([]any)
	if !ok || len(submodels) == 0 {
		return
	}
	first, ok := submodels[0].(map[string]any)
	if !ok {
		return
	}
	first["id"] = submodelID
}

func lookupShellsByAssetLink(t *testing.T, payload []any, headers map[string]string) map[string]any {
	t.Helper()
	status, body, _ := doDTRRequest(t, dtrNoRedirectClient, http.MethodPost, BaseURL+"/lookup/shellsByAssetLink", payload, headers)
	require.Equal(t, http.StatusOK, status)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	return parsed
}

func assertLookupResultIDs(t *testing.T, body map[string]any, expectedIDs ...string) {
	t.Helper()
	resultRaw, hasResult := body["result"]
	if len(expectedIDs) == 0 {
		require.False(t, hasResult)
		return
	}

	require.True(t, hasResult)
	result, ok := resultRaw.([]any)
	require.True(t, ok)
	require.Len(t, result, len(expectedIDs))
	for idx, expectedID := range expectedIDs {
		actualID, ok := result[idx].(string)
		require.True(t, ok)
		require.Equal(t, expectedID, actualID)
	}
}

func doDTRRequest(t *testing.T, client *http.Client, method string, endpoint string, payload any, headers map[string]string) (int, []byte, http.Header) {
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
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, respBody, resp.Header
}
