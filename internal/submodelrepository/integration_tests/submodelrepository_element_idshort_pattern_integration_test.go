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
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
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
			t.Run("stored invalid idShort", h.testStoredInvalidIDShort)
			t.Run("nameless PUT retains invalid idShort", h.testPutRetainsStoredInvalidIDShort)
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

// testStoredInvalidIDShort covers elements stored before the idShort pattern
// was enforced: they stay readable and deletable by their raw idShortPath, a
// percent-encoded path still addresses no element, and they can only be
// updated by a PUT that renames them to a valid idShort.
func (h *idShortHarness) testStoredInvalidIDShort(t *testing.T) {
	id := h.seedElementIDShortSubmodel(t)
	h.renameStoredElement(t, id, "Coll.Child", "a;b")
	elements := h.path(id) + "/submodel-elements"

	require.Equal(t, "a;b", h.json(t, http.MethodGet, elements+"/Coll.a;b", nil, http.StatusOK)["idShort"])
	h.json(t, http.MethodGet, elements+"/Coll.a%3Bb", nil, http.StatusNotFound)
	if h.service.environment {
		return
	}

	description := map[string]any{"modelType": "Property", "valueType": "xs:string", "description": []any{map[string]any{"language": "en", "text": "patched"}}}
	h.requireInvalidIDShortRejection(t, h.json(t, http.MethodPatch, elements+"/Coll.a;b", description, http.StatusBadRequest))
	h.requireInvalidIDShortRejection(t, h.json(t, http.MethodPatch, elements+"/Coll.a;b/$metadata", description, http.StatusBadRequest))
	h.requireInvalidIDShortRejection(t, h.json(t, http.MethodPut, elements+"/Coll.a;b", namedProperty("a;b"), http.StatusBadRequest))
	h.requireInvalidIDShortRejection(t, h.json(t, http.MethodPut, elements+"/Coll.a;b", namelessProperty(), http.StatusBadRequest))

	h.json(t, http.MethodPut, elements+"/Coll.a;b", namedProperty("Renamed"), http.StatusNoContent)
	h.json(t, http.MethodGet, elements+"/Coll.a;b", nil, http.StatusNotFound)
	require.Equal(t, "Renamed", h.json(t, http.MethodGet, elements+"/Coll.Renamed", nil, http.StatusOK)["idShort"])

	h.renameStoredElement(t, id, "Coll.Renamed", "x y")
	h.json(t, http.MethodDelete, elements+"/Coll.x%20y", nil, http.StatusNoContent)
	h.json(t, http.MethodGet, elements+"/Coll.x%20y", nil, http.StatusNotFound)
}

func (h *idShortHarness) testPutRetainsStoredInvalidIDShort(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		idShort  string
		listItem bool
	}{
		{name: "dot in name", idShort: "Old.Name"},
		{name: "unmatched bracket in name", idShort: "a[broken"},
		{name: "list item name", idShort: "a;b", listItem: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			id := h.seedElementIDShortSubmodel(t)
			path := "Coll." + testCase.idShort
			payload := namelessProperty()
			if testCase.listItem {
				path = "List[0]"
				h.json(t, http.MethodPost, h.path(id)+"/submodel-elements/List", collection("Item", namedProperty("Child")), http.StatusCreated)
				h.storeElementIDShort(t, id, path, testCase.idShort, path)
				payload = collection("", namedProperty("Child"))
			} else {
				h.renameStoredElement(t, id, "Coll.Child", testCase.idShort)
			}
			before := h.json(t, http.MethodGet, h.path(id), nil, http.StatusOK)
			response := h.json(t, http.MethodPut, h.path(id)+"/submodel-elements/"+path, payload, http.StatusBadRequest)
			h.requireInvalidIDShortRejection(t, response)
			require.Equal(t, before, h.json(t, http.MethodGet, h.path(id), nil, http.StatusOK))
		})
	}
}

func (h *idShortHarness) renameStoredElement(t *testing.T, submodelID string, idShortPath string, idShort string) {
	t.Helper()
	parentPath := idShortPath[:strings.LastIndex(idShortPath, ".")+1]
	h.storeElementIDShort(t, submodelID, idShortPath, idShort, parentPath+idShort)
}

func (h *idShortHarness) storeElementIDShort(t *testing.T, submodelID, idShortPath, idShort, updatedPath string) {
	t.Helper()
	dialect := goqu.Dialect(common.Dialect)
	submodelDatabaseID := dialect.From("submodel").Select("id").Where(goqu.Ex{"submodel_identifier": submodelID})
	query, args, err := dialect.Update("submodel_element").
		Set(goqu.Record{"id_short": idShort, "idshort_path": updatedPath}).
		Where(goqu.C("submodel_id").Eq(submodelDatabaseID), goqu.Ex{"idshort_path": idShortPath}).
		ToSQL()
	require.NoError(t, err)
	result, err := h.db.ExecContext(h.ctx, query, args...)
	require.NoError(t, err)
	updated, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
}
