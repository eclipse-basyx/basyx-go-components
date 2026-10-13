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

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

const valueOnlyNativeTypesSubmodelValue = `{
	"Count": 42,
	"Ratio": 42.0,
	"Enabled": true,
	"Serial": "42",
	"Unset": null,
	"Infinite": "INF",
	"Big": 18446744073709551615,
	"Amount": 1234567890.1234567890123456789,
	"Limits": {"min": 1, "max": 10},
	"Data": {"Count": 1, "Rows": [1, 2]},
	"Name": [{"en": "Name"}],
	"Asset": {"entityType": "SelfManagedEntity", "globalAssetId": "urn:example:asset", "statements": {"Weight": 1.5}},
	"Payload": {"contentType": "text/plain"}
}`

func TestValueOnlyNativeTypesRead(t *testing.T) {
	endpoint := createValueOnlyNativeTypesSubmodel(t)

	body := requireValueOnlyStatus(t, http.MethodGet, endpoint+"/$value", nil, http.StatusOK)
	require.JSONEq(t, valueOnlyNativeTypesSubmodelValue, string(body))
	require.Contains(t, string(body), `"Big":18446744073709551615`)
	require.Contains(t, string(body), `"Amount":1234567890.1234567890123456789`)

	body = requireValueOnlyStatus(t, http.MethodGet, endpoint+"/submodel-elements/Count/$value", nil, http.StatusOK)
	require.JSONEq(t, `42`, string(body))
	body = requireValueOnlyStatus(t, http.MethodGet, endpoint+"/submodel-elements/$value", nil, http.StatusOK)
	require.Contains(t, string(body), `{"Count":42}`)
}

func TestValueOnlyNativeTypesPatchRejectsMismatchingPayloads(t *testing.T) {
	endpoint := createValueOnlyNativeTypesSubmodel(t)

	tests := []struct {
		name   string
		path   string
		body   string
		status int
	}{
		{"numeric string for int", "Count", `"7"`, http.StatusBadRequest},
		{"boolean for int", "Count", `true`, http.StatusBadRequest},
		{"fraction for int", "Count", `1.5`, http.StatusBadRequest},
		{"object for int", "Count", `{"value":7}`, http.StatusBadRequest},
		{"number for string", "Serial", `42`, http.StatusBadRequest},
		{"string for boolean", "Enabled", `"true"`, http.StatusBadRequest},
		{"string range bound", "Limits", `{"min":"1"}`, http.StatusBadRequest},
		{"unknown range member", "Limits", `{"step":1}`, http.StatusBadRequest},
		{"multi language item with two languages", "Name", `[{"en":"a","de":"b"}]`, http.StatusBadRequest},
		{"legacy multi language shape", "Name", `[{"language":"en","text":"a"}]`, http.StatusBadRequest},
		{"invalid base64 blob", "Payload", `{"value":"not base64"}`, http.StatusBadRequest},
		{"unknown entity type", "Asset", `{"entityType":"Unknown"}`, http.StatusBadRequest},
		{"unknown collection child", "Data", `{"Unknown":1}`, http.StatusNotFound},
		{"too many list items", "Data.Rows", `[1,2,3]`, http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireValueOnlyStatus(t, http.MethodPatch, endpoint+"/submodel-elements/"+test.path+"/$value", json.RawMessage(test.body), test.status)
		})
	}

	requireValueOnlyStatus(t, http.MethodPatch, endpoint+"/$value", json.RawMessage(`{"Count":7,"Serial":5}`), http.StatusBadRequest)
	body := requireValueOnlyStatus(t, http.MethodGet, endpoint+"/$value", nil, http.StatusOK)
	require.JSONEq(t, valueOnlyNativeTypesSubmodelValue, string(body))
}

func TestValueOnlyNativeTypesPatchAppliesTypedValues(t *testing.T) {
	endpoint := createValueOnlyNativeTypesSubmodel(t)
	blob := []byte{0x00, 0xff, 'h', 'i'}

	patches := []struct {
		path string
		body string
	}{
		{"Count", `7`},
		{"Enabled", `false`},
		{"Infinite", `"-INF"`},
		{"Ratio", `null`},
		{"Limits", `{"max":20}`},
		{"Data.Rows", `[5]`},
		{"Name", `[{"en":"Updated"},{"de":"Aktualisiert"}]`},
		{"Asset", `{"entityType":"SelfManagedEntity","globalAssetId":"urn:example:asset:2"}`},
		{"Payload", fmt.Sprintf(`{"value":%q}`, base64.StdEncoding.EncodeToString(blob))},
	}
	for _, patch := range patches {
		requireValueOnlyStatus(t, http.MethodPatch, endpoint+"/submodel-elements/"+patch.path+"/$value", json.RawMessage(patch.body), http.StatusNoContent)
	}
	requireValueOnlyStatus(t, http.MethodPatch, endpoint+"/$value", json.RawMessage(`{"Big":0,"Data":{"Count":9007199254740993}}`), http.StatusNoContent)

	body := requireValueOnlyStatus(t, http.MethodGet, endpoint+"/$value?extent=withBlobValue", nil, http.StatusOK)
	require.JSONEq(t, fmt.Sprintf(`{
		"Count": 7,
		"Ratio": null,
		"Enabled": false,
		"Serial": "42",
		"Unset": null,
		"Infinite": "-INF",
		"Big": 0,
		"Amount": 1234567890.1234567890123456789,
		"Limits": {"min": 1, "max": 20},
		"Data": {"Count": 9007199254740993, "Rows": [5, 2]},
		"Name": [{"de": "Aktualisiert"}, {"en": "Updated"}],
		"Asset": {"entityType": "SelfManagedEntity", "globalAssetId": "urn:example:asset:2", "statements": {"Weight": 1.5}},
		"Payload": {"contentType": "text/plain", "value": %q}
	}`, base64.StdEncoding.EncodeToString(blob)), string(body))
	require.Contains(t, string(body), `"Count":9007199254740993`)
}

func createValueOnlyNativeTypesSubmodel(t *testing.T) string {
	t.Helper()
	submodelID := fmt.Sprintf("urn:basyx:integration:value-only-native-types-%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(submodelID)
	submodel := fmt.Sprintf(`{
		"id": %q,
		"idShort": "ValueOnlyNativeTypes",
		"modelType": "Submodel",
		"submodelElements": [
			{"idShort": "Count", "modelType": "Property", "valueType": "xs:int", "value": "42"},
			{"idShort": "Ratio", "modelType": "Property", "valueType": "xs:float", "value": "42.0"},
			{"idShort": "Enabled", "modelType": "Property", "valueType": "xs:boolean", "value": "true"},
			{"idShort": "Serial", "modelType": "Property", "valueType": "xs:string", "value": "42"},
			{"idShort": "Unset", "modelType": "Property", "valueType": "xs:int"},
			{"idShort": "Infinite", "modelType": "Property", "valueType": "xs:double", "value": "INF"},
			{"idShort": "Big", "modelType": "Property", "valueType": "xs:unsignedLong", "value": "18446744073709551615"},
			{"idShort": "Amount", "modelType": "Property", "valueType": "xs:decimal", "value": "1234567890.1234567890123456789"},
			{"idShort": "Limits", "modelType": "Range", "valueType": "xs:int", "min": "1", "max": "10"},
			{"idShort": "Data", "modelType": "SubmodelElementCollection", "value": [
				{"idShort": "Count", "modelType": "Property", "valueType": "xs:long", "value": "1"},
				{"idShort": "Rows", "modelType": "SubmodelElementList", "typeValueListElement": "Property", "valueTypeListElement": "xs:int", "value": [
					{"modelType": "Property", "valueType": "xs:int", "value": "1"},
					{"modelType": "Property", "valueType": "xs:int", "value": "2"}
				]}
			]},
			{"idShort": "Name", "modelType": "MultiLanguageProperty", "value": [{"language": "en", "text": "Name"}]},
			{"idShort": "Asset", "modelType": "Entity", "entityType": "SelfManagedEntity", "globalAssetId": "urn:example:asset", "statements": [
				{"idShort": "Weight", "modelType": "Property", "valueType": "xs:double", "value": "1.5"}
			]},
			{"idShort": "Payload", "modelType": "Blob", "contentType": "text/plain", "value": "aGVsbG8="}
		]
	}`, submodelID)
	requireValueOnlyStatus(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", json.RawMessage(submodel), http.StatusCreated)
	t.Cleanup(func() {
		_, _, _ = requestJSON(http.MethodDelete, endpoint, nil)
	})
	return endpoint
}

func requireValueOnlyStatus(t *testing.T, method string, endpoint string, payload any, expectedStatus int) []byte {
	t.Helper()
	status, body, err := requestJSON(method, endpoint, payload)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, status, "%s %s response: %s", method, endpoint, body)
	return body
}
