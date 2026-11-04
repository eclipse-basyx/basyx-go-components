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
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type elementIDShortCase struct {
	name   string
	method string
	path   string
	body   map[string]any
}

func TestSubmodelElementIDShortPresence(t *testing.T) {
	for _, run := range []struct {
		mode    string
		service idShortService
	}{
		{mode: "off", service: idShortService{name: "standalone"}},
		{mode: "permissive", service: idShortService{name: "standalone"}},
		{mode: "strict", service: idShortService{name: "standalone"}},
		{mode: "permissive", service: idShortService{name: "aas-repository", nested: true}},
		{mode: "permissive", service: idShortService{name: "environment", environment: true}},
	} {
		t.Run(run.mode+"/"+run.service.name, func(t *testing.T) {
			h := newIDShortHarness(t, run.mode, run.service)
			t.Run("rejected writes", h.testRejectedIDShortWrites)
			t.Run("rejected submodel creation", h.testRejectedSubmodelCreation)
			t.Run("nameless list item", h.testNamelessListItemIsAccepted)
			t.Run("patch of operation with stored nameless variable", h.testPatchKeepsStoredNamelessOperationVariable)
			if run.service.environment {
				t.Run("rejected upload", h.testRejectedUpload)
			}
			if run.service.name == "standalone" {
				t.Run("rejected dpp writes", h.testRejectedDPPWrites)
			}
		})
	}
}

func (h *idShortHarness) testRejectedIDShortWrites(t *testing.T) {
	id := h.seedElementIDShortSubmodel(t)
	before := h.json(t, http.MethodGet, h.path(id), nil, http.StatusOK)
	for _, testCase := range h.rejectedIDShortWrites(id) {
		t.Run(testCase.name, func(t *testing.T) {
			h.requireMissingIDShortRejection(t, h.json(t, testCase.method, testCase.path, testCase.body, http.StatusBadRequest))
		})
	}
	require.Equal(t, before, h.json(t, http.MethodGet, h.path(id), nil, http.StatusOK))
}

// requireMissingIDShortRejection checks that persistence rejected the write.
// Strict semantic verification may reject it first with its own error code.
func (h *idShortHarness) requireMissingIDShortRejection(t *testing.T, response map[string]any) {
	t.Helper()
	if h.mode != "strict" {
		requireMissingIDShortError(t, response)
	}
}

func requireMissingIDShortError(t *testing.T, response map[string]any) {
	t.Helper()
	body, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(body), "-MISSINGIDSHORT")
}

// rejectedIDShortWrites leaves out element writes for the environment, which
// routes them through the unchanged submodel repository service.
func (h *idShortHarness) rejectedIDShortWrites(id string) []elementIDShortCase {
	submodelWrites := []elementIDShortCase{
		{name: "put submodel", method: http.MethodPut, path: h.path(id), body: elementIDShortSubmodel(id, namelessProperty())},
		{name: "patch submodel", method: http.MethodPatch, path: h.path(id), body: elementIDShortSubmodel(id, namelessProperty())},
	}
	if h.service.environment {
		return submodelWrites
	}
	elements := h.path(id) + "/submodel-elements"
	return append([]elementIDShortCase{
		{name: "top-level omitted", method: http.MethodPost, path: elements, body: namelessProperty()},
		{name: "top-level empty", method: http.MethodPost, path: elements, body: namedProperty("")},
		{name: "collection child", method: http.MethodPost, path: elements + "/Coll", body: namelessProperty()},
		{name: "entity statement", method: http.MethodPost, path: elements + "/Ent", body: namelessProperty()},
		{name: "annotation", method: http.MethodPost, path: elements + "/Rel", body: namelessProperty()},
		{name: "nested in posted element", method: http.MethodPost, path: elements, body: collection("Outer", namelessProperty())},
		{name: "operation variable", method: http.MethodPost, path: elements, body: operationWithInput(namelessProperty())},
		{name: "child of list item", method: http.MethodPost, path: elements + "/List", body: collection("", namelessProperty())},
		{name: "put replacement child", method: http.MethodPut, path: elements + "/Coll", body: collection("Coll", namelessProperty())},
		{name: "put creation child", method: http.MethodPut, path: elements + "/NewColl", body: collection("NewColl", namelessProperty())},
		{name: "patch child", method: http.MethodPatch, path: elements + "/Coll", body: collection("Coll", namelessProperty())},
	}, submodelWrites...)
}

func (h *idShortHarness) testRejectedSubmodelCreation(t *testing.T) {
	id := "urn:idshort:element:" + uuid.NewString()
	h.requireMissingIDShortRejection(t, h.create(t, elementIDShortSubmodel(id, collection("Coll", namelessProperty())), http.StatusBadRequest))
	h.json(t, http.MethodGet, h.path(id), nil, http.StatusNotFound)
}

func (h *idShortHarness) testRejectedUpload(t *testing.T) {
	id := "urn:idshort:element:" + uuid.NewString()
	h.track(t, id)
	environment, err := json.Marshal(map[string]any{"submodels": []any{elementIDShortSubmodel(id, collection("Coll", namelessProperty()))}})
	require.NoError(t, err)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "environment.json")
	require.NoError(t, err)
	_, err = file.Write(environment)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	response := h.request(t, http.MethodPost, "/upload", body.Bytes(), writer.FormDataContentType())
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "-MISSINGIDSHORT")
	h.json(t, http.MethodGet, h.path(id), nil, http.StatusNotFound)
}

// testRejectedDPPWrites expects 409 because DPP maps the 400 from persistence
// to its fallback status; correcting that mapping is planned separately.
func (h *idShortHarness) testRejectedDPPWrites(t *testing.T) {
	const section = "urn:idshort:dpp:technical-data"
	rejectedID := "urn:idshort:dpp:" + uuid.NewString()
	rejectedDocument := dppDocument(rejectedID, section, map[string]any{"Dimensions": map[string]any{"": 5}})
	requireMissingIDShortError(t, h.json(t, http.MethodPost, "/v1/dpps", rejectedDocument, http.StatusConflict))
	h.json(t, http.MethodGet, dppPath(rejectedID), nil, http.StatusNotFound)

	id := "urn:idshort:dpp:" + uuid.NewString()
	h.json(t, http.MethodPost, "/v1/dpps", dppDocument(id, section, map[string]any{"Dimensions": map[string]any{"HeightMm": 80}}), http.StatusCreated)
	t.Cleanup(func() { h.json(t, http.MethodDelete, dppPath(id), nil, http.StatusNoContent) })
	element := dppPath(id) + "/elements/" + url.PathEscape("$['"+section+"']['Dimensions']")
	before := h.json(t, http.MethodGet, element, nil, http.StatusOK)
	requireMissingIDShortError(t, h.json(t, http.MethodPatch, element, map[string]any{"HeightMm": 80, "": 5}, http.StatusConflict))
	require.Equal(t, before, h.json(t, http.MethodGet, element, nil, http.StatusOK))
}

func dppDocument(id string, section string, content map[string]any) map[string]any {
	return map[string]any{
		"digitalProductPassportId": id,
		"uniqueProductIdentifier":  id + ":product",
		"granularity":              "Item",
		"dppSchemaVersion":         "1.0.0",
		"dppStatus":                "active",
		"lastUpdate":               "2026-01-02T03:04:05Z",
		"economicOperatorId":       "operator-1",
		"contentSpecificationIds":  []any{section},
		section:                    content,
	}
}

func dppPath(id string) string {
	return "/v1/dpps/" + url.PathEscape(id)
}

func (h *idShortHarness) testNamelessListItemIsAccepted(t *testing.T) {
	id := h.seedElementIDShortSubmodel(t)
	h.json(t, http.MethodPost, h.path(id)+"/submodel-elements/List", collection("", namedProperty("Child")), http.StatusCreated)
	item := h.json(t, http.MethodGet, h.path(id)+"/submodel-elements/List[0].Child", nil, http.StatusOK)
	require.Equal(t, "Child", item["idShort"])
}

// testPatchKeepsStoredNamelessOperationVariable covers Operations stored before
// AASd-117 was enforced: a PATCH that does not send the variables must neither
// be rejected because of them nor change them. Strict mode verifies the
// persisted element and therefore still rejects such data.
func (h *idShortHarness) testPatchKeepsStoredNamelessOperationVariable(t *testing.T) {
	id := h.seedElementIDShortSubmodel(t)
	elements := h.path(id) + "/submodel-elements"
	h.json(t, http.MethodPost, elements, operationWithInput(namedProperty("Input")), http.StatusCreated)
	h.storeLegacyOperationInput(t, id, []any{map[string]any{"value": namelessProperty()}})
	before := h.json(t, http.MethodGet, elements+"/Op", nil, http.StatusOK)

	expectedStatus := http.StatusNoContent
	if h.mode == "strict" {
		expectedStatus = http.StatusBadRequest
	}
	description := func(text string) map[string]any {
		return map[string]any{"modelType": "Operation", "idShort": "Op", "description": []any{map[string]any{"language": "en", "text": text}}}
	}
	h.json(t, http.MethodPatch, elements+"/Op", description("patched"), expectedStatus)
	h.json(t, http.MethodPatch, elements+"/Op/$metadata", description("patched metadata"), expectedStatus)

	after := h.json(t, http.MethodGet, elements+"/Op", nil, http.StatusOK)
	require.Equal(t, before["inputVariables"], after["inputVariables"])
	if expectedStatus == http.StatusNoContent {
		require.Equal(t, description("patched metadata")["description"], after["description"])
	}
}

func (h *idShortHarness) storeLegacyOperationInput(t *testing.T, submodelID string, inputVariables []any) {
	t.Helper()
	inputJSON, err := json.Marshal(inputVariables)
	require.NoError(t, err)
	dialect := goqu.Dialect(common.Dialect)
	submodelDatabaseID := dialect.From("submodel").Select("id").Where(goqu.Ex{"submodel_identifier": submodelID})
	operationID := dialect.From("submodel_element").Select("id").Where(goqu.C("submodel_id").Eq(submodelDatabaseID), goqu.Ex{"idshort_path": "Op"})
	query, args, err := dialect.Update("operation_element").Set(goqu.Record{"input_variables": string(inputJSON)}).Where(goqu.C("id").Eq(operationID)).ToSQL()
	require.NoError(t, err)
	result, err := h.db.ExecContext(h.ctx, query, args...)
	require.NoError(t, err)
	updated, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
}

func (h *idShortHarness) seedElementIDShortSubmodel(t *testing.T) string {
	t.Helper()
	id := "urn:idshort:element:" + uuid.NewString()
	reference := map[string]any{"type": "ModelReference", "keys": []any{map[string]any{"type": "Submodel", "value": id}}}
	h.create(t, elementIDShortSubmodel(id,
		collection("Coll", namedProperty("Child")),
		map[string]any{"modelType": "Entity", "idShort": "Ent", "entityType": "CoManagedEntity", "statements": []any{namedProperty("Statement")}},
		map[string]any{"modelType": "AnnotatedRelationshipElement", "idShort": "Rel", "first": reference, "second": reference, "annotations": []any{namedProperty("Annotation")}},
		map[string]any{"modelType": "SubmodelElementList", "idShort": "List", "typeValueListElement": "SubmodelElementCollection"},
	), http.StatusCreated)
	return id
}

func elementIDShortSubmodel(id string, elements ...map[string]any) map[string]any {
	submodelElements := make([]any, 0, len(elements))
	for _, element := range elements {
		submodelElements = append(submodelElements, element)
	}
	return map[string]any{"modelType": "Submodel", "id": id, "idShort": "ElementIDShorts", "submodelElements": submodelElements}
}

func namelessProperty() map[string]any {
	return map[string]any{"modelType": "Property", "valueType": "xs:string", "value": "nameless"}
}

func namedProperty(idShort string) map[string]any {
	property := namelessProperty()
	property["idShort"] = idShort
	return property
}

func collection(idShort string, children ...map[string]any) map[string]any {
	value := make([]any, 0, len(children))
	for _, child := range children {
		value = append(value, child)
	}
	element := map[string]any{"modelType": "SubmodelElementCollection", "value": value}
	if idShort != "" {
		element["idShort"] = idShort
	}
	return element
}

func operationWithInput(value map[string]any) map[string]any {
	return map[string]any{"modelType": "Operation", "idShort": "Op", "inputVariables": []any{map[string]any{"value": value}}}
}
