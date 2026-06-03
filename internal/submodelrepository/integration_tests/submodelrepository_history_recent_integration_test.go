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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubmodelRepositoryHistoryTracksSubmodelElementChangesAndRecentDeletes(t *testing.T) {
	baseURL := "http://localhost:6004"
	submodelID := fmt.Sprintf("urn:example:sm:history:%d", time.Now().UnixNano())
	encodedSubmodelID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	semanticID := "urn:example:semantic:history"
	supplementalSemanticID := "urn:example:semantic:history:supplemental"
	encodedSemanticID := base64.RawURLEncoding.EncodeToString([]byte(semanticID))

	t.Cleanup(func() {
		status, _, err := requestJSON(http.MethodDelete, baseURL+"/submodels/"+encodedSubmodelID, nil)
		if err != nil {
			t.Logf("cleanup delete failed: %v", err)
			return
		}
		if status != http.StatusNoContent && status != http.StatusNotFound {
			t.Logf("cleanup delete returned unexpected status=%d", status)
		}
	})

	createPayload := map[string]any{
		"id":        submodelID,
		"idShort":   "HistorySubmodel",
		"kind":      "Instance",
		"modelType": "Submodel",
		"administration": map[string]any{
			"createdAt": "2026-01-02T03:04:05Z",
			"updatedAt": "2026-01-02T03:04:06Z",
		},
		"semanticId": map[string]any{
			"type": "ModelReference",
			"keys": []any{map[string]any{"type": "GlobalReference", "value": semanticID}},
		},
		"supplementalSemanticIds": []any{map[string]any{
			"type": "ExternalReference",
			"keys": []any{map[string]any{"type": "GlobalReference", "value": supplementalSemanticID}},
		}},
		"submodelElements": []any{
			map[string]any{
				"modelType": "Property",
				"idShort":   "Temperature",
				"valueType": "xs:string",
				"value":     "v1",
			},
			map[string]any{
				"modelType":   "File",
				"idShort":     "Attachment",
				"contentType": "application/octet-stream",
				"value":       "",
			},
		},
	}

	status, body, err := requestJSON(http.MethodPost, baseURL+"/submodels", createPayload)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))

	time.Sleep(30 * time.Millisecond)
	v1Date := time.Now().UTC()
	time.Sleep(30 * time.Millisecond)

	updatedElement := map[string]any{
		"modelType": "Property",
		"idShort":   "Temperature",
		"valueType": "xs:string",
		"value":     "v2-from-sme",
	}
	status, body, err = requestJSON(http.MethodPut, baseURL+"/submodels/"+encodedSubmodelID+"/submodel-elements/Temperature", updatedElement)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	status, body, err = requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s", baseURL, encodedSubmodelID, v1Date.Format(time.RFC3339Nano)), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	historical := decodeMap(t, body)
	require.Equal(t, "v1", getPropertyValueByIDShort(t, historical, "Temperature"))

	status, body, err = requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s", baseURL, encodedSubmodelID, time.Now().UTC().Format(time.RFC3339Nano)), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	require.Equal(t, "v2-from-sme", getPropertyValueByIDShort(t, decodeMap(t, body), "Temperature"))

	status, body, err = requestJSON(http.MethodPatch, baseURL+"/submodels/"+encodedSubmodelID+"/submodel-elements/Temperature/$value", "v3-from-value-only")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	status, body, err = requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s", baseURL, encodedSubmodelID, time.Now().UTC().Format(time.RFC3339Nano)), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	require.Equal(t, "v3-from-value-only", getPropertyValueByIDShort(t, decodeMap(t, body), "Temperature"))

	attachmentEndpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/Attachment/attachment", baseURL, encodedSubmodelID)
	statusCode, uploadErr := uploadFileAttachment(attachmentEndpoint, "testFiles/marcus.gif", "marcus.gif")
	require.NoError(t, uploadErr)
	require.Equal(t, http.StatusNoContent, statusCode)

	status, body, err = requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s", baseURL, encodedSubmodelID, time.Now().UTC().Format(time.RFC3339Nano)), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	uploadedFile := getElementByIDShort(t, decodeMap(t, body), "Attachment")
	require.NotEmpty(t, uploadedFile["value"])
	require.Equal(t, "image/gif", uploadedFile["contentType"])

	status, body, err = requestJSON(http.MethodDelete, attachmentEndpoint, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))

	status, body, err = requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s", baseURL, encodedSubmodelID, time.Now().UTC().Format(time.RFC3339Nano)), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	deletedFile := getElementByIDShort(t, decodeMap(t, body), "Attachment")
	require.Empty(t, deletedFile["value"])

	status, body, err = requestJSON(http.MethodGet, baseURL+"/submodels/$recent-changes?limit=10", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	recent := decodeMap(t, body)
	requireRecentChangesForIDSubmodel(t, recent, submodelID, 5)
	requireRecentSubmodelReference(t, recent, submodelID, "semanticId", semanticID)
	requireRecentSubmodelReference(t, recent, submodelID, "supplementalSemanticIds", supplementalSemanticID)
	requireRecentChangeTypeForIDSubmodel(t, recent, submodelID, "Created")
	requireRecentChangeTypeForIDSubmodel(t, recent, submodelID, "Updated")

	status, body, err = requestJSON(http.MethodGet, baseURL+"/submodels/$recent-changes?semanticId="+encodedSemanticID+"&limit=10", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	requireRecentChangesForIDSubmodel(t, decodeMap(t, body), submodelID, 5)

	status, body, err = requestJSON(http.MethodDelete, baseURL+"/submodels/"+encodedSubmodelID, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	status, body, err = requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s", baseURL, encodedSubmodelID, time.Now().UTC().Format(time.RFC3339Nano)), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, status, "response=%s", string(body))

	status, body, err = requestJSON(http.MethodGet, baseURL+"/submodels/$recent-changes?limit=1", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	recentPage := decodeMap(t, body)
	requireFirstRecentChangeForIDSubmodel(t, recentPage, submodelID, "Deleted")
	nextCursor := requireRecentCursor(t, recentPage)

	status, body, err = requestJSON(http.MethodGet, baseURL+"/submodels/$recent-changes?limit=1&cursor="+nextCursor, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	requireFirstRecentChangeForIDSubmodel(t, decodeMap(t, body), submodelID, "Updated")

	status, body, err = requestJSON(http.MethodGet, baseURL+"/submodels/$recent-changes?limit=10", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	recent = decodeMap(t, body)
	requireRecentChangesForIDSubmodel(t, recent, submodelID, 6)
	requireRecentChangeTypeForIDSubmodel(t, recent, submodelID, "Deleted")
}

func decodeMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func requireRecentChangesForIDSubmodel(t *testing.T, payload map[string]any, id string, minimumCount int) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "recent changes result must be an array")
	count := 0
	sawSemanticID := false
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if item["id"] == id {
			require.NotEmpty(t, item["type"])
			require.NotEmpty(t, item["createdAt"])
			require.NotEmpty(t, item["updatedAt"])
			if item["semanticId"] != nil {
				sawSemanticID = true
			}
			count++
		}
	}
	require.GreaterOrEqual(t, count, minimumCount, "recent changes payload: %#v", payload)
	require.True(t, sawSemanticID, "expected Submodel semantic metadata in recent changes payload: %#v", payload)
}

func requireRecentChangeTypeForIDSubmodel(t *testing.T, payload map[string]any, id string, changeType string) {
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

func requireFirstRecentChangeForIDSubmodel(t *testing.T, payload map[string]any, id string, changeType string) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "recent changes result must be an array")
	require.NotEmpty(t, result, "recent changes result must not be empty")
	item, ok := result[0].(map[string]any)
	require.True(t, ok, "first recent change must be an object")
	require.Equal(t, id, item["id"], "recent changes must return newest matching change first: %#v", payload)
	require.Equal(t, changeType, item["type"], "recent changes must return newest matching change first: %#v", payload)
}

func requireRecentCursor(t *testing.T, payload map[string]any) string {
	t.Helper()
	pagingMetadata, ok := payload["paging_metadata"].(map[string]any)
	require.True(t, ok, "recent changes paging_metadata must be an object")
	cursor, ok := pagingMetadata["cursor"].(string)
	require.True(t, ok, "recent changes cursor must be a string")
	require.NotEmpty(t, cursor, "recent changes cursor must not be empty when more pages exist")
	return cursor
}

func requireRecentSubmodelReference(t *testing.T, payload map[string]any, id string, field string, value string) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "recent changes result must be an array")
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if !ok || item["id"] != id {
			continue
		}
		if recentChangeReferenceFieldContainsValue(item[field], value) {
			return
		}
	}
	t.Fatalf("expected recent change id=%s field=%s containing reference value=%s in payload: %#v", id, field, value, payload)
}

func recentChangeReferenceFieldContainsValue(raw any, value string) bool {
	references, ok := raw.([]any)
	if !ok {
		references = []any{raw}
	}
	for _, rawReference := range references {
		reference, ok := rawReference.(map[string]any)
		if !ok {
			continue
		}
		keys, ok := reference["keys"].([]any)
		if !ok {
			continue
		}
		for _, rawKey := range keys {
			key, ok := rawKey.(map[string]any)
			if ok && key["value"] == value {
				return true
			}
		}
	}
	return false
}

func getElementByIDShort(t *testing.T, submodel map[string]any, idShort string) map[string]any {
	t.Helper()
	elements, ok := submodel["submodelElements"].([]any)
	require.True(t, ok, "submodelElements must be an array")
	for _, raw := range elements {
		element, ok := raw.(map[string]any)
		if ok && element["idShort"] == idShort {
			return element
		}
	}
	t.Fatalf("expected submodel element idShort=%s in payload: %#v", idShort, submodel)
	return nil
}
