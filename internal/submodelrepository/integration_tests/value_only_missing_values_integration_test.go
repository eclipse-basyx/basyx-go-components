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
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const missingValuesElementsJSON = `[
	{"modelType": "Property", "idShort": "prop", "valueType": "xs:int"},
	{"modelType": "MultiLanguageProperty", "idShort": "mlp"},
	{"modelType": "Range", "idShort": "rngmin", "valueType": "xs:int", "min": "1"},
	{"modelType": "Range", "idShort": "rngnone", "valueType": "xs:int"},
	{"modelType": "File", "idShort": "file", "contentType": "text/plain"},
	{"modelType": "File", "idShort": "fileNone"},
	{"modelType": "Blob", "idShort": "blobNone"},
	{"modelType": "ReferenceElement", "idShort": "ref"},
	{"modelType": "SubmodelElementCollection", "idShort": "emptyColl"},
	{"modelType": "SubmodelElementCollection", "idShort": "coll",
		"value": [{"modelType": "Property", "idShort": "inner", "valueType": "xs:int"}]},
	{"modelType": "SubmodelElementList", "idShort": "members", "typeValueListElement": "SubmodelElement", "value": [
		{"modelType": "Property", "valueType": "xs:int", "value": "10"},
		{"modelType": "Property", "valueType": "xs:int"},
		{"modelType": "MultiLanguageProperty"},
		{"modelType": "Range", "valueType": "xs:int", "min": "1"},
		{"modelType": "Range", "valueType": "xs:int"},
		{"modelType": "File", "contentType": "text/plain"},
		{"modelType": "ReferenceElement"},
		{"modelType": "SubmodelElementCollection"}
	]},
	{"modelType": "SubmodelElementList", "idShort": "withOperation", "typeValueListElement": "SubmodelElement", "value": [
		{"modelType": "Property", "valueType": "xs:int", "value": "10"},
		{"modelType": "Operation"}
	]}
]`

const missingValuesExpectedValueOnly = `{
	"rngmin": {"min": 1},
	"file": {"contentType": "text/plain"},
	"coll": {},
	"members": [10, null, [], {"min": 1}, {}, {"contentType": "text/plain"}, null, {}]
}`

func TestValueOnlyOmitsMissingValues(t *testing.T) {
	for _, service := range []idShortService{
		{name: "submodel-repository"},
		{name: "aas-repository", nested: true},
	} {
		t.Run(service.name, func(t *testing.T) {
			h := newIDShortHarness(t, "strict", service)
			id := "urn:valueonly:novalue:" + uuid.NewString()
			h.create(t, missingValuesSubmodel(t, id), http.StatusCreated)
			path := h.path(id)

			require.JSONEq(t, missingValuesExpectedValueOnly, h.valueOnly(t, path+"/$value", http.StatusOK))
			h.assertElementsValueOnly(t, path+"/submodel-elements/$value", missingValuesExpectedValueOnly)
			h.assertElementsByPathValueOnly(t, path)
			if !service.nested {
				h.assertSubmodelsValueOnly(t, id, "", missingValuesExpectedValueOnly)
			}
			h.assertValueOnlyRoundTrip(t, path)
		})
	}
}

func TestValueOnlyCoreLevelSerializesNestedContainers(t *testing.T) {
	for _, service := range []idShortService{
		{name: "submodel-repository"},
		{name: "aas-repository", nested: true},
	} {
		t.Run(service.name, func(t *testing.T) {
			h := newIDShortHarness(t, "strict", service)
			id := "urn:valueonly:core:" + uuid.NewString()
			h.create(t, valueOnlyTestSubmodel(t, id, coreLevelElementsJSON), http.StatusCreated)
			path := h.path(id)
			rootCore := `{"RootCollection": {}}`
			rootChildrenCore := `{"collection": {}, "list": [], "prop": 3}`

			require.JSONEq(t, rootCore, h.valueOnly(t, path+"/$value?level=core", http.StatusOK))
			h.assertCoreSubmodelOmitsGrandchildren(t, h.json(t, http.MethodGet, path+"?level=core", nil, http.StatusOK))
			h.assertElementsValueOnly(t, path+"/submodel-elements/$value?level=core", `{"RootCollection": `+rootChildrenCore+`}`)
			require.JSONEq(t, rootChildrenCore, h.valueOnly(t, path+"/submodel-elements/RootCollection/$value?level=core", http.StatusOK))
			require.JSONEq(t, `{"collection": {"inner": 1}, "list": [2], "prop": 3}`,
				h.valueOnly(t, path+"/submodel-elements/RootCollection/$value", http.StatusOK))
			if !service.nested {
				h.assertSubmodelsValueOnly(t, id, "&level=core", rootCore)
				page := h.json(t, http.MethodGet, "/submodels?cursor="+common.EncodeString(id)+"&limit=1&level=core", nil, http.StatusOK)
				h.assertCoreSubmodelOmitsGrandchildren(t, page["result"].([]any)[0].(map[string]any))
			}
		})
	}
}

func (h *idShortHarness) assertCoreSubmodelOmitsGrandchildren(t *testing.T, submodel map[string]any) {
	t.Helper()
	elements := submodel["submodelElements"].([]any)
	require.Len(t, elements, 1)
	root := elements[0].(map[string]any)
	require.Equal(t, "RootCollection", root["idShort"])
	require.NotContains(t, root, "value")
}

const coreLevelElementsJSON = `[
	{"modelType": "SubmodelElementCollection", "idShort": "RootCollection", "value": [
		{"modelType": "SubmodelElementCollection", "idShort": "collection",
			"value": [{"modelType": "Property", "idShort": "inner", "valueType": "xs:int", "value": "1"}]},
		{"modelType": "SubmodelElementList", "idShort": "list", "typeValueListElement": "Property", "valueTypeListElement": "xs:int",
			"value": [{"modelType": "Property", "valueType": "xs:int", "value": "2"}]},
		{"modelType": "Property", "idShort": "prop", "valueType": "xs:int", "value": "3"},
		{"modelType": "Property", "idShort": "unset", "valueType": "xs:int"}
	]}
]`

func missingValuesSubmodel(t *testing.T, id string) map[string]any {
	t.Helper()
	return valueOnlyTestSubmodel(t, id, missingValuesElementsJSON)
}

func valueOnlyTestSubmodel(t *testing.T, id string, elementsJSON string) map[string]any {
	t.Helper()
	var elements []any
	require.NoError(t, json.Unmarshal([]byte(elementsJSON), &elements))
	return map[string]any{"modelType": "Submodel", "id": id, "submodelElements": elements}
}

func (h *idShortHarness) valueOnly(t *testing.T, path string, status int) string {
	t.Helper()
	response := h.request(t, http.MethodGet, path, nil, "application/json")
	require.Equal(t, status, response.Code, "GET %s: %s", path, response.Body.String())
	return response.Body.String()
}

func (h *idShortHarness) assertElementsValueOnly(t *testing.T, endpoint string, expected string) {
	t.Helper()
	page := h.json(t, http.MethodGet, endpoint, nil, http.StatusOK)
	merged := map[string]any{}
	for _, item := range page["result"].([]any) {
		member := item.(map[string]any)
		require.Len(t, member, 1)
		for idShort, value := range member {
			merged[idShort] = value
		}
	}
	actual, err := json.Marshal(merged)
	require.NoError(t, err)
	require.JSONEq(t, expected, string(actual))
}

func (h *idShortHarness) assertElementsByPathValueOnly(t *testing.T, path string) {
	t.Helper()
	for idShortPath, expected := range map[string]string{
		"prop":       `null`,
		"mlp":        `[]`,
		"rngmin":     `{"min": 1}`,
		"rngnone":    `{}`,
		"file":       `{"contentType": "text/plain"}`,
		"fileNone":   `{}`,
		"blobNone":   `{}`,
		"ref":        `null`,
		"emptyColl":  `{}`,
		"coll":       `{}`,
		"coll.inner": `null`,
		"members[1]": `null`,
		"members[4]": `{}`,
	} {
		require.JSONEq(t, expected, h.valueOnly(t, path+"/submodel-elements/"+idShortPath+"/$value", http.StatusOK), idShortPath)
	}
	h.valueOnly(t, path+"/submodel-elements/withOperation/$value", http.StatusNotFound)
}

func (h *idShortHarness) assertSubmodelsValueOnly(t *testing.T, id string, query string, expected string) {
	t.Helper()
	page := h.json(t, http.MethodGet, "/submodels/$value?cursor="+common.EncodeString(id)+"&limit=1"+query, nil, http.StatusOK)
	results := page["result"].([]any)
	require.Len(t, results, 1)
	actual, err := json.Marshal(results[0])
	require.NoError(t, err)
	require.JSONEq(t, expected, string(actual))
}

func (h *idShortHarness) assertValueOnlyRoundTrip(t *testing.T, path string) {
	t.Helper()
	before := h.json(t, http.MethodGet, path, nil, http.StatusOK)
	var valueOnly map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.valueOnly(t, path+"/$value", http.StatusOK)), &valueOnly))
	h.json(t, http.MethodPatch, path+"/$value", valueOnly, http.StatusNoContent)
	require.Equal(t, before, h.json(t, http.MethodGet, path, nil, http.StatusOK))
}
