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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestListItemLocationAddressesTheCreatedElement(t *testing.T) {
	for _, service := range []idShortService{
		{name: "submodel-repository"},
		{name: "aas-repository", nested: true},
		{name: "environment", environment: true},
		{name: "environment-superpath", environment: true, nested: true},
	} {
		t.Run(service.name, func(t *testing.T) {
			h := newIDShortHarness(t, "strict", service)
			id := "urn:list-location:" + uuid.NewString()
			h.create(t, listLocationSubmodel(id), http.StatusCreated)
			root := h.path(id) + "/submodel-elements/"
			for _, test := range []struct {
				parent string
				index  int
				value  map[string]any
			}{
				{"Empty", 0, listLocationProperty("2")},
				{"Populated", 1, listLocationProperty("2")},
				{"Group.Rows", 0, listLocationProperty("2")},
				{"Nested[0]", 1, listLocationProperty("2")},
				{"Collections", 0, map[string]any{"modelType": "SubmodelElementCollection", "value": []any{map[string]any{"modelType": "Property", "idShort": "Child", "valueType": "xs:int", "value": "2"}}}},
			} {
				t.Run(test.parent, func(t *testing.T) {
					payload, err := json.Marshal(test.value)
					require.NoError(t, err)
					response := h.request(t, http.MethodPost, root+url.PathEscape(test.parent), payload, "application/json")
					assertListItemLocation(t, h, response, fmt.Sprintf("%s%s[%d]", root, test.parent, test.index), test.value)
				})
			}
			t.Run("concurrent appends", func(t *testing.T) {
				assertConcurrentListLocations(t, h, root+"Concurrent")
			})
		})
	}
}

func listLocationProperty(value string) map[string]any {
	return map[string]any{"modelType": "Property", "valueType": "xs:int", "value": value}
}

func listLocationList(name string, values ...any) map[string]any {
	result := map[string]any{"modelType": "SubmodelElementList", "typeValueListElement": "Property", "valueTypeListElement": "xs:int"}
	if name != "" {
		result["idShort"] = name
	}
	if values != nil {
		result["value"] = values
	}
	return result
}

func listLocationSubmodel(id string) map[string]any {
	return map[string]any{
		"modelType": "Submodel", "id": id,
		"submodelElements": []any{
			listLocationList("Empty"),
			listLocationList("Populated", listLocationProperty("1")),
			map[string]any{"modelType": "SubmodelElementCollection", "idShort": "Group", "value": []any{listLocationList("Rows")}},
			map[string]any{"modelType": "SubmodelElementList", "idShort": "Nested", "typeValueListElement": "SubmodelElementList", "value": []any{listLocationList("", listLocationProperty("1"))}},
			map[string]any{"modelType": "SubmodelElementList", "idShort": "Collections", "typeValueListElement": "SubmodelElementCollection"},
			listLocationList("Concurrent"),
		},
	}
}

func assertListItemLocation(t *testing.T, h *idShortHarness, response *httptest.ResponseRecorder, expectedPath string, value map[string]any) {
	t.Helper()
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	location, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	require.True(t, location.IsAbs(), response.Header().Get("Location"))
	require.Equal(t, expectedPath, location.Path)
	payload, err := json.Marshal(value)
	require.NoError(t, err)
	require.JSONEq(t, string(payload), response.Body.String())
	stored := h.json(t, http.MethodGet, location.RequestURI(), nil, http.StatusOK)
	actual, err := json.Marshal(stored)
	require.NoError(t, err)
	require.JSONEq(t, string(payload), string(actual))
}

func assertConcurrentListLocations(t *testing.T, h *idShortHarness, parentPath string) {
	t.Helper()
	const count = 4
	type result struct {
		response *httptest.ResponseRecorder
		value    map[string]any
	}
	results := make(chan result, count)
	var workers sync.WaitGroup
	for index := range count {
		value := listLocationProperty(fmt.Sprint(index))
		payload, err := json.Marshal(value)
		require.NoError(t, err)
		workers.Go(func() {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, parentPath, strings.NewReader(string(payload)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			h.handler.ServeHTTP(response, request)
			results <- result{response: response, value: value}
		})
	}
	workers.Wait()
	close(results)
	paths := make([]string, 0, count)
	for result := range results {
		location, err := url.Parse(result.response.Header().Get("Location"))
		require.NoError(t, err)
		assertListItemLocation(t, h, result.response, location.Path, result.value)
		paths = append(paths, location.Path)
	}
	expected := make([]string, count)
	for index := range count {
		expected[index] = fmt.Sprintf("%s[%d]", parentPath, index)
	}
	require.ElementsMatch(t, expected, paths)
}
