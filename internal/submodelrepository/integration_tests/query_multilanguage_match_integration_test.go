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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryMatchCorrelatesMultiLanguagePropertyEntries(t *testing.T) {
	submodel := createMultiLanguageMatchSubmodel(t)
	productName := elementByIDShort(t, arrayValue(t, submodel["submodelElements"]), "ProductName")

	for _, test := range []struct {
		name          string
		operator      string
		text          string
		language      string
		expectedCount int
	}{
		{name: "MatchFrenchTextWithFrench", operator: "$match", text: "Capteur", language: "fr", expectedCount: 1},
		{name: "MatchFrenchTextWithEnglish", operator: "$match", text: "Capteur", language: "en", expectedCount: 0},
		{name: "MatchEnglishTextWithEnglish", operator: "$match", text: "Industrial Sensor", language: "en", expectedCount: 1},
		{name: "MatchEnglishTextWithFrench", operator: "$match", text: "Industrial Sensor", language: "fr", expectedCount: 0},
		{name: "AndFrenchTextWithEnglish", operator: "$and", text: "Capteur", language: "en", expectedCount: 1},
		{name: "AndEnglishTextWithFrench", operator: "$and", text: "Industrial Sensor", language: "fr", expectedCount: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := map[string]any{
				"$condition": map[string]any{
					"$and": []any{
						equalsCondition("$sm#id", submodel["id"].(string)),
						map[string]any{
							test.operator: []any{
								map[string]any{
									"$contains": []any{
										map[string]any{"$field": "$sme.ProductName#value"},
										map[string]any{"$strVal": test.text},
									},
								},
								equalsCondition("$sme.ProductName#language", test.language),
							},
						},
					},
				},
			}
			statusCode, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/query/submodels", payload)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))

			var response struct {
				Result []map[string]any `json:"result"`
			}
			require.NoError(t, json.Unmarshal(body, &response), "response=%s", string(body))
			require.Len(t, response.Result, test.expectedCount, "response=%s", string(body))
			if test.expectedCount == 0 {
				return
			}
			assert.Equal(t, submodel["id"], response.Result[0]["id"])
			elements := arrayValue(t, response.Result[0]["submodelElements"])
			assert.Equal(t, productName, elementByIDShort(t, elements, "ProductName"))
		})
	}
}

func createMultiLanguageMatchSubmodel(t *testing.T) map[string]any {
	t.Helper()

	fixture, err := os.ReadFile("bodies/post/query/submodelG.json")
	require.NoError(t, err)
	var submodel map[string]any
	require.NoError(t, json.Unmarshal(fixture, &submodel))
	submodelID := fmt.Sprintf("urn:basyx:integration:query-multilanguage-match:%d", time.Now().UnixNano())
	submodel["id"] = submodelID

	statusCode, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", submodel)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))
	t.Cleanup(func() {
		encodedID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
		deleteStatus, deleteBody, deleteErr := requestJSON(http.MethodDelete, submodelRepositoryBaseURL+"/submodels/"+encodedID, nil)
		assert.NoError(t, deleteErr)
		assert.Equal(t, http.StatusNoContent, deleteStatus, "response=%s", string(deleteBody))
	})
	return submodel
}
