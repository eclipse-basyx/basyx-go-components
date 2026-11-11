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

// TestPutRenamesTypedSubmodelElements checks that a PUT which renames an
// element also stores its type-specific attributes at the renamed path.
func TestPutRenamesTypedSubmodelElements(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:put-rename-%d", time.Now().UnixNano())
	elementsURL := submodelRepositoryBaseURL + "/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(submodelID)) + "/submodel-elements/"
	statusCode, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", map[string]any{
		"id": submodelID, "idShort": "PutRename", "modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{"idShort": "DocFile", "modelType": "File", "contentType": "application/pdf", "value": "/aasx/files/doc.pdf"},
			renameTestOperation("Op", "In1"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))
	t.Cleanup(func() {
		_, _, _ = requestJSON(http.MethodDelete, submodelRepositoryBaseURL+"/submodels/"+base64.RawURLEncoding.EncodeToString([]byte(submodelID)), nil)
	})

	t.Run("File", func(t *testing.T) {
		renamed := renameTestPut(t, elementsURL, "DocFile", map[string]any{"idShort": "RenamedFile", "modelType": "File", "contentType": "text/plain", "value": "/aasx/files/other.txt"})
		require.Equal(t, "/aasx/files/other.txt", renamed["value"])
		require.Equal(t, "text/plain", renamed["contentType"])
	})

	t.Run("Operation", func(t *testing.T) {
		renamed := renameTestPut(t, elementsURL, "Op", renameTestOperation("RenamedOp", "In2"))
		variable := renamed["inputVariables"].([]any)[0].(map[string]any)["value"].(map[string]any)
		require.Equal(t, "In2", variable["idShort"])
	})
}

func renameTestPut(t *testing.T, elementsURL string, oldIDShort string, element map[string]any) map[string]any {
	t.Helper()
	statusCode, body, err := requestJSON(http.MethodPut, elementsURL+oldIDShort, element)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, statusCode, "response=%s", string(body))
	statusCode, body, err = requestJSON(http.MethodGet, elementsURL+oldIDShort, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, statusCode, "response=%s", string(body))
	statusCode, body, err = requestJSON(http.MethodGet, elementsURL+element["idShort"].(string), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))
	renamed := map[string]any{}
	require.NoError(t, json.Unmarshal(body, &renamed))
	return renamed
}

func renameTestOperation(idShort string, inputIDShort string) map[string]any {
	return map[string]any{
		"idShort": idShort, "modelType": "Operation",
		"inputVariables": []any{map[string]any{"value": map[string]any{"idShort": inputIDShort, "modelType": "Property", "valueType": "xs:string"}}},
	}
}
