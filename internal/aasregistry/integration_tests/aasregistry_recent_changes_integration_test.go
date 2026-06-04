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
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

func TestAASRegistryRecentChangesAndBatchAssetKind(t *testing.T) {
	deleteAllAASDescriptorsHTTP(t)
	const changedAfter = "2029-01-01T00:00:00Z"
	descriptorID := fmt.Sprintf("https://example.com/ids/aasdesc/history-batch-%d", time.Now().UnixNano())
	encodedDescriptorID := base64.RawURLEncoding.EncodeToString([]byte(descriptorID))
	globalAssetID := "urn:example:asset:descriptor-history"
	encodedAssetID := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"name":"globalAssetId","value":%q}`, globalAssetID)))
	encodedAssetType := base64.RawURLEncoding.EncodeToString([]byte("type-v2"))
	t.Cleanup(func() {
		status, _, _ := doAASRequest(t, aasNoRedirectClient, http.MethodDelete, aasRegistryBaseURL+"/shell-descriptors/"+encodedDescriptorID, nil)
		if status != http.StatusNoContent && status != http.StatusNotFound {
			t.Logf("cleanup delete returned unexpected status=%d", status)
		}
	})

	createPayload := map[string]any{
		"id":            descriptorID,
		"idShort":       "BatchDescriptor",
		"assetKind":     "Batch",
		"assetType":     "type-v1",
		"globalAssetId": globalAssetID,
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
		"id":            descriptorID,
		"idShort":       "BatchDescriptor",
		"assetKind":     "Batch",
		"assetType":     "type-v2",
		"globalAssetId": globalAssetID,
		"administration": map[string]any{
			"createdAt": "2030-01-02T03:04:05Z",
			"updatedAt": "2030-01-02T03:04:07Z",
		},
	}
	status, body, _ = doAASRequest(t, aasNoRedirectClient, http.MethodPut, aasRegistryBaseURL+"/shell-descriptors/"+encodedDescriptorID, updatePayload)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	submodelID := fmt.Sprintf("urn:example:ids:sm-desc:history-child-%d", time.Now().UnixNano())
	submodelPayload := map[string]any{
		"id": submodelID,
		"endpoints": []any{
			map[string]any{
				"interface": "AAS-3.0",
				"protocolInformation": map[string]any{
					"href":             "https://example.com/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(submodelID)),
					"endpointProtocol": "https",
				},
			},
		},
	}
	status, body, _ = doAASRequest(t, aasNoRedirectClient, http.MethodPost, aasRegistryBaseURL+"/shell-descriptors/"+encodedDescriptorID+"/submodel-descriptors", submodelPayload)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	requireDescriptorHistoryPayloadTypes(t, descriptorID, []string{"snapshot", "diff", "diff"})

	recentURL := aasRegistryBaseURL + "/shell-descriptors/$recent-changes?limit=10&updatedFrom=" + url.QueryEscape(changedAfter)
	status, body, _ = doAASRequest(t, aasNoRedirectClient, http.MethodGet, recentURL, nil)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	payload := decodeAASRegistryMap(t, body)
	requireDescriptorWithAssetType(t, payload, descriptorID, "type-v1")
	requireDescriptorWithAssetType(t, payload, descriptorID, "type-v2")
	requireDescriptorWithSubmodel(t, payload, descriptorID, submodelID)

	filteredURL := aasRegistryBaseURL + "/shell-descriptors/$recent-changes?limit=10&assetKind=Batch&assetType=" + encodedAssetType + "&assetIds=" + url.QueryEscape(encodedAssetID)
	status, body, _ = doAASRequest(t, aasNoRedirectClient, http.MethodGet, filteredURL, nil)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	filtered := decodeAASRegistryMap(t, body)
	requireDescriptorWithAssetType(t, filtered, descriptorID, "type-v2")
	requireNoDescriptorWithAssetType(t, filtered, descriptorID, "type-v1")
}

func requireDescriptorHistoryPayloadTypes(t *testing.T, id string, expected []string) {
	t.Helper()
	db, err := sql.Open("postgres", "postgres://admin:admin123@127.0.0.1:6432/basyxTestDB?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	query, args, err := goqu.From("descriptor_history").
		Select(goqu.C("payload_type")).
		Where(goqu.C("identifier").Eq(id)).
		Order(goqu.C("history_id").Asc()).
		ToSQL()
	require.NoError(t, err)

	rows, err := db.Query(query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	actual := make([]string, 0, len(expected))
	for rows.Next() {
		var payloadType string
		require.NoError(t, rows.Scan(&payloadType))
		actual = append(actual, payloadType)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, expected, actual)
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

func requireDescriptorWithSubmodel(t *testing.T, payload map[string]any, id string, submodelID string) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "result must be an array")
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if !ok || item["id"] != id {
			continue
		}
		submodels, ok := item["submodelDescriptors"].([]any)
		if !ok {
			continue
		}
		for _, rawSubmodel := range submodels {
			submodel, ok := rawSubmodel.(map[string]any)
			if ok && submodel["id"] == submodelID {
				return
			}
		}
	}
	t.Fatalf("expected descriptor id=%s with submodel id=%s in payload: %#v", id, submodelID, payload)
}

func requireNoDescriptorWithAssetType(t *testing.T, payload map[string]any, id string, assetType string) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "result must be an array")
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if ok && item["id"] == id && item["assetType"] == assetType {
			t.Fatalf("did not expect descriptor id=%s assetType=%s in payload: %#v", id, assetType, payload)
		}
	}
}
