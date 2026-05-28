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
			"keys": []any{map[string]any{"type": "GlobalReference", "value": "urn:example:semantic:history"}},
		},
		"submodelElements": []any{
			map[string]any{
				"modelType": "Property",
				"idShort":   "Temperature",
				"valueType": "xs:string",
				"value":     "v1",
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

	status, body, err = requestJSON(http.MethodGet, baseURL+"/submodels/$recent-changes?limit=10", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	recent := decodeMap(t, body)
	requireRecentChangeForIDAndTypeSubmodel(t, recent, submodelID, "Created")
	requireRecentChangeForIDAndTypeSubmodel(t, recent, submodelID, "Updated")

	status, body, err = requestJSON(http.MethodGet, baseURL+"/submodels/$recent-changes?semanticId=urn:example:semantic:history&limit=10", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	requireRecentChangeForIDAndTypeSubmodel(t, decodeMap(t, body), submodelID, "Updated")

	status, body, err = requestJSON(http.MethodDelete, baseURL+"/submodels/"+encodedSubmodelID, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	status, body, err = requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s", baseURL, encodedSubmodelID, time.Now().UTC().Format(time.RFC3339Nano)), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, status, "response=%s", string(body))

	status, body, err = requestJSON(http.MethodGet, baseURL+"/submodels/$recent-changes?limit=10", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	requireRecentChangeForIDAndTypeSubmodel(t, decodeMap(t, body), submodelID, "Deleted")
}

func decodeMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func requireRecentChangeForIDAndTypeSubmodel(t *testing.T, payload map[string]any, id string, changeType string) {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "recent changes result must be an array")
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if item["id"] == id && item["type"] == changeType {
			return
		}
	}
	t.Fatalf("expected recent change id=%s type=%s in payload: %#v", id, changeType, payload)
}
