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
	"database/sql"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	aasdb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	cdapi "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/api"
	cddb "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	smdb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	cdopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/conceptdescriptionrepositoryapi/go"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type conceptDescriptionVerificationFixture struct {
	name      string
	fields    map[string]any
	violation string
}

func conceptDescriptionVerificationFixtures() []conceptDescriptionVerificationFixture {
	return []conceptDescriptionVerificationFixture{
		{name: "valid-idShort", fields: map[string]any{"idShort": "Valid-name_01"}},
		{name: "omitted-idShort"},
		{name: "invalid-idShort", fields: map[string]any{"idShort": "1Invalid"}, violation: "ID-short"},
		{
			name: "revision-without-version",
			fields: map[string]any{
				"idShort":        "Valid_01",
				"administration": map[string]any{"revision": "1"},
			},
			violation: "AASd-005",
		},
	}
}

func (fixture conceptDescriptionVerificationFixture) payload(id string) map[string]any {
	payload := map[string]any{"modelType": "ConceptDescription", "id": id}
	for key, value := range fixture.fields {
		payload[key] = value
	}
	return payload
}

type conceptDescriptionVerificationOperation struct {
	name    string
	method  string
	replace bool
	upload  bool
}

func conceptDescriptionVerificationOperations(service string) []conceptDescriptionVerificationOperation {
	operations := []conceptDescriptionVerificationOperation{
		{name: "POST-create", method: http.MethodPost},
		{name: "PUT-create", method: http.MethodPut},
		{name: "PUT-replace", method: http.MethodPut, replace: true},
	}
	if service == "environment" {
		operations = append(operations,
			conceptDescriptionVerificationOperation{name: "upload-create", upload: true},
			conceptDescriptionVerificationOperation{name: "upload-replace", replace: true, upload: true},
		)
	}
	return operations
}

func TestConceptDescriptionSemanticVerification(t *testing.T) {
	db := conceptDescriptionVerificationDB(t)
	for _, service := range []string{"standalone", "environment"} {
		t.Run(service, func(t *testing.T) {
			for _, mode := range conceptDescriptionVerificationModes() {
				t.Run(string(mode), func(t *testing.T) {
					router := conceptDescriptionVerificationRouter(t, db, service, mode)
					testConceptDescriptionVerificationFixtures(t, db, router, service, mode)
				})
			}
		})
	}
}

func conceptDescriptionVerificationModes() []model.VerificationMode {
	return []model.VerificationMode{model.VerificationModeOff, model.VerificationModePermissive, model.VerificationModeStrict}
}

func testConceptDescriptionVerificationFixtures(t *testing.T, db *sql.DB, router chi.Router, service string, mode model.VerificationMode) {
	t.Helper()
	for _, fixture := range conceptDescriptionVerificationFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			for _, operation := range conceptDescriptionVerificationOperations(service) {
				t.Run(operation.name, func(t *testing.T) {
					testConceptDescriptionVerificationWrite(t, db, router, mode, fixture, operation)
				})
			}
		})
	}
}

func conceptDescriptionVerificationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", conceptDescriptionRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	previousHistory := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeAPI})
	t.Cleanup(func() { history.Configure(previousHistory) })
	return db
}

func conceptDescriptionVerificationRouter(t *testing.T, db *sql.DB, service string, mode model.VerificationMode) chi.Router {
	t.Helper()
	backend, err := cddb.NewConceptDescriptionBackendFromDB(db, string(mode))
	require.NoError(t, err)
	base := cdapi.NewConceptDescriptionRepositoryAPIAPIService(backend)
	var servicer cdopenapi.ConceptDescriptionRepositoryAPIAPIServicer = base
	router := chi.NewRouter()
	cfg := &common.Config{}
	cfg.Server.StrictVerification = string(mode)
	router.Use(common.ConfigMiddleware(cfg))
	if service == "environment" {
		persistence := conceptDescriptionVerificationEnvironment(t, db, backend, mode)
		servicer = aasenvironment.NewCustomConceptDescriptionRepositoryService(base, persistence)
		aasenvironment.RegisterUploadAPI(router, aasenvironment.NewUploadAPIService(persistence, nil, nil), 1<<20, binarycontent.NewStager(db))
	}
	controller := cdopenapi.NewConceptDescriptionRepositoryAPIAPIController(servicer, "", string(mode))
	for _, route := range controller.OrderedRoutes() {
		router.MethodFunc(route.Method, route.Pattern, route.HandlerFunc)
	}
	return router
}

func conceptDescriptionVerificationEnvironment(t *testing.T, db *sql.DB, backend *cddb.ConceptDescriptionBackend, mode model.VerificationMode) *aasenvironment.Persistence {
	t.Helper()
	aasBackend, err := aasdb.NewAssetAdministrationShellDatabaseFromDB(db, string(mode))
	require.NoError(t, err)
	submodelBackend, err := smdb.NewSubmodelDatabaseFromDB(db, nil, string(mode))
	require.NoError(t, err)
	return &aasenvironment.Persistence{
		DB: db, ConceptDescriptionRepository: backend,
		AASRepository: aasBackend, SubmodelRepository: submodelBackend,
	}
}

type conceptDescriptionVerificationSnapshot struct {
	body        string
	persistence conceptDescriptionPersistenceState
	historyRows int
}

func testConceptDescriptionVerificationWrite(t *testing.T, db *sql.DB, router chi.Router, mode model.VerificationMode, fixture conceptDescriptionVerificationFixture, operation conceptDescriptionVerificationOperation) {
	t.Helper()
	id := "urn:example:cd:verification:" + uuid.NewString()
	path := "/concept-descriptions/" + common.EncodeString(id)
	t.Cleanup(func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, path, nil))
		require.Contains(t, []int{http.StatusNoContent, http.StatusNotFound}, response.Code)
	})
	before := seedConceptDescriptionVerificationReplacement(t, db, router, id, operation.replace)
	logs := captureConceptDescriptionVerificationLogs(t)
	payload := fixture.payload(id)
	response := writeConceptDescriptionVerification(t, router, id, payload, operation)
	rejected := fixture.violation != "" && mode == model.VerificationModeStrict
	assert.Equal(t, conceptDescriptionVerificationStatus(operation, rejected), response.Code, "response=%s", response.Body.String())
	assertConceptDescriptionVerificationLogs(t, logs.String(), mode, fixture)
	if rejected {
		assert.Contains(t, response.Body.String(), fixture.violation)
		assertConceptDescriptionVerificationUnchanged(t, db, router, id, before, operation.replace)
		return
	}
	read := serveConceptDescriptionVerificationJSON(t, router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, read.Code, "response=%s", read.Body.String())
	expected, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), read.Body.String())
}

func seedConceptDescriptionVerificationReplacement(t *testing.T, db *sql.DB, router chi.Router, id string, replace bool) conceptDescriptionVerificationSnapshot {
	t.Helper()
	if !replace {
		return conceptDescriptionVerificationSnapshot{}
	}
	payload := conceptDescriptionUpdatePayload(id, "Original_01", "2030-01-02T03:04:06Z", true)
	response := serveConceptDescriptionVerificationJSON(t, router, http.MethodPost, "/concept-descriptions", payload)
	require.Equal(t, http.StatusCreated, response.Code, "response=%s", response.Body.String())
	read := serveConceptDescriptionVerificationJSON(t, router, http.MethodGet, "/concept-descriptions/"+common.EncodeString(id), nil)
	require.Equal(t, http.StatusOK, read.Code)
	return conceptDescriptionVerificationSnapshot{
		body: read.Body.String(), persistence: readConceptDescriptionPersistenceState(t, db, id),
		historyRows: countConceptDescriptionVerificationRows(t, db, "concept_description_history", "identifier", id),
	}
}

func conceptDescriptionVerificationStatus(operation conceptDescriptionVerificationOperation, rejected bool) int {
	if rejected {
		return http.StatusBadRequest
	}
	if operation.upload {
		return http.StatusOK
	}
	if operation.replace {
		return http.StatusNoContent
	}
	return http.StatusCreated
}

func writeConceptDescriptionVerification(t *testing.T, router chi.Router, id string, payload map[string]any, operation conceptDescriptionVerificationOperation) *httptest.ResponseRecorder {
	t.Helper()
	if operation.upload {
		return uploadConceptDescriptionVerification(t, router, payload)
	}
	path := "/concept-descriptions"
	if operation.method == http.MethodPut {
		path += "/" + common.EncodeString(id)
	}
	return serveConceptDescriptionVerificationJSON(t, router, operation.method, path, payload)
}

func serveConceptDescriptionVerificationJSON(t *testing.T, router http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		require.NoError(t, err)
	}
	request := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func uploadConceptDescriptionVerification(t *testing.T, router http.Handler, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	environment, err := json.Marshal(map[string]any{"conceptDescriptions": []any{payload}})
	require.NoError(t, err)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "environment.json")
	require.NoError(t, err)
	_, err = file.Write(environment)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func captureConceptDescriptionVerificationLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	previousLogger := slog.Default()
	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	return &logs
}

func assertConceptDescriptionVerificationLogs(t *testing.T, logs string, mode model.VerificationMode, fixture conceptDescriptionVerificationFixture) {
	t.Helper()
	if mode == model.VerificationModePermissive && fixture.violation != "" {
		assert.Contains(t, logs, "semantic verification warning")
		assert.Contains(t, logs, "MODEL-VALIDATEWITHMODE-WARN")
		assert.Contains(t, logs, fixture.violation)
		return
	}
	assert.NotContains(t, logs, "semantic verification warning")
}

func assertConceptDescriptionVerificationUnchanged(t *testing.T, db *sql.DB, router http.Handler, id string, before conceptDescriptionVerificationSnapshot, replace bool) {
	t.Helper()
	read := serveConceptDescriptionVerificationJSON(t, router, http.MethodGet, "/concept-descriptions/"+common.EncodeString(id), nil)
	if replace {
		assert.Equal(t, http.StatusOK, read.Code)
		assert.JSONEq(t, before.body, read.Body.String())
		assert.Equal(t, before.persistence, readConceptDescriptionPersistenceState(t, db, id))
	} else {
		assert.Equal(t, http.StatusNotFound, read.Code, "response=%s", read.Body.String())
		assert.Zero(t, countConceptDescriptionVerificationRows(t, db, "concept_description", "id", id))
	}
	assert.Equal(t, before.historyRows, countConceptDescriptionVerificationRows(t, db, "concept_description_history", "identifier", id))
}

func countConceptDescriptionVerificationRows(t *testing.T, db *sql.DB, table, column, id string) int {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From(table).Select(goqu.COUNT("*")).
		Where(goqu.C(column).Eq(id)).Prepared(true).ToSQL()
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count
}

func TestConceptDescriptionVerificationPreservesRequestChecks(t *testing.T) {
	db := conceptDescriptionVerificationDB(t)
	for _, service := range []string{"standalone", "environment"} {
		for _, mode := range conceptDescriptionVerificationModes() {
			t.Run(service+"/"+string(mode), func(t *testing.T) {
				router := conceptDescriptionVerificationRouter(t, db, service, mode)
				testConceptDescriptionVerificationParsing(t, router)
				testConceptDescriptionVerificationIdentifiers(t, db, router)
				testConceptDescriptionVerificationAuthorization(t, db, router)
			})
		}
	}
}

func testConceptDescriptionVerificationParsing(t *testing.T, router http.Handler) {
	t.Helper()
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		path := "/concept-descriptions"
		if method == http.MethodPut {
			path += "/" + common.EncodeString("urn:example:cd:malformed-json")
		}
		request := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewBufferString("{"))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code, "response=%s", response.Body.String())
	}
}

func testConceptDescriptionVerificationIdentifiers(t *testing.T, db *sql.DB, router http.Handler) {
	t.Helper()
	id := "urn:example:cd:verification:" + uuid.NewString()
	payload := map[string]any{"modelType": "ConceptDescription", "id": id, "idShort": "Valid_01"}
	pathID := id + ":different"
	response := serveConceptDescriptionVerificationJSON(t, router, http.MethodPut, "/concept-descriptions/"+common.EncodeString(pathID), payload)
	assert.Equal(t, http.StatusBadRequest, response.Code, "response=%s", response.Body.String())
	assert.Zero(t, countConceptDescriptionVerificationRows(t, db, "concept_description", "id", id))
	assert.Zero(t, countConceptDescriptionVerificationRows(t, db, "concept_description", "id", pathID))
}

func testConceptDescriptionVerificationAuthorization(t *testing.T, db *sql.DB, router http.Handler) {
	t.Helper()
	id := "urn:example:cd:verification:" + uuid.NewString()
	payload, err := json.Marshal(map[string]any{"modelType": "ConceptDescription", "id": id, "idShort": "Valid_01"})
	require.NoError(t, err)
	denied := false
	formula := grammar.LogicalExpression{Boolean: &denied}
	ctx := auth.WithQueryFilter(t.Context(), &auth.QueryFilter{
		Formula:         &formula,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{grammar.RightsEnumCREATE: formula},
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/concept-descriptions", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusForbidden, response.Code, "response=%s", response.Body.String())
	assert.Zero(t, countConceptDescriptionVerificationRows(t, db, "concept_description", "id", id))
}
