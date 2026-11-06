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
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptyElementArraysAreOmitted(t *testing.T) {
	id := "urn:basyx:test:empty-elements:" + uuid.NewString()
	idShort := "EmptyElements" + fmt.Sprint(time.Now().UnixNano())
	path := submodelRepositoryBaseURL + "/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(id))
	created := emptyElementsRequest(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", map[string]any{
		"modelType": "Submodel", "id": id, "idShort": idShort,
	}, http.StatusCreated)
	t.Cleanup(func() { _, _, _ = requestJSON(http.MethodDelete, path, nil) })
	t.Run("create submodel", func(t *testing.T) { assert.NotContains(t, created, "submodelElements") })

	for _, test := range []struct{ name, path string }{
		{"get submodel", path},
		{"history", path + "/$history?date=" + url.QueryEscape(time.Now().UTC().Format(time.RFC3339Nano))},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.NotContains(t, emptyElementsRequest(t, http.MethodGet, test.path, nil, http.StatusOK), "submodelElements")
		})
	}
	t.Run("list submodels", func(t *testing.T) {
		result := emptyElementsResult(t, http.MethodGet, submodelRepositoryBaseURL+"/submodels?idShort="+url.QueryEscape(idShort), nil)
		assert.NotContains(t, result[0], "submodelElements")
	})
	t.Run("query submodels", func(t *testing.T) {
		result := emptyElementsResult(t, http.MethodPost, submodelRepositoryBaseURL+"/query/submodels", map[string]any{
			"$condition": equalsCondition("$sm#id", id),
		})
		assert.NotContains(t, result[0], "submodelElements")
	})
}

func TestEmptyContainerValuesAreOmitted(t *testing.T) {
	id := "urn:basyx:test:empty-containers:" + uuid.NewString()
	idShort := "Containers" + fmt.Sprint(time.Now().UnixNano())
	path := submodelRepositoryBaseURL + "/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(id))
	property := map[string]any{"modelType": "Property", "idShort": "Child", "valueType": "xs:string", "value": "kept"}
	elements := []any{
		map[string]any{"modelType": "SubmodelElementCollection", "idShort": "EmptyCollection"},
		map[string]any{"modelType": "SubmodelElementList", "idShort": "EmptyList", "typeValueListElement": "Property", "valueTypeListElement": "xs:string"},
		map[string]any{"modelType": "SubmodelElementCollection", "idShort": "FullCollection", "value": []any{property}},
		map[string]any{"modelType": "SubmodelElementList", "idShort": "FullList", "typeValueListElement": "Property", "valueTypeListElement": "xs:string", "value": []any{map[string]any{"modelType": "Property", "valueType": "xs:string", "value": "kept"}}},
	}
	created := emptyElementsRequest(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", map[string]any{
		"modelType": "Submodel", "id": id, "idShort": idShort, "submodelElements": elements,
	}, http.StatusCreated)
	t.Cleanup(func() { _, _, _ = requestJSON(http.MethodDelete, path, nil) })
	check := func(t *testing.T, submodel map[string]any) {
		t.Helper()
		raw, ok := submodel["submodelElements"].([]any)
		require.True(t, ok)
		require.Len(t, raw, 4)
		for _, name := range []string{"EmptyCollection", "EmptyList"} {
			assert.NotContains(t, elementByIDShort(t, raw, name), "value", name)
		}
		for _, name := range []string{"FullCollection", "FullList"} {
			children, ok := elementByIDShort(t, raw, name)["value"].([]any)
			require.True(t, ok, "%s value must be an array", name)
			require.Len(t, children, 1, name)
			assert.Equal(t, "kept", objectValue(t, children[0])["value"], name)
		}
	}
	t.Run("create submodel", func(t *testing.T) { check(t, created) })
	t.Run("get submodel", func(t *testing.T) { check(t, emptyElementsRequest(t, http.MethodGet, path, nil, http.StatusOK)) })
	t.Run("history", func(t *testing.T) {
		check(t, emptyElementsRequest(t, http.MethodGet, path+"/$history?date="+url.QueryEscape(time.Now().UTC().Format(time.RFC3339Nano)), nil, http.StatusOK))
	})
	t.Run("list submodels", func(t *testing.T) {
		check(t, emptyElementsResult(t, http.MethodGet, submodelRepositoryBaseURL+"/submodels?idShort="+url.QueryEscape(idShort), nil)[0])
	})
	t.Run("query submodels", func(t *testing.T) {
		check(t, emptyElementsResult(t, http.MethodPost, submodelRepositoryBaseURL+"/query/submodels", map[string]any{"$condition": equalsCondition("$sm#id", id)})[0])
	})
	t.Run("list elements", func(t *testing.T) {
		result := emptyElementsRequest(t, http.MethodGet, path+"/submodel-elements", nil, http.StatusOK)
		raw, ok := result["result"].([]any)
		require.True(t, ok)
		require.Len(t, raw, 4)
		for _, name := range []string{"EmptyCollection", "EmptyList"} {
			assert.NotContains(t, elementByIDShort(t, raw, name), "value", name)
		}
	})
	for _, name := range []string{"EmptyCollection", "EmptyList"} {
		t.Run("get "+name, func(t *testing.T) {
			assert.NotContains(t, emptyElementsRequest(t, http.MethodGet, path+"/submodel-elements/"+name, nil, http.StatusOK), "value")
		})
	}
	for _, test := range []struct{ name, fragment string }{
		{"FullCollection", "$sme.FullCollection.Child"},
		{"FullList", "$sme.FullList[]"},
	} {
		t.Run("filter empties "+test.name, func(t *testing.T) {
			query := map[string]any{"$condition": equalsCondition("$sm#id", id), "$filters": []any{
				map[string]any{"$fragment": test.fragment, "$condition": map[string]any{"$boolean": false}},
			}}
			submodel := emptyElementsResult(t, http.MethodPost, submodelRepositoryBaseURL+"/query/submodels", query)[0]
			elements := arrayValue(t, submodel["submodelElements"])
			assert.NotContains(t, elementByIDShort(t, elements, test.name), "value")
		})
	}
	for _, element := range []map[string]any{
		{"modelType": "SubmodelElementCollection", "idShort": "CreatedCollection"},
		{"modelType": "SubmodelElementList", "idShort": "CreatedList", "typeValueListElement": "Property", "valueTypeListElement": "xs:string"},
	} {
		t.Run("create "+element["idShort"].(string), func(t *testing.T) {
			createdElement := emptyElementsRequest(t, http.MethodPost, path+"/submodel-elements", element, http.StatusCreated)
			assert.NotContains(t, createdElement, "value")
		})
	}
	t.Run("filter removes all elements", func(t *testing.T) {
		query := map[string]any{"$condition": equalsCondition("$sm#id", id), "$filters": []any{
			map[string]any{"$fragment": "$sme", "$condition": map[string]any{"$boolean": false}},
		}}
		result := emptyElementsResult(t, http.MethodPost, submodelRepositoryBaseURL+"/query/submodels", query)
		assert.NotContains(t, result[0], "submodelElements")
	})
}

func emptyElementsRequest(t *testing.T, method, endpoint string, payload any, expected int) map[string]any {
	t.Helper()
	status, body, err := requestJSON(method, endpoint, payload)
	require.NoError(t, err)
	require.Equal(t, expected, status, "response=%s", body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result), "response=%s", body)
	return result
}

func emptyElementsResult(t *testing.T, method, endpoint string, payload any) []map[string]any {
	t.Helper()
	response := emptyElementsRequest(t, method, endpoint, payload, http.StatusOK)
	raw, ok := response["result"].([]any)
	require.True(t, ok)
	require.Len(t, raw, 1)
	result, ok := raw[0].(map[string]any)
	require.True(t, ok)
	return []map[string]any{result}
}
