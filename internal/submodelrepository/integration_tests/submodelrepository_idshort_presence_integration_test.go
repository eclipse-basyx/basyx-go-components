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
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasapi "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	aasdb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	cddb "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	smapi "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
	smdb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	aasgen "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
	smgen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type idShortService struct {
	name        string
	environment bool
	nested      bool
	sync        bool
}

type idShortHarness struct {
	db      *sql.DB
	ctx     context.Context
	backend *smdb.SubmodelDatabase
	handler http.Handler
	mode    string
	service idShortService
	shellID string
}

func TestSubmodelIDShortPresence(t *testing.T) {
	for _, mode := range []string{"off", "permissive", "strict"} {
		t.Run(mode, func(t *testing.T) {
			h := newIDShortHarness(t, mode, idShortService{name: "standalone"})
			t.Run("creation", h.testCreation)
			t.Run("replacement", h.testReplacement)
			t.Run("patch", h.testPatch)
		})
	}
}

func TestSubmodelIDShortPresenceThroughServiceRoutes(t *testing.T) {
	for _, service := range []idShortService{
		{name: "aas-repository", nested: true},
		{name: "environment", environment: true},
		{name: "environment-nested", environment: true, nested: true},
		{name: "environment-sync", environment: true, sync: true},
		{name: "environment-sync-nested", environment: true, nested: true, sync: true},
	} {
		t.Run(service.name, func(t *testing.T) {
			h := newIDShortHarness(t, "strict", service)
			id := h.seed(t, nil)
			h.assertState(t, id, nil)
			h.json(t, http.MethodPatch, h.path(id)+"/$metadata", idShortMetadataUpdate(id), http.StatusNoContent)
			h.assertState(t, id, nil)
			assertIDShort(t, h.json(t, http.MethodGet, h.path(id)+"/$metadata", nil, http.StatusOK), nil)
			if service.nested {
				h.json(t, http.MethodPut, h.path(id), idShortPayload(id, new("ExistingName")), http.StatusNoContent)
				payload := idShortPayload(id, nil)
				payload["idShort"] = nil
				h.json(t, http.MethodPut, h.path(id), payload, http.StatusNoContent)
				h.assertState(t, id, nil)
			}
		})
	}
}

func TestSubmodelIDShortHistoryPreservesOmission(t *testing.T) {
	h := newIDShortHarness(t, "strict", idShortService{name: "standalone"})
	history.Configure(history.Config{Mode: history.ModeAPI})
	id := h.seed(t, nil)
	h.json(t, http.MethodPatch, h.path(id)+"/$metadata", idShortMetadataUpdate(id), http.StatusNoContent)
	historical, err := h.backend.GetSubmodelByIDAndDate(h.ctx, id, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Nil(t, historical.IDShort())
}

func idShortMetadataUpdate(id string) map[string]any {
	payload := idShortPayload(id, nil)
	payload["description"] = []any{map[string]any{"language": "en", "text": "updated"}}
	return payload
}

func newIDShortHarness(t *testing.T, mode string, service idShortService) *idShortHarness {
	t.Helper()
	previous := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff})
	t.Cleanup(func() { history.Configure(previous) })
	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	cfg := &common.Config{}
	cfg.Server.StrictVerification = mode
	cfg.Server.Pagination.DefaultLimit = 100
	cfg.Server.Pagination.MaxLimit = 1000
	ctx := common.ContextWithConfig(context.WithoutCancel(t.Context()), cfg)
	backend, err := smdb.NewSubmodelDatabaseFromDB(db, nil, mode)
	require.NoError(t, err)
	shells, err := aasdb.NewAssetAdministrationShellDatabaseFromDB(db, mode)
	require.NoError(t, err)
	persistence := &aasenvironment.Persistence{DB: db, SubmodelRepository: backend, AASRepository: shells}
	baseSubmodels := smapi.NewSubmodelRepositoryAPIAPIService(ctx, *backend, nil)
	baseAAS := aasapi.NewAssetAdministrationShellRepositoryAPIAPIService(ctx, shells, backend, false)
	var submodels smgen.SubmodelRepositoryAPIAPIServicer = baseSubmodels
	var aas aasgen.AssetAdministrationShellRepositoryAPIAPIServicer = baseAAS
	if service.environment {
		persistence.ConceptDescriptionRepository, err = cddb.NewConceptDescriptionBackendFromDB(db, mode)
		require.NoError(t, err)
		persistence.AASRegistry, err = aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromDB(db, false)
		require.NoError(t, err)
		persistence.SubmodelRegistry, err = smregistrydb.NewPostgreSQLSMBackendFromDB(db)
		require.NoError(t, err)
		syncConfig, syncErr := aasenvironment.NewRegistrySyncConfig(service.sync, service.sync, "http://idshort.example")
		require.NoError(t, syncErr)
		submodels = aasenvironment.NewCustomSubmodelRepositoryService(baseSubmodels, persistence, syncConfig)
		aas = aasenvironment.NewCustomAASRepositoryService(baseAAS, persistence, syncConfig)
	}
	router := chi.NewRouter()
	router.Use(common.ConfigMiddleware(cfg))
	for _, route := range smgen.NewSubmodelRepositoryAPIAPIController(submodels, "", mode).Routes() {
		router.Method(route.Method, route.Pattern, route.HandlerFunc)
	}
	for _, route := range aasgen.NewAssetAdministrationShellRepositoryAPIAPIController(aas, "", mode).Routes() {
		router.Method(route.Method, route.Pattern, route.HandlerFunc)
	}
	if service.environment {
		stager := binarycontent.NewStager(db)
		aasenvironment.RegisterSerializationAPI(router, aasenvironment.NewSerializationAPIService(persistence, stager))
		aasenvironment.RegisterUploadAPI(router, aasenvironment.NewUploadAPIService(persistence, aas, submodels), 1<<20, stager)
	}
	h := &idShortHarness{db: db, ctx: ctx, backend: backend, handler: router, mode: mode, service: service}
	if service.nested {
		h.shellID = "urn:idshort:shell:" + uuid.NewString()
		h.json(t, http.MethodPost, "/shells", map[string]any{
			"modelType": "AssetAdministrationShell", "id": h.shellID,
			"assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": h.shellID + ":asset"},
		}, http.StatusCreated)
		t.Cleanup(func() {
			h.json(t, http.MethodDelete, "/shells/"+common.EncodeString(h.shellID), nil, http.StatusNoContent)
		})
	}
	return h
}

func (h *idShortHarness) request(t *testing.T, method, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(context.WithoutCancel(t.Context()), method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func (h *idShortHarness) json(t *testing.T, method, path string, body any, status int) map[string]any {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	response := h.request(t, method, path, payload, "application/json")
	require.Equal(t, status, response.Code, "%s %s: %s", method, path, response.Body.String())
	result := map[string]any{}
	if response.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	}
	return result
}

func (h *idShortHarness) path(id string) string {
	prefix := ""
	if h.service.nested {
		prefix = "/shells/" + common.EncodeString(h.shellID)
	}
	return prefix + "/submodels/" + common.EncodeString(id)
}

func idShortPayload(id string, idShort *string) map[string]any {
	payload := map[string]any{"modelType": "Submodel", "id": id}
	if idShort != nil {
		payload["idShort"] = *idShort
	}
	return payload
}

func (h *idShortHarness) track(t *testing.T, id string) {
	t.Helper()
	t.Cleanup(func() {
		response := h.request(t, http.MethodDelete, h.path(id), nil, "application/json")
		require.Contains(t, []int{http.StatusNoContent, http.StatusNotFound}, response.Code, response.Body.String())
	})
}

func (h *idShortHarness) create(t *testing.T, payload map[string]any, status int) map[string]any {
	t.Helper()
	id := payload["id"].(string)
	h.track(t, id)
	if h.service.nested {
		return h.json(t, http.MethodPut, h.path(id), payload, status)
	}
	return h.json(t, http.MethodPost, "/submodels", payload, status)
}

func (h *idShortHarness) seed(t *testing.T, idShort *string) string {
	t.Helper()
	id := "urn:idshort:submodel:" + uuid.NewString()
	payload := idShortPayload(id, idShort)
	if h.mode != "strict" || idShort == nil || *idShort != "" {
		h.create(t, payload, http.StatusCreated)
		return id
	}
	backend, err := smdb.NewSubmodelDatabaseFromDB(h.db, nil, "off")
	require.NoError(t, err)
	submodel, err := jsonization.SubmodelFromJsonable(payload)
	require.NoError(t, err)
	require.NoError(t, backend.CreateSubmodel(h.ctx, submodel))
	h.track(t, id)
	if h.service.nested {
		h.json(t, http.MethodPost, "/shells/"+common.EncodeString(h.shellID)+"/submodel-refs", map[string]any{
			"type": "ModelReference", "keys": []any{map[string]any{"type": "Submodel", "value": id}},
		}, http.StatusCreated)
	}
	return id
}

func assertIDShort(t *testing.T, document map[string]any, expected *string) {
	t.Helper()
	value, present := document["idShort"]
	require.Equal(t, expected != nil, present, "idShort presence: %v", document)
	if expected != nil {
		require.Equal(t, *expected, value)
	}
}

func (h *idShortHarness) assertState(t *testing.T, id string, expected *string) {
	t.Helper()
	query, args, err := goqu.Dialect(common.Dialect).From("submodel").Select("id_short").Where(goqu.Ex{"submodel_identifier": id}).ToSQL()
	require.NoError(t, err)
	var stored sql.NullString
	require.NoError(t, h.db.QueryRowContext(h.ctx, query, args...).Scan(&stored))
	require.Equal(t, expected != nil, stored.Valid)
	if expected != nil {
		require.Equal(t, *expected, stored.String)
	}
	assertIDShort(t, h.json(t, http.MethodGet, h.path(id), nil, http.StatusOK), expected)
}

func (h *idShortHarness) assertLists(t *testing.T, id string, expected *string) {
	t.Helper()
	for _, suffix := range []string{"", "/$metadata"} {
		page := h.json(t, http.MethodGet, "/submodels"+suffix+"?cursor="+common.EncodeString(id)+"&limit=1", nil, http.StatusOK)
		results := page["result"].([]any)
		require.Len(t, results, 1)
		document := results[0].(map[string]any)
		require.Equal(t, id, document["id"])
		assertIDShort(t, document, expected)
	}
	query := map[string]any{"$condition": map[string]any{"$eq": []any{map[string]any{"$field": "$sm#id"}, map[string]any{"$strVal": id}}}}
	page := h.json(t, http.MethodPost, "/query/submodels", query, http.StatusOK)
	results := page["result"].([]any)
	require.Len(t, results, 1)
	assertIDShort(t, results[0].(map[string]any), expected)
}

func (h *idShortHarness) testCreation(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		idShort *string
		null    bool
	}{
		{name: "omitted"}, {name: "null", null: true},
		{name: "empty", idShort: new("")}, {name: "populated", idShort: new("CreatedName")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			id := "urn:idshort:submodel:" + uuid.NewString()
			payload := idShortPayload(id, testCase.idShort)
			if testCase.null {
				payload["idShort"] = nil
			}
			status := http.StatusCreated
			if h.mode == "strict" && testCase.idShort != nil && *testCase.idShort == "" {
				status = http.StatusBadRequest
			}
			result := h.create(t, payload, status)
			if status == http.StatusBadRequest {
				h.json(t, http.MethodGet, h.path(id), nil, http.StatusNotFound)
				return
			}
			assertIDShort(t, result, testCase.idShort)
			h.assertState(t, id, testCase.idShort)
			assertIDShort(t, h.json(t, http.MethodGet, h.path(id)+"/$metadata", nil, http.StatusOK), testCase.idShort)
			if testCase.name == "omitted" || testCase.name == "populated" {
				h.assertLists(t, id, testCase.idShort)
			}
		})
	}
	t.Run("put creation", func(t *testing.T) {
		id := "urn:idshort:submodel:" + uuid.NewString()
		h.track(t, id)
		result := h.json(t, http.MethodPut, h.path(id), idShortPayload(id, nil), http.StatusCreated)
		assertIDShort(t, result, nil)
		h.assertState(t, id, nil)
	})
}

func (h *idShortHarness) testReplacement(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		previous *string
		target   *string
		null     bool
	}{
		{name: "absent unchanged"},
		{name: "populated to omitted", previous: new("ExistingName")},
		{name: "empty to omitted", previous: new("")},
		{name: "absent to empty", target: new("")},
		{name: "explicit null", previous: new("ExistingName"), null: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			id := h.seed(t, testCase.previous)
			payload := idShortPayload(id, testCase.target)
			if testCase.null {
				payload["idShort"] = nil
			}
			status := http.StatusNoContent
			expected := testCase.target
			if (testCase.null && !h.service.nested) || (h.mode == "strict" && testCase.target != nil && *testCase.target == "") {
				status = http.StatusBadRequest
				expected = testCase.previous
			}
			h.json(t, http.MethodPut, h.path(id), payload, status)
			h.assertState(t, id, expected)
		})
	}
}

type idShortPatchCase struct {
	variant  string
	name     string
	previous *string
	value    *string
	null     bool
}

func (h *idShortHarness) testPatch(t *testing.T) {
	for _, testCase := range []idShortPatchCase{
		{variant: "normal", name: "omit absent"},
		{variant: "normal", name: "omit populated", previous: new("ExistingName")},
		{variant: "normal", name: "null populated", previous: new("ExistingName"), null: true},
		{variant: "normal", name: "set empty", value: new("")},
		{variant: "normal", name: "set populated", value: new("PatchedName")},
		{variant: "metadata", name: "omit absent"},
		{variant: "metadata", name: "omit populated", previous: new("ExistingName")},
		{variant: "metadata", name: "null populated", previous: new("ExistingName"), null: true},
		{variant: "metadata", name: "set empty", value: new("")},
		{variant: "elements", name: "omit absent"},
		{variant: "elements", name: "omit populated", previous: new("ExistingName")},
	} {
		t.Run(testCase.variant+"/"+testCase.name, func(t *testing.T) { h.checkPatch(t, testCase) })
	}
}

func (h *idShortHarness) checkPatch(t *testing.T, testCase idShortPatchCase) {
	t.Helper()
	id := h.seed(t, testCase.previous)
	payload := idShortPayload(id, testCase.value)
	payload["description"] = []any{map[string]any{"language": "en", "text": "updated"}}
	path := h.path(id)
	if testCase.null {
		payload["idShort"] = nil
	}
	if testCase.variant == "metadata" {
		path += "/$metadata"
	}
	if testCase.variant == "elements" {
		payload["submodelElements"] = []any{map[string]any{"modelType": "Property", "idShort": "Reading", "valueType": "xs:string", "value": "updated"}}
	}
	expected := testCase.previous
	if testCase.value != nil || (testCase.null && testCase.variant == "metadata") {
		expected = testCase.value
	}
	status := http.StatusNoContent
	if h.mode == "strict" && testCase.value != nil && *testCase.value == "" {
		status = http.StatusBadRequest
		expected = testCase.previous
	}
	h.json(t, http.MethodPatch, path, payload, status)
	h.assertState(t, id, expected)
}
