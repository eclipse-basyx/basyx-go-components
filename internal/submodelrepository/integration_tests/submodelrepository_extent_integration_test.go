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

func TestSubmodelRepositoryExtentShapesBlobValues(t *testing.T) {
	baseURL := submodelRepositoryBaseURL
	submodelID := fmt.Sprintf("urn:basyx:integration:extent-%d", time.Now().UnixNano())
	submodelIDEncoded := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	topBlobValue := base64.StdEncoding.EncodeToString([]byte("top-blob-value"))
	nestedBlobValue := base64.StdEncoding.EncodeToString([]byte("nested-blob-value"))

	payload := extentTestSubmodelPayload(submodelID, topBlobValue, nestedBlobValue)
	statusCode, body, err := requestJSON(http.MethodPost, baseURL+"/submodels", payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))

	t.Cleanup(func() {
		status, cleanupBody, cleanupErr := requestJSON(http.MethodDelete, baseURL+"/submodels/"+submodelIDEncoded, nil)
		if cleanupErr != nil {
			t.Logf("cleanup delete failed: %v", cleanupErr)
			return
		}
		if status != http.StatusNoContent && status != http.StatusNotFound {
			t.Logf("cleanup delete returned status=%d response=%s", status, string(cleanupBody))
		}
	})

	t.Run("default and withoutBlobValue omit blob values", func(t *testing.T) {
		for _, query := range []string{"", "?extent=withoutBlobValue"} {
			status, responseBody, requestErr := requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+query, nil)
			require.NoError(t, requestErr)
			require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
			requireSubmodelBlobValueState(t, decodeMap(t, responseBody), "TopBlob", "text/plain", "", false)
			requireSubmodelBlobValueState(t, decodeMap(t, responseBody), "NestedBlob", "application/octet-stream", "", false)
		}
	})

	t.Run("withBlobValue includes blob values", func(t *testing.T) {
		status, responseBody, err := requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"?extent=withBlobValue", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
		payload := decodeMap(t, responseBody)
		requireSubmodelBlobValueState(t, payload, "TopBlob", "text/plain", topBlobValue, true)
		requireSubmodelBlobValueState(t, payload, "NestedBlob", "application/octet-stream", nestedBlobValue, true)
	})

	t.Run("value-only submodel honors extent", func(t *testing.T) {
		status, responseBody, err := requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/$value?extent=withoutBlobValue", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
		requireValueOnlyBlobValueState(t, decodeMap(t, responseBody)["TopBlob"], "text/plain", "", false)
		requireValueOnlyBlobValueState(t, valueOnlyNestedValue(t, decodeMap(t, responseBody), "MainCollection", "NestedBlob"), "application/octet-stream", "", false)

		status, responseBody, err = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/$value?extent=withBlobValue", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
		requireValueOnlyBlobValueState(t, decodeMap(t, responseBody)["TopBlob"], "text/plain", topBlobValue, true)
		requireValueOnlyBlobValueState(t, valueOnlyNestedValue(t, decodeMap(t, responseBody), "MainCollection", "NestedBlob"), "application/octet-stream", nestedBlobValue, true)
	})

	t.Run("submodel element reads honor extent", func(t *testing.T) {
		status, responseBody, err := requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements?level=deep&extent=withBlobValue", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
		result := decodeMap(t, responseBody)["result"].([]any)
		requireElementBlobValueState(t, findSubmodelElementInList(t, result, "TopBlob"), "text/plain", topBlobValue, true)
		requireElementBlobValueState(t, findSubmodelElementInList(t, result, "NestedBlob"), "application/octet-stream", nestedBlobValue, true)

		status, responseBody, err = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/TopBlob", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
		requireElementBlobValueState(t, decodeMap(t, responseBody), "text/plain", "", false)

		status, responseBody, err = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/TopBlob?extent=withBlobValue", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
		requireElementBlobValueState(t, decodeMap(t, responseBody), "text/plain", topBlobValue, true)
	})

	t.Run("value-only submodel element reads honor extent", func(t *testing.T) {
		status, responseBody, err := requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/$value?level=deep&extent=withBlobValue", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
		requireValueOnlyBlobValueState(t, wrappedValueOnlyResult(t, decodeMap(t, responseBody), "TopBlob"), "text/plain", topBlobValue, true)

		status, responseBody, err = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/TopBlob/$value", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
		requireValueOnlyBlobValueState(t, decodeMap(t, responseBody), "text/plain", "", false)

		status, responseBody, err = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/TopBlob/$value?extent=withBlobValue", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
		requireValueOnlyBlobValueState(t, decodeMap(t, responseBody), "text/plain", topBlobValue, true)
	})

	t.Run("invalid extent and level return bad request", func(t *testing.T) {
		status, responseBody, err := requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"?extent=invalid", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, status, "response=%s", string(responseBody))

		status, responseBody, err = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/TopBlob?extent=invalid", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, status, "response=%s", string(responseBody))

		status, responseBody, err = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/$value?level=invalid", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, status, "response=%s", string(responseBody))

		status, responseBody, err = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/TopBlob/$value?level=invalid", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, status, "response=%s", string(responseBody))
	})
}

func TestSubmodelRepositoryHistoryExtentAndCoreLevel(t *testing.T) {
	baseURL := submodelRepositoryBaseURL
	submodelID := fmt.Sprintf("urn:basyx:integration:history-extent-%d", time.Now().UnixNano())
	submodelIDEncoded := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	topBlobValue := base64.StdEncoding.EncodeToString([]byte("history-top-blob-value"))
	nestedBlobValue := base64.StdEncoding.EncodeToString([]byte("history-nested-blob-value"))

	statusCode, body, err := requestJSON(http.MethodPost, baseURL+"/submodels", extentTestSubmodelPayload(submodelID, topBlobValue, nestedBlobValue))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))

	t.Cleanup(func() {
		_, _, _ = requestJSON(http.MethodDelete, baseURL+"/submodels/"+submodelIDEncoded, nil)
	})

	time.Sleep(30 * time.Millisecond)
	historyDate := url.QueryEscape(time.Now().UTC().Format(time.RFC3339Nano))

	status, responseBody, err := requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s&level=deep&extent=withBlobValue", baseURL, submodelIDEncoded, historyDate), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
	requireSubmodelBlobValueState(t, decodeMap(t, responseBody), "TopBlob", "text/plain", topBlobValue, true)
	requireSubmodelBlobValueState(t, decodeMap(t, responseBody), "NestedBlob", "application/octet-stream", nestedBlobValue, true)

	status, responseBody, err = requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s&level=deep&extent=withoutBlobValue", baseURL, submodelIDEncoded, historyDate), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
	requireSubmodelBlobValueState(t, decodeMap(t, responseBody), "TopBlob", "text/plain", "", false)
	requireSubmodelBlobValueState(t, decodeMap(t, responseBody), "NestedBlob", "application/octet-stream", "", false)

	status, responseBody, err = requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$history?date=%s&level=core&extent=withBlobValue", baseURL, submodelIDEncoded, historyDate), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))
	coreSnapshot := decodeMap(t, responseBody)
	requireSubmodelBlobValueState(t, coreSnapshot, "TopBlob", "text/plain", topBlobValue, true)
	mainCollection := findSubmodelElementInList(t, coreSnapshot["submodelElements"].([]any), "MainCollection")
	_, hasNestedValue := mainCollection["value"]
	require.False(t, hasNestedValue, "core historical collection must not include nested value payload")
}

func extentTestSubmodelPayload(submodelID string, topBlobValue string, nestedBlobValue string) map[string]any {
	return map[string]any{
		"id":        submodelID,
		"idShort":   "ExtentSubmodel",
		"kind":      "Instance",
		"modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{
				"idShort":     "TopBlob",
				"modelType":   "Blob",
				"contentType": "text/plain",
				"value":       topBlobValue,
			},
			map[string]any{
				"idShort":   "MainCollection",
				"modelType": "SubmodelElementCollection",
				"value": []any{
					map[string]any{
						"idShort":     "NestedBlob",
						"modelType":   "Blob",
						"contentType": "application/octet-stream",
						"value":       nestedBlobValue,
					},
					map[string]any{
						"idShort":   "NestedProperty",
						"modelType": "Property",
						"valueType": "xs:string",
						"value":     "nested",
					},
				},
			},
		},
	}
}

func requireSubmodelBlobValueState(t *testing.T, submodel map[string]any, idShort string, contentType string, expectedValue string, expectValue bool) {
	t.Helper()
	rawElements, ok := submodel["submodelElements"].([]any)
	require.True(t, ok, "submodelElements must be an array")
	requireElementBlobValueState(t, findSubmodelElementInList(t, rawElements, idShort), contentType, expectedValue, expectValue)
}

func requireElementBlobValueState(t *testing.T, element map[string]any, contentType string, expectedValue string, expectValue bool) {
	t.Helper()
	require.Equal(t, "Blob", element["modelType"])
	require.Equal(t, contentType, element["contentType"])
	actualValue, hasValue := element["value"]
	require.Equal(t, expectValue, hasValue, "blob value presence mismatch in element: %#v", element)
	if expectValue {
		require.Equal(t, expectedValue, actualValue)
	}
}

func requireValueOnlyBlobValueState(t *testing.T, raw any, contentType string, expectedValue string, expectValue bool) {
	t.Helper()
	blobValue, ok := raw.(map[string]any)
	require.True(t, ok, "value-only blob must be an object: %#v", raw)
	require.Equal(t, contentType, blobValue["contentType"])
	actualValue, hasValue := blobValue["value"]
	require.Equal(t, expectValue, hasValue, "blob value presence mismatch in value-only payload: %#v", blobValue)
	if expectValue {
		require.Equal(t, expectedValue, actualValue)
	}
}

func findSubmodelElementInList(t *testing.T, rawElements []any, idShort string) map[string]any {
	t.Helper()
	for _, rawElement := range rawElements {
		element, ok := rawElement.(map[string]any)
		require.True(t, ok, "submodel element must be an object: %#v", rawElement)
		if element["idShort"] == idShort {
			return element
		}
		if rawValue, ok := element["value"].([]any); ok {
			if nested := findSubmodelElementInListOptional(t, rawValue, idShort); nested != nil {
				return nested
			}
		}
	}
	t.Fatalf("expected submodel element idShort=%s in payload: %#v", idShort, rawElements)
	return nil
}

func findSubmodelElementInListOptional(t *testing.T, rawElements []any, idShort string) map[string]any {
	t.Helper()
	for _, rawElement := range rawElements {
		element, ok := rawElement.(map[string]any)
		require.True(t, ok, "submodel element must be an object: %#v", rawElement)
		if element["idShort"] == idShort {
			return element
		}
		if rawValue, ok := element["value"].([]any); ok {
			if nested := findSubmodelElementInListOptional(t, rawValue, idShort); nested != nil {
				return nested
			}
		}
	}
	return nil
}

func valueOnlyNestedValue(t *testing.T, submodelValue map[string]any, parent string, child string) any {
	t.Helper()
	parentValue, ok := submodelValue[parent].(map[string]any)
	require.True(t, ok, "value-only parent must be an object: %#v", submodelValue[parent])
	return parentValue[child]
}

func wrappedValueOnlyResult(t *testing.T, payload map[string]any, idShort string) any {
	t.Helper()
	result, ok := payload["result"].([]any)
	require.True(t, ok, "value-only result must be an array")
	for _, rawEntry := range result {
		entry, ok := rawEntry.(map[string]any)
		require.True(t, ok, "value-only result entry must be an object: %#v", rawEntry)
		if value, exists := entry[idShort]; exists {
			return value
		}
	}
	t.Fatalf("expected value-only result for idShort=%s in payload: %#v", idShort, payload)
	return nil
}
