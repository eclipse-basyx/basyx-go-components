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
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

type referenceKey struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type modelReference struct {
	Type string         `json:"type"`
	Keys []referenceKey `json:"keys"`
}

func TestSubmodelElementReferenceUsesModelTypeOfListItems(t *testing.T) {
	baseURL := submodelRepositoryBaseURL
	submodelID := fmt.Sprintf("urn:basyx:integration:sme-reference-list-items-%d", time.Now().UnixNano())
	submodelIDEncoded := common.EncodeString(submodelID)

	statusCode, body, err := requestJSON(http.MethodPost, baseURL+"/submodels", referenceListItemsSubmodel(submodelID))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))
	t.Cleanup(func() { deleteSubmodelForLargeObjectCleanupTest(t, baseURL, submodelIDEncoded) })

	submodelKey := referenceKey{Type: "Submodel", Value: submodelID}
	testCases := []struct {
		path         string
		expectedKeys []referenceKey
	}{
		{
			path: "readings",
			expectedKeys: []referenceKey{
				submodelKey,
				{Type: "SubmodelElementList", Value: "readings"},
			},
		},
		{
			path: "readings[0]",
			expectedKeys: []referenceKey{
				submodelKey,
				{Type: "SubmodelElementList", Value: "readings"},
				{Type: "Property", Value: "0"},
			},
		},
		{
			path: "documents[0]",
			expectedKeys: []referenceKey{
				submodelKey,
				{Type: "SubmodelElementList", Value: "documents"},
				{Type: "SubmodelElementCollection", Value: "0"},
			},
		},
		{
			path: "documents[0].title",
			expectedKeys: []referenceKey{
				submodelKey,
				{Type: "SubmodelElementList", Value: "documents"},
				{Type: "SubmodelElementCollection", Value: "0"},
				{Type: "Property", Value: "title"},
			},
		},
		{
			path: "matrix[0][0]",
			expectedKeys: []referenceKey{
				submodelKey,
				{Type: "SubmodelElementList", Value: "matrix"},
				{Type: "SubmodelElementList", Value: "0"},
				{Type: "Property", Value: "0"},
			},
		},
		{
			path: "machine.parts[0].serialNumber",
			expectedKeys: []referenceKey{
				submodelKey,
				{Type: "Entity", Value: "machine"},
				{Type: "SubmodelElementList", Value: "parts"},
				{Type: "SubmodelElementCollection", Value: "0"},
				{Type: "Property", Value: "serialNumber"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.path, func(t *testing.T) {
			endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/%s/$reference", baseURL, submodelIDEncoded, testCase.path)
			statusCode, body, err := requestJSON(http.MethodGet, endpoint, nil)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))

			var reference modelReference
			require.NoError(t, json.Unmarshal(body, &reference), "response=%s", string(body))
			require.Equal(t, "ModelReference", reference.Type)
			require.Equal(t, testCase.expectedKeys, reference.Keys)
		})
	}

	t.Run("missing list item", func(t *testing.T) {
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/readings[5]/$reference", baseURL, submodelIDEncoded)
		statusCode, body, err := requestJSON(http.MethodGet, endpoint, nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, statusCode, "response=%s", string(body))
	})
}

func referenceListItemsSubmodel(submodelID string) map[string]any {
	return map[string]any{
		"modelType": "Submodel",
		"id":        submodelID,
		"submodelElements": []any{
			map[string]any{
				"modelType":            "SubmodelElementList",
				"idShort":              "readings",
				"typeValueListElement": "Property",
				"valueTypeListElement": "xs:int",
				"value": []any{
					map[string]any{"modelType": "Property", "valueType": "xs:int", "value": "10"},
				},
			},
			map[string]any{
				"modelType":            "SubmodelElementList",
				"idShort":              "documents",
				"typeValueListElement": "SubmodelElementCollection",
				"value": []any{
					map[string]any{
						"modelType": "SubmodelElementCollection",
						"value": []any{
							map[string]any{"modelType": "Property", "idShort": "title", "valueType": "xs:string", "value": "Manual"},
						},
					},
				},
			},
			map[string]any{
				"modelType":            "SubmodelElementList",
				"idShort":              "matrix",
				"typeValueListElement": "SubmodelElementList",
				"value": []any{
					map[string]any{
						"modelType":            "SubmodelElementList",
						"typeValueListElement": "Property",
						"valueTypeListElement": "xs:int",
						"value": []any{
							map[string]any{"modelType": "Property", "valueType": "xs:int", "value": "1"},
						},
					},
				},
			},
			map[string]any{
				"modelType":     "Entity",
				"idShort":       "machine",
				"entityType":    "SelfManagedEntity",
				"globalAssetId": "urn:basyx:integration:machine",
				"statements": []any{
					map[string]any{
						"modelType":            "SubmodelElementList",
						"idShort":              "parts",
						"typeValueListElement": "SubmodelElementCollection",
						"value": []any{
							map[string]any{
								"modelType": "SubmodelElementCollection",
								"value": []any{
									map[string]any{"modelType": "Property", "idShort": "serialNumber", "valueType": "xs:string", "value": "SN-1"},
								},
							},
						},
					},
				},
			},
		},
	}
}
