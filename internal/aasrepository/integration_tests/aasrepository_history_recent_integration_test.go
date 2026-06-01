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
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAASRepositoryHistoryRecentChangesAndBatchAssetKind(t *testing.T) {
	baseURL := "http://localhost:6004"
	aasID := fmt.Sprintf("https://example.com/ids/aas/history-batch-%d", time.Now().UnixNano())
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	globalAssetID := "urn:example:asset:history-batch"
	encodedAssetID := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"name":"globalAssetId","value":%q}`, globalAssetID)))

	t.Cleanup(func() {
		status, err := deleteResponseStatus(baseURL + "/shells/" + encodedAASID)
		if err != nil {
			t.Logf("cleanup delete failed: %v", err)
			return
		}
		if status != http.StatusNoContent && status != http.StatusNotFound {
			t.Logf("cleanup delete returned unexpected status=%d", status)
		}
	})

	createdAt := "2026-01-02T03:04:05Z"
	updatedAtV1 := "2026-01-02T03:04:06Z"
	createBody := fmt.Sprintf(`{
		"id": %q,
		"idShort": "HistoryBatchAAS",
		"modelType": "AssetAdministrationShell",
		"administration": {"createdAt": %q, "updatedAt": %q},
		"assetInformation": {"assetKind": "Batch", "assetType": "type-v1", "globalAssetId": %q}
	}`, aasID, createdAt, updatedAtV1, globalAssetID)

	status, err := postResponseStatus(baseURL+"/shells", createBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	current, status, err := getJSONResponse(baseURL + "/shells/" + encodedAASID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "Batch", current["assetInformation"].(map[string]any)["assetKind"])
	require.Equal(t, "type-v1", current["assetInformation"].(map[string]any)["assetType"])

	time.Sleep(30 * time.Millisecond)
	v1Date := time.Now().UTC()
	time.Sleep(30 * time.Millisecond)

	updatedAtV2 := "2026-01-02T03:04:07Z"
	updateBody := fmt.Sprintf(`{
		"id": %q,
		"idShort": "HistoryBatchAAS",
		"modelType": "AssetAdministrationShell",
		"administration": {"createdAt": %q, "updatedAt": %q},
		"assetInformation": {"assetKind": "Batch", "assetType": "type-v2", "globalAssetId": %q}
	}`, aasID, createdAt, updatedAtV2, globalAssetID)

	_, status, _, err = putJSONResponse(baseURL+"/shells/"+encodedAASID, updateBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)

	current, status, err = getJSONResponse(baseURL + "/shells/" + encodedAASID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "type-v2", current["assetInformation"].(map[string]any)["assetType"])

	time.Sleep(30 * time.Millisecond)

	childUpdateBody := fmt.Sprintf(`{"assetKind":"Batch","assetType":"type-v3-child","globalAssetId":%q}`, globalAssetID)
	_, status, _, err = putJSONResponse(baseURL+"/shells/"+encodedAASID+"/asset-information", childUpdateBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)

	current, status, err = getJSONResponse(baseURL + "/shells/" + encodedAASID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "type-v3-child", current["assetInformation"].(map[string]any)["assetType"])

	historical, status, err := getJSONResponse(fmt.Sprintf("%s/shells/%s/$history?date=%s", baseURL, encodedAASID, v1Date.Format(time.RFC3339Nano)))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "type-v1", historical["assetInformation"].(map[string]any)["assetType"])

	latestHistorical, status, err := getJSONResponse(fmt.Sprintf("%s/shells/%s/$history?date=%s", baseURL, encodedAASID, time.Now().UTC().Format(time.RFC3339Nano)))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "type-v3-child", latestHistorical["assetInformation"].(map[string]any)["assetType"])

	recent, status, err := getJSONResponse(baseURL + "/shells/$recent-changes?limit=10")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	requireRecentChangesForID(t, recent, aasID, 3)
	requireRecentChangeTypeForID(t, recent, aasID, "Created")
	requireRecentChangeTypeForID(t, recent, aasID, "Updated")

	recent, status, err = getJSONResponse(baseURL + "/shells/$recent-changes?limit=10&assetIds=" + url.QueryEscape(encodedAssetID))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	requireRecentChangesForID(t, recent, aasID, 3)

	status, err = deleteResponseStatus(baseURL + "/shells/" + encodedAASID)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)

	status, err = getResponseStatus(fmt.Sprintf("%s/shells/%s/$history?date=%s", baseURL, encodedAASID, time.Now().UTC().Format(time.RFC3339Nano)))
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, status)

	recent, status, err = getJSONResponse(baseURL + "/shells/$recent-changes?limit=10")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	requireRecentChangesForID(t, recent, aasID, 4)
	requireRecentChangeTypeForID(t, recent, aasID, "Deleted")
}

func requireRecentChangesForID(t *testing.T, payload map[string]any, id string, minimumCount int) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "recent changes result must be an array")
	count := 0
	sawGlobalAssetID := false
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if item["id"] == id {
			require.NotEmpty(t, item["type"])
			require.NotEmpty(t, item["createdAt"])
			require.NotEmpty(t, item["updatedAt"])
			if item["globalAssetId"] != nil {
				sawGlobalAssetID = true
			}
			count++
		}
	}
	require.GreaterOrEqual(t, count, minimumCount, "recent changes payload: %#v", payload)
	require.True(t, sawGlobalAssetID, "expected AAS asset metadata in recent changes payload: %#v", payload)
}

func requireRecentChangeTypeForID(t *testing.T, payload map[string]any, id string, changeType string) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "recent changes result must be an array")
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if ok && item["id"] == id && item["type"] == changeType {
			return
		}
	}
	t.Fatalf("expected recent change id=%s type=%s in payload: %#v", id, changeType, payload)
}
