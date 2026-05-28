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

//nolint:all
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAASRegistryRecentChangesAndBatchAssetKind(t *testing.T) {
	deleteAllAASDescriptorsHTTP(t)
	const changedAfter = "2029-01-01T00:00:00Z"
	descriptorID := fmt.Sprintf("https://example.com/ids/aasdesc/history-batch-%d", time.Now().UnixNano())
	encodedDescriptorID := base64.RawURLEncoding.EncodeToString([]byte(descriptorID))
	t.Cleanup(func() {
		status, _, _ := doAASRequest(t, aasNoRedirectClient, http.MethodDelete, aasRegistryBaseURL+"/shell-descriptors/"+encodedDescriptorID, nil)
		if status != http.StatusNoContent && status != http.StatusNotFound {
			t.Logf("cleanup delete returned unexpected status=%d", status)
		}
	})

	createPayload := map[string]any{
		"id":        descriptorID,
		"idShort":   "BatchDescriptor",
		"assetKind": "Batch",
		"assetType": "type-v1",
		"administration": map[string]any{
			"createdAt": "2030-01-02T03:04:05Z",
			"updatedAt": "2030-01-02T03:04:06Z",
		},
	}

	status, body, _ := doAASRequest(t, aasNoRedirectClient, http.MethodPost, aasRegistryBaseURL+"/shell-descriptors", createPayload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))

	created := getAASDescriptor(t, descriptorID)
	require.Equal(t, "Batch", created["assetKind"])

	updatePayload := map[string]any{
		"id":        descriptorID,
		"idShort":   "BatchDescriptor",
		"assetKind": "Batch",
		"assetType": "type-v2",
		"administration": map[string]any{
			"createdAt": "2030-01-02T03:04:05Z",
			"updatedAt": "2030-01-02T03:04:07Z",
		},
	}
	status, body, _ = doAASRequest(t, aasNoRedirectClient, http.MethodPut, aasRegistryBaseURL+"/shell-descriptors/"+encodedDescriptorID, updatePayload)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	recentURL := aasRegistryBaseURL + "/shell-descriptors/$recent-changes?limit=10&updatedFrom=" + url.QueryEscape(changedAfter)
	status, body, _ = doAASRequest(t, aasNoRedirectClient, http.MethodGet, recentURL, nil)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	payload := decodeAASRegistryMap(t, body)
	requireDescriptorWithAssetType(t, payload, descriptorID, "type-v1")
	requireDescriptorWithAssetType(t, payload, descriptorID, "type-v2")
}

func decodeAASRegistryMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func requireDescriptorWithAssetType(t *testing.T, payload map[string]any, id string, assetType string) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "result must be an array")
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if item["id"] == id && item["assetType"] == assetType {
			return
		}
	}
	t.Fatalf("expected descriptor id=%s assetType=%s in payload: %#v", id, assetType, payload)
}
