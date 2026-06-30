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

func TestAASEnvironmentDelegatesHistoryRecentChangesAndBatchAssetKind(t *testing.T) {
	resetDatabase(t)

	aasID := fmt.Sprintf("https://example.com/ids/aasenv/history-batch-%d", time.Now().UnixNano())
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	submodelID := fmt.Sprintf("urn:example:aasenv:submodel:history:%d", time.Now().UnixNano())
	encodedSubmodelID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	descriptorID := fmt.Sprintf("https://example.com/ids/aasenv/descriptor/recent-%d", time.Now().UnixNano())
	encodedDescriptorID := base64.RawURLEncoding.EncodeToString([]byte(descriptorID))

	t.Cleanup(func() {
		doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodDelete, aasEnvBaseURL+"/shells/"+encodedAASID, nil)
		doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodDelete, aasEnvBaseURL+"/submodels/"+encodedSubmodelID, nil)
		doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodDelete, aasEnvBaseURL+"/shell-descriptors/"+encodedDescriptorID, nil)
	})

	aasPayload := map[string]any{
		"id":        aasID,
		"idShort":   "AASEnvHistoryBatch",
		"modelType": "AssetAdministrationShell",
		"administration": map[string]any{
			"createdAt": "2026-01-02T03:04:05Z",
			"updatedAt": "2026-01-02T03:04:06Z",
		},
		"assetInformation": map[string]any{
			"assetKind": "Batch",
			"assetType": "env-type-v1",
		},
	}
	status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/shells", aasPayload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))

	time.Sleep(30 * time.Millisecond)
	aasV1Date := time.Now().UTC()
	time.Sleep(30 * time.Millisecond)

	aasPayload["assetInformation"].(map[string]any)["assetType"] = "env-type-v2"
	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPut, aasEnvBaseURL+"/shells/"+encodedAASID, aasPayload)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, fmt.Sprintf("%s/shells/%s/$history?date=%s", aasEnvBaseURL, encodedAASID, aasV1Date.Format(time.RFC3339Nano)), nil)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	historicalAAS := decodeAASEnvMap(t, body)
	require.Equal(t, "Batch", historicalAAS["assetInformation"].(map[string]any)["assetKind"])
	require.Equal(t, "env-type-v1", historicalAAS["assetInformation"].(map[string]any)["assetType"])

	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/shells/$recent-changes?limit=10", nil)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	requireAASEnvRecentChange(t, decodeAASEnvMap(t, body), aasID)

	submodelPayload := map[string]any{
		"id":        submodelID,
		"idShort":   "AASEnvHistorySubmodel",
		"kind":      "Instance",
		"modelType": "Submodel",
		"administration": map[string]any{
			"createdAt": "2026-01-02T03:04:05Z",
			"updatedAt": "2026-01-02T03:04:06Z",
		},
	}
	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/submodels", submodelPayload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/submodels/$recent-changes?limit=10", nil)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	requireAASEnvRecentChange(t, decodeAASEnvMap(t, body), submodelID)

	descriptorPayload := map[string]any{
		"id":             descriptorID,
		"idShort":        "AASEnvDescriptorRecent",
		"assetKind":      "Batch",
		"assetType":      "descriptor-type-v1",
		"administration": map[string]any{"createdAt": "2026-01-02T03:04:05Z", "updatedAt": "2026-01-02T03:04:06Z"},
	}
	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/shell-descriptors", descriptorPayload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/shell-descriptors?limit=10&updatedFrom="+url.QueryEscape("2026-01-02T03:04:06Z"), nil)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	requireAASEnvDescriptor(t, decodeAASEnvMap(t, body), descriptorID, "descriptor-type-v1")
}

func TestAASEnvironmentHistoryAllowsAddingIDShortAfterCreate(t *testing.T) {
	resetDatabase(t)

	aasID := fmt.Sprintf("https://example.com/ids/aasenv/history-add-idshort-%d", time.Now().UnixNano())
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))

	t.Cleanup(func() {
		doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodDelete, aasEnvBaseURL+"/shells/"+encodedAASID, nil)
		doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodDelete, aasEnvBaseURL+"/shell-descriptors/"+encodedAASID, nil)
	})

	aasPayload := map[string]any{
		"id":        aasID,
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind": "Instance",
		},
	}
	status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/shells", aasPayload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))

	aasPayload["idShort"] = "AddedLater"
	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPut, aasEnvBaseURL+"/shells/"+encodedAASID, aasPayload)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, aasEnvBaseURL+"/shells/"+encodedAASID, nil)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	current := decodeAASEnvMap(t, body)
	require.Equal(t, "AddedLater", current["idShort"])
}

func decodeAASEnvMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func requireAASEnvRecentChange(t *testing.T, payload map[string]any, id string) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "result must be an array")
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if item["id"] == id {
			require.NotContains(t, item, "type")
			require.NotEmpty(t, item["createdAt"])
			require.NotEmpty(t, item["updatedAt"])
			return
		}
	}
	t.Fatalf("expected recent change id=%s in payload: %#v", id, payload)
}

func requireAASEnvDescriptor(t *testing.T, payload map[string]any, id string, assetType string) {
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
