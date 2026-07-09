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

// TestPutFileSubmodelElementWithoutValue reproduces the panic and NULL-scan
// failures that occurred when a File submodel element was PUT without the
// optional value field (see PostgreSQLFileHandler.Update).
func TestPutFileSubmodelElementWithoutValue(t *testing.T) {
	baseURL := submodelRepositoryBaseURL
	submodelID := fmt.Sprintf("urn:basyx:integration:file-put-no-value-%d", time.Now().UnixNano())
	submodelIDEncoded := base64.RawURLEncoding.EncodeToString([]byte(submodelID))

	payload := map[string]any{
		"id":        submodelID,
		"idShort":   "FilePutSubmodel",
		"kind":      "Instance",
		"modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{
				"idShort":     "DocFile",
				"modelType":   "File",
				"contentType": "application/pdf",
				"value":       "/aasx/files/doc.pdf",
			},
			map[string]any{
				"idShort":     "EmptyFile",
				"modelType":   "File",
				"contentType": "application/pdf",
			},
		},
	}

	statusCode, body, err := requestJSON(http.MethodPost, baseURL+"/submodels", payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))

	t.Cleanup(func() {
		_, _, _ = requestJSON(http.MethodDelete, baseURL+"/submodels/"+submodelIDEncoded, nil)
	})

	t.Run("PutWithoutValueOnFileWithStoredValue", func(t *testing.T) {
		status, responseBody, requestErr := requestJSON(http.MethodPut, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/DocFile", map[string]any{
			"idShort":     "DocFile",
			"modelType":   "File",
			"contentType": "application/pdf",
		})
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusNoContent, status, "response=%s", string(responseBody))

		status, responseBody, requestErr = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/DocFile", nil)
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))

		var element map[string]any
		require.NoError(t, json.Unmarshal(responseBody, &element), "response=%s", string(responseBody))
		require.Equal(t, "File", element["modelType"])
		if rawValue, hasValue := element["value"]; hasValue && rawValue != nil {
			require.Equal(t, "", rawValue, "file value must be cleared after PUT without value")
		}
	})

	t.Run("PutWithoutValueOnFileWithoutStoredValue", func(t *testing.T) {
		status, responseBody, requestErr := requestJSON(http.MethodPut, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/EmptyFile", map[string]any{
			"idShort":     "EmptyFile",
			"modelType":   "File",
			"contentType": "text/plain",
		})
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusNoContent, status, "response=%s", string(responseBody))
	})

	t.Run("PutWithValueStillPersistsValue", func(t *testing.T) {
		status, responseBody, requestErr := requestJSON(http.MethodPut, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/EmptyFile", map[string]any{
			"idShort":     "EmptyFile",
			"modelType":   "File",
			"contentType": "text/plain",
			"value":       "/aasx/files/readme.txt",
		})
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusNoContent, status, "response=%s", string(responseBody))

		status, responseBody, requestErr = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/EmptyFile", nil)
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))

		var element map[string]any
		require.NoError(t, json.Unmarshal(responseBody, &element), "response=%s", string(responseBody))
		require.Equal(t, "/aasx/files/readme.txt", element["value"], "response=%s", string(responseBody))
	})
}
