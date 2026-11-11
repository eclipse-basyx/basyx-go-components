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

	"github.com/stretchr/testify/require"
)

func TestSubmodelElementIDShortPattern(t *testing.T) {
	for _, run := range []struct {
		mode    string
		service idShortService
	}{
		{mode: "off", service: idShortService{name: "standalone"}},
		{mode: "permissive", service: idShortService{name: "standalone"}},
		{mode: "strict", service: idShortService{name: "standalone"}},
		{mode: "off", service: idShortService{name: "aas-repository", nested: true}},
		{mode: "off", service: idShortService{name: "environment", environment: true}},
	} {
		t.Run(run.mode+"/"+run.service.name, func(t *testing.T) {
			h := newIDShortHarness(t, run.mode, run.service)
			t.Run("rejected writes", h.testRejectedInvalidIDShortWrites)
		})
	}
}

func (h *idShortHarness) testRejectedInvalidIDShortWrites(t *testing.T) {
	id := h.seedElementIDShortSubmodel(t)
	before := h.json(t, http.MethodGet, h.path(id), nil, http.StatusOK)
	for _, testCase := range h.invalidIDShortWrites(id) {
		t.Run(testCase.name, func(t *testing.T) {
			h.requireInvalidIDShortRejection(t, h.json(t, testCase.method, testCase.path, testCase.body, http.StatusBadRequest))
		})
	}
	require.Equal(t, before, h.json(t, http.MethodGet, h.path(id), nil, http.StatusOK))
}

// requireInvalidIDShortRejection checks that persistence rejected the write.
// Strict semantic verification may reject it first with its own error code.
func (h *idShortHarness) requireInvalidIDShortRejection(t *testing.T, response map[string]any) {
	t.Helper()
	if h.mode == "strict" {
		return
	}
	body, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(body), "-INVALIDIDSHORT")
}

// invalidIDShortWrites leaves out element writes for the environment, which
// routes them through the unchanged submodel repository service.
func (h *idShortHarness) invalidIDShortWrites(id string) []elementIDShortCase {
	invalidSubmodel := idShortPayload(id, new("Sub:model"))
	emptyListItem := collection("", namedProperty("Child"))
	emptyListItem["idShort"] = ""
	submodelWrites := []elementIDShortCase{
		{name: "put submodel element", method: http.MethodPut, path: h.path(id), body: elementIDShortSubmodel(id, namedProperty("a;b"))},
		{name: "patch submodel element", method: http.MethodPatch, path: h.path(id), body: elementIDShortSubmodel(id, collection("Coll", namedProperty("x")))},
		{name: "put submodel idShort", method: http.MethodPut, path: h.path(id), body: invalidSubmodel},
		{name: "empty submodel idShort", method: http.MethodPut, path: h.path(id), body: idShortPayload(id, new(""))},
		{name: "empty nested list item", method: http.MethodPut, path: h.path(id), body: elementIDShortSubmodel(id, map[string]any{
			"modelType": "SubmodelElementList", "idShort": "List", "typeValueListElement": "SubmodelElementCollection", "value": []any{emptyListItem},
		})},
		{name: "patch submodel metadata idShort", method: http.MethodPatch, path: h.path(id) + "/$metadata", body: invalidSubmodel},
	}
	if h.service.environment {
		return submodelWrites
	}
	elements := h.path(id) + "/submodel-elements"
	return append([]elementIDShortCase{
		{name: "top-level empty", method: http.MethodPost, path: elements, body: namedProperty("")},
		{name: "empty list item", method: http.MethodPost, path: elements + "/List", body: emptyListItem},
		{name: "empty PUT idShort", method: http.MethodPut, path: elements + "/Coll.Child", body: namedProperty("")},
		{name: "empty PATCH idShort", method: http.MethodPatch, path: elements + "/Coll.Child", body: namedProperty("")},
		{name: "empty metadata PATCH idShort", method: http.MethodPatch, path: elements + "/Coll.Child/$metadata", body: map[string]any{"modelType": "Property", "valueType": "xs:string", "idShort": ""}},
		{name: "top-level", method: http.MethodPost, path: elements, body: namedProperty("a;b")},
		{name: "collection child", method: http.MethodPost, path: elements + "/Coll", body: namedProperty("a b")},
		{name: "named list item", method: http.MethodPost, path: elements + "/List", body: collection("1st", namedProperty("Child"))},
		{name: "child of list item", method: http.MethodPost, path: elements + "/List", body: collection("", namedProperty("a.b"))},
		{name: "operation variable", method: http.MethodPost, path: elements, body: operationWithInput(namedProperty("In-"))},
		{name: "put creation from path", method: http.MethodPut, path: elements + "/Coll.a;b", body: namelessProperty()},
		{name: "put creation from encoded path", method: http.MethodPut, path: elements + "/Coll.a%3Bb", body: namelessProperty()},
		{name: "put replacement idShort", method: http.MethodPut, path: elements + "/Coll", body: collection("C/oll", namedProperty("Child"))},
		{name: "put replacement child", method: http.MethodPut, path: elements + "/Coll", body: collection("Coll", namedProperty("x"))},
		{name: "patch child", method: http.MethodPatch, path: elements + "/Coll", body: collection("Coll", namedProperty("Child"), namedProperty("é1"))},
	}, submodelWrites...)
}
