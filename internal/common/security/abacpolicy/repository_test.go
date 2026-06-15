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

package abacpolicy

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/go-chi/chi/v5"
)

func TestRefreshActiveModelFailsClosedWithoutActivePolicy(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock setup failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	repo, err := NewRepository(db, "test-service", chi.NewRouter(), "")
	if err != nil {
		t.Fatalf("repository setup failed: %v", err)
	}
	mock.ExpectQuery(`SELECT (.+) FROM "abac_policy_versions"`).
		WillReturnRows(sqlmock.NewRows([]string{"version_id"}))

	err = repo.RefreshActiveModel(t.Context())
	if !common.IsErrServiceUnavailable(err) {
		t.Fatalf("expected service unavailable fail-closed error, got %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRefreshActiveModelClearsStaleCacheOnFailure(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock setup failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	repo, err := NewRepository(db, "test-service", chi.NewRouter(), "")
	if err != nil {
		t.Fatalf("repository setup failed: %v", err)
	}
	repo.publishActivePolicy(activePolicy{model: &auth.AccessModel{}})
	mock.ExpectQuery(`SELECT (.+) FROM "abac_policy_versions"`).
		WillReturnError(errors.New("database unavailable"))

	err = repo.RefreshActiveModel(t.Context())
	if err == nil {
		t.Fatal("expected refresh error")
	}
	if repo.ActiveAccessModel() != nil {
		t.Fatal("expected stale active model to be cleared")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestPublishActivePolicyAfterFailClosedClearRestoresCache(t *testing.T) {
	t.Parallel()

	repo := &Repository{}
	repo.clearActiveModel()
	if repo.ActiveAccessModel() != nil {
		t.Fatal("expected cleared cache to read as nil")
	}
	model := &auth.AccessModel{}
	repo.publishActivePolicy(activePolicy{model: model})
	if repo.ActiveAccessModel() != model {
		t.Fatal("expected published active model after cache clear")
	}
}

func TestResolvePolicyFileImportModeAppliesServiceDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		configured   string
		serviceScope string
		want         string
	}{
		{
			name:         "dtr default",
			serviceScope: "digitaltwinregistryservice",
			want:         common.ABACPolicyFileImportAlways,
		},
		{
			name:         "other service default",
			serviceScope: "submodelrepositoryservice",
			want:         common.ABACPolicyFileImportIfMissing,
		},
		{
			name:         "explicit never",
			configured:   "never",
			serviceScope: "digitaltwinregistryservice",
			want:         common.ABACPolicyFileImportNever,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolvePolicyFileImportMode(test.configured, test.serviceScope)
			if err != nil {
				t.Fatalf("resolve mode failed: %v", err)
			}
			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestResolvePolicyFileImportModeRejectsUnsupportedValue(t *testing.T) {
	t.Parallel()

	_, err := resolvePolicyFileImportMode("sometimes", "submodelrepositoryservice")
	if err == nil {
		t.Fatal("expected unsupported mode error")
	}
}

func TestManagementAPIAllowedRejectsDigitalTwinRegistry(t *testing.T) {
	t.Parallel()

	if ManagementAPIAllowed("digitaltwinregistryservice") {
		t.Fatal("expected Digital Twin Registry management API to be rejected")
	}
	if ManagementAPIAllowed(" DigitalTwinRegistryService ") {
		t.Fatal("expected Digital Twin Registry management API to be rejected with whitespace and case normalization")
	}
	if !ManagementAPIAllowed("submodelrepositoryservice") {
		t.Fatal("expected non-DTR service scope to allow opt-in management API")
	}
}

func TestRegisterManagementRoutesAfterMiddlewareDoesNotPanic(t *testing.T) {
	t.Parallel()

	cfg := managementAPIEnabledConfig()
	router := chi.NewRouter()
	router.Use(noopMiddleware)
	guard := history.NewMutationCoverageGuard(router)
	router.Use(guard.Middleware)
	router.Use(noopMiddleware)

	assertNotPanics(t, func() {
		ExemptManagementMutationRoutesIfEnabled(cfg, guard, "aasenvironmentservice")
		RegisterManagementRoutesIfEnabled(cfg, router, &Repository{}, "aasenvironmentservice")
		router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	})
}

func TestRegisterManagementRoutesServesActivePolicyEndpoints(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock setup failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	repo, err := NewRepository(db, "test-service", chi.NewRouter(), "")
	if err != nil {
		t.Fatalf("repository setup failed: %v", err)
	}
	router := chi.NewRouter()
	RegisterManagementRoutes(router, repo)

	active := testPolicyVersion(3, StatusActive, testMaterializedPolicy(t))
	mock.ExpectQuery(`SELECT (.+) FROM "abac_policy_versions"`).
		WillReturnRows(policyVersionRows(active))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/security/abac/active-policy", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected active policy status 200, got %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"active"`) {
		t.Fatalf("expected active policy response, got %s", response.Body.String())
	}

	mock.ExpectQuery(`SELECT (.+) FROM "abac_policy_versions"`).
		WillReturnRows(policyVersionRows(active))
	mock.ExpectQuery(`SELECT (.+) FROM "abac_policy_rules"`).
		WillReturnRows(policyRuleRows())

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/security/abac/active-policy/rules", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected active rules status 200, got %d body=%s", response.Code, response.Body.String())
	}
	if strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("expected empty active rules list, got %s", response.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestManagementMutationRoutesAreHistoryExempt(t *testing.T) {
	cfg := managementAPIEnabledConfig()
	previous := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeAPI, Immutability: history.ImmutabilityNone})
	defer history.Configure(previous)

	router := chi.NewRouter()
	guard := history.NewMutationCoverageGuard(router)
	router.Use(guard.Middleware)
	ExemptManagementMutationRoutesIfEnabled(cfg, guard, "aasenvironmentservice")
	router.Post("/security/abac/policy-versions/{versionID}/validate", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/security/abac/policy-versions/1/validate", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected exempt management mutation to reach handler, got status %d", response.Code)
	}
}

func TestMergeJSONObjectsRemovesNullFieldsRecursively(t *testing.T) {
	t.Parallel()

	merged := mergeJSONObjects(
		map[string]any{
			"ACL": map[string]any{
				"ACCESS":        "ALLOW",
				"USEATTRIBUTES": "admin",
			},
			"USEFORMULA": "is_admin",
		},
		map[string]any{
			"ACL": map[string]any{
				"USEATTRIBUTES": nil,
				"ATTRIBUTES":    []any{map[string]any{"GLOBAL": "ANONYMOUS"}},
			},
			"USEFORMULA": nil,
		},
	)

	acl := merged["ACL"].(map[string]any)
	if _, ok := acl["USEATTRIBUTES"]; ok {
		t.Fatalf("expected nested null field to be removed: %#v", acl)
	}
	if _, ok := merged["USEFORMULA"]; ok {
		t.Fatalf("expected top-level null field to be removed: %#v", merged)
	}
	if _, ok := acl["ATTRIBUTES"]; !ok {
		t.Fatalf("expected replacement field to be present: %#v", acl)
	}
}

func TestDecodeRuleMutationAcceptsDirectRuleBody(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/security/abac/policy-versions/1/rules", stringsReader(`{
		"ACL":{"ACCESS":"ALLOW","RIGHTS":["READ"],"ATTRIBUTES":[{"GLOBAL":"ANONYMOUS"}]},
		"OBJECTS":[{"ROUTE":"/description"}],
		"FORMULA":{"$boolean":true}
	}`))

	mutation, err := decodeRuleMutation(req)
	if err != nil {
		t.Fatalf("decode mutation failed: %v", err)
	}
	if len(mutation.Rule) == 0 {
		t.Fatalf("expected direct rule body to be preserved")
	}
}

func TestReadBodyRejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/security/abac/policy-versions", stringsReader(strings.Repeat("x", maxManagementRequestBodyBytes+1)))

	_, err := readBody(req)
	if !common.IsErrBadRequest(err) {
		t.Fatalf("expected bad request for oversized body, got %v", err)
	}
	if !strings.Contains(err.Error(), "ABACPOLICY-API-BODYTOOLARGE") {
		t.Fatalf("expected body-too-large error code, got %v", err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(maxManagementRequestBodyBytes)) {
		t.Fatalf("expected body-too-large error to include configured byte limit, got %v", err)
	}
}

func TestDuplicateRuleRejectsMalformedOptionalBody(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	router.Post("/security/abac/policy-versions/{versionID}/rules/{ruleIndex}/duplicate", duplicateRuleHandler(&Repository{}))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/security/abac/policy-versions/1/rules/1/duplicate", stringsReader(`{`))

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed optional body to be rejected, got status %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "ABACPOLICY-API-DECODE") {
		t.Fatalf("expected decode error response, got %s", response.Body.String())
	}
}

func TestDefinitionCRUDMutatesConfiguredPolicy(t *testing.T) {
	t.Parallel()

	policy, err := decodeConfiguredPolicy(testPolicyRaw(t))
	if err != nil {
		t.Fatalf("decode policy failed: %v", err)
	}
	rawDefinition := json.RawMessage(`{"name":"ownerClaims","attributes":[{"CLAIM":"owner"}]}`)
	if err = createDefinitionInPolicy(&policy, definitionKindAttributes, rawDefinition); err != nil {
		t.Fatalf("create definition failed: %v", err)
	}
	definitions := definitionsFromPolicy(policy)
	if len(definitions.Attributes) != 1 || definitions.Attributes[0].Name != "ownerClaims" {
		t.Fatalf("expected created attribute definition, got %#v", definitions.Attributes)
	}
	if definitions.Attributes[0].Attributes[0].Value != "owner" {
		t.Fatalf("expected created claim attribute, got %#v", definitions.Attributes[0].Attributes[0])
	}

	err = createDefinitionInPolicy(&policy, definitionKindAttributes, rawDefinition)
	if !common.IsErrConflict(err) {
		t.Fatalf("expected duplicate conflict, got %v", err)
	}

	err = patchDefinitionInPolicy(&policy, definitionKindAttributes, "ownerClaims", json.RawMessage(`{"attributes":[{"CLAIM":"tenant"}]}`))
	if err != nil {
		t.Fatalf("patch definition failed: %v", err)
	}
	definitions = definitionsFromPolicy(policy)
	if definitions.Attributes[0].Attributes[0].Value != "tenant" {
		t.Fatalf("expected patched claim attribute, got %#v", definitions.Attributes[0].Attributes[0])
	}

	err = replaceDefinitionInPolicy(&policy, definitionKindAttributes, "ownerClaims", json.RawMessage(`{"name":"other","attributes":[{"CLAIM":"owner"}]}`))
	if !common.IsErrBadRequest(err) {
		t.Fatalf("expected path/body name mismatch to be rejected, got %v", err)
	}

	if err = deleteDefinitionFromPolicy(&policy, definitionKindAttributes, "ownerClaims"); err != nil {
		t.Fatalf("delete definition failed: %v", err)
	}
	if len(definitionsFromPolicy(policy).Attributes) != 0 {
		t.Fatalf("expected definition to be deleted, got %#v", definitionsFromPolicy(policy).Attributes)
	}
}

func TestDeleteReferencedDefinitionBreaksMaterialization(t *testing.T) {
	t.Parallel()

	policy, err := decodeConfiguredPolicy(testPolicyWithReferencedFormulaRaw())
	if err != nil {
		t.Fatalf("decode policy failed: %v", err)
	}
	if err = deleteDefinitionFromPolicy(&policy, definitionKindFormulas, "always"); err != nil {
		t.Fatalf("delete formula definition failed: %v", err)
	}
	raw, err := common.CanonicalJSON(policy)
	if err != nil {
		t.Fatalf("canonical policy failed: %v", err)
	}
	router := chi.NewRouter()
	router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	_, err = auth.MaterializeABACPolicy(raw, router, "")
	if err == nil {
		t.Fatal("expected materialization to reject missing referenced formula")
	}
}

func TestDefinitionEndpointsValidateKindBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	RegisterManagementRoutes(router, &Repository{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/security/abac/policy-versions/1/definitions/nope", nil)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported definition kind to return 400, got %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "ABACPOLICY-DEFINITION-KIND") {
		t.Fatalf("expected definition kind error, got %s", response.Body.String())
	}
}

func TestListDefinitionsEndpointReturnsConfiguredDefinitions(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock setup failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	repo, err := NewRepository(db, "test-service", chi.NewRouter(), "")
	if err != nil {
		t.Fatalf("repository setup failed: %v", err)
	}
	router := chi.NewRouter()
	RegisterManagementRoutes(router, repo)

	materialized := testMaterializedPolicyFromRaw(t, testPolicyWithDefinitionsRaw())
	version := testPolicyVersion(7, StatusStaged, materialized)
	mock.ExpectQuery(`SELECT (.+) FROM "abac_policy_versions"`).
		WillReturnRows(policyVersionRows(version))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/security/abac/policy-versions/7/definitions", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected definitions status 200, got %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"attributes"`) || !strings.Contains(response.Body.String(), `"adminClaims"`) {
		t.Fatalf("expected configured definitions in response, got %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"rules"`) {
		t.Fatalf("expected definitions response to omit rules, got %s", response.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestCreateDefinitionRejectsImmutablePolicyVersion(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock setup failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	repo, err := NewRepository(db, "test-service", chi.NewRouter(), "")
	if err != nil {
		t.Fatalf("repository setup failed: %v", err)
	}
	active := testPolicyVersion(5, StatusActive, testMaterializedPolicy(t))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT (.+) FROM "abac_policy_versions"`).
		WillReturnRows(policyVersionRows(active))
	mock.ExpectRollback()

	_, err = repo.CreateDefinition(t.Context(), 5, "attributes", json.RawMessage(`{"name":"ownerClaims","attributes":[{"CLAIM":"owner"}]}`))
	if !common.IsErrConflict(err) {
		t.Fatalf("expected immutable version conflict, got %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestImportPolicyAndActivateRollsBackImportedVersionOnActivationFailure(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock setup failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	router := chi.NewRouter()
	router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	repo, err := NewRepository(db, "test-service", router, "")
	if err != nil {
		t.Fatalf("repository setup failed: %v", err)
	}
	raw := testPolicyRaw(t)
	materialized := testMaterializedPolicy(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "abac_policy_versions"`).
		WillReturnRows(sqlmock.NewRows([]string{"version_id"}).AddRow(11))
	mock.ExpectExec(`INSERT INTO "abac_policy_rules"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "abac_policy_events"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT (.+) FROM "abac_policy_versions"`).
		WillReturnRows(policyVersionRows(testPolicyVersion(11, StatusStaged, materialized)))
	mock.ExpectQuery(`SELECT (.+) FROM "abac_policy_versions"`).
		WillReturnRows(policyVersionRows(testPolicyVersion(11, StatusActive, materialized)))
	mock.ExpectRollback()

	_, err = repo.ImportPolicyAndActivate(t.Context(), raw, "test-import")
	if !common.IsErrConflict(err) {
		t.Fatalf("expected activation conflict, got %v", err)
	}
	if repo.ActiveAccessModel() != nil {
		t.Fatal("expected failed atomic import+activate to leave runtime cache unchanged")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestBuildActivationEvidenceArtifactIncludesActivationMetadata(t *testing.T) {
	t.Parallel()

	activatedAt := time.Date(2026, 6, 10, 10, 30, 0, 0, time.UTC)
	version := PolicyVersion{
		VersionID:              17,
		ServiceScope:           "aasenvironmentservice",
		PolicyID:               strings.Repeat("a", 64),
		Status:                 StatusActive,
		SourceType:             SourceTypeAPI,
		ConfiguredPolicyJSON:   json.RawMessage(`{"AllAccessPermissionRules":{"rules":[]}}`),
		ConfiguredPolicyHash:   strings.Repeat("b", 64),
		MaterializedPolicyJSON: json.RawMessage(`{"rules":[]}`),
		MaterializedPolicyHash: strings.Repeat("c", 64),
		CreatedAt:              activatedAt.Add(-time.Hour),
		ActivatedAt:            &activatedAt,
		ActivatedBySubject:     "admin",
		ActivatedByIssuer:      "issuer",
		ActivatedByClientID:    "client",
	}

	artifact, err := buildActivationEvidenceArtifact(version, nil, auditActor{Subject: "admin"})
	if err != nil {
		t.Fatalf("build evidence artifact failed: %v", err)
	}
	var doc map[string]any
	if err = json.Unmarshal(artifact.Data, &doc); err != nil {
		t.Fatalf("decode evidence artifact failed: %v", err)
	}

	if doc["status"] != StatusActive {
		t.Fatalf("expected active status in evidence artifact, got %#v", doc["status"])
	}
	if doc["activated_at"] != activatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("expected activated_at %s, got %#v", activatedAt.Format(time.RFC3339Nano), doc["activated_at"])
	}
	if doc["activated_by_subject"] != "admin" {
		t.Fatalf("expected activation subject in evidence artifact, got %#v", doc["activated_by_subject"])
	}
}

func stringsReader(value string) *strings.Reader {
	return strings.NewReader(value)
}

func managementAPIEnabledConfig() *common.Config {
	return &common.Config{
		ABAC: common.ABACConfig{
			Enabled: true,
			ManagementAPI: common.ABACManagementAPIConfig{
				Enabled: true,
			},
		},
	}
}

func noopMiddleware(next http.Handler) http.Handler {
	return next
}

func assertNotPanics(t *testing.T, run func()) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("unexpected panic: %v", recovered)
		}
	}()
	run()
}

func testPolicyRaw(t *testing.T) []byte {
	t.Helper()
	return []byte(`{
		"AllAccessPermissionRules": {
			"rules": [
				{
					"ACL": {
						"ACCESS": "ALLOW",
						"RIGHTS": ["READ"],
						"ATTRIBUTES": [{ "GLOBAL": "ANONYMOUS" }]
					},
					"OBJECTS": [{ "ROUTE": "/description" }],
					"FORMULA": { "$boolean": true }
				}
			]
		}
	}`)
}

func testPolicyWithDefinitionsRaw() []byte {
	return []byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [
				{
					"name": "adminClaims",
					"attributes": [{ "CLAIM": "role" }]
				}
			],
			"DEFFORMULAS": [
				{
					"name": "always",
					"formula": { "$boolean": true }
				}
			],
			"rules": [
				{
					"ACL": {
						"ACCESS": "ALLOW",
						"RIGHTS": ["READ"],
						"ATTRIBUTES": [{ "GLOBAL": "ANONYMOUS" }]
					},
					"OBJECTS": [{ "ROUTE": "/description" }],
					"FORMULA": { "$boolean": true }
				}
			]
		}
	}`)
}

func testPolicyWithReferencedFormulaRaw() []byte {
	return []byte(`{
		"AllAccessPermissionRules": {
			"DEFFORMULAS": [
				{
					"name": "always",
					"formula": { "$boolean": true }
				}
			],
			"rules": [
				{
					"ACL": {
						"ACCESS": "ALLOW",
						"RIGHTS": ["READ"],
						"ATTRIBUTES": [{ "GLOBAL": "ANONYMOUS" }]
					},
					"OBJECTS": [{ "ROUTE": "/description" }],
					"USEFORMULA": "always"
				}
			]
		}
	}`)
}

func testMaterializedPolicy(t *testing.T) auth.MaterializedABACPolicy {
	t.Helper()
	return testMaterializedPolicyFromRaw(t, testPolicyRaw(t))
}

func testMaterializedPolicyFromRaw(t *testing.T, raw []byte) auth.MaterializedABACPolicy {
	t.Helper()
	router := chi.NewRouter()
	router.Get("/description", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	materialized, err := auth.MaterializeABACPolicy(raw, router, "")
	if err != nil {
		t.Fatalf("materialize test policy failed: %v", err)
	}
	return materialized
}

func testPolicyVersion(versionID int64, status string, materialized auth.MaterializedABACPolicy) PolicyVersion {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	return PolicyVersion{
		VersionID:              versionID,
		ServiceScope:           "test-service",
		PolicyID:               materialized.PolicyID,
		Status:                 status,
		SourceType:             SourceTypeAPI,
		SourceRef:              "test-import",
		ConfiguredPolicyJSON:   json.RawMessage(materialized.ConfiguredPolicyJSON),
		ConfiguredPolicyHash:   materialized.ConfiguredPolicyHash,
		RawPolicyHash:          materialized.RawPolicyHash,
		MaterializedPolicyJSON: json.RawMessage(materialized.MaterializedPolicyJSON),
		MaterializedPolicyHash: materialized.MaterializedPolicyHash,
		CreatedAt:              now,
	}
}

func policyVersionRows(version PolicyVersion) *sqlmock.Rows {
	return sqlmock.NewRows(policyVersionColumnNames()).AddRow(
		version.VersionID,
		version.ServiceScope,
		version.PolicyID,
		version.Status,
		version.SourceType,
		nullableRowString(version.SourceRef),
		[]byte(version.ConfiguredPolicyJSON),
		version.ConfiguredPolicyHash,
		nullableRowString(version.RawPolicyHash),
		[]byte(version.MaterializedPolicyJSON),
		version.MaterializedPolicyHash,
		version.CreatedAt,
		nullableRowString(version.CreatedBySubject),
		nullableRowString(version.CreatedByIssuer),
		nullableRowString(version.CreatedByClientID),
		nullableRowTime(version.UpdatedAt),
		nullableRowString(version.UpdatedBySubject),
		nullableRowString(version.UpdatedByIssuer),
		nullableRowString(version.UpdatedByClientID),
		nullableRowTime(version.ActivatedAt),
		nullableRowString(version.ActivatedBySubject),
		nullableRowString(version.ActivatedByIssuer),
		nullableRowString(version.ActivatedByClientID),
		nullableRowTime(version.SupersededAt),
		nullableJSONRowString(version.ArtifactRef),
	)
}

func policyVersionColumnNames() []string {
	return []string{
		"version_id",
		"service_scope",
		"policy_id",
		"status",
		"source_type",
		"source_ref",
		"configured_policy_json",
		"configured_policy_hash",
		"raw_policy_hash",
		"materialized_policy_json",
		"materialized_policy_hash",
		"created_at",
		"created_by_subject",
		"created_by_issuer",
		"created_by_client_id",
		"updated_at",
		"updated_by_subject",
		"updated_by_issuer",
		"updated_by_client_id",
		"activated_at",
		"activated_by_subject",
		"activated_by_issuer",
		"activated_by_client_id",
		"superseded_at",
		"artifact_ref",
	}
}

func policyRuleRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"rule_id",
		"version_id",
		"policy_id",
		"service_scope",
		"rule_index",
		"matched_rule_id",
		"configured_rule_json",
		"materialized_rule_json",
		"acl_json",
		"attributes_json",
		"objects_json",
		"formula_json",
		"filters_json",
		"access",
		"rights",
		"rule_hash",
		"materialized_rule_hash",
		"created_at",
		"created_by_subject",
		"created_by_issuer",
		"created_by_client_id",
	})
}

func nullableRowString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableJSONRowString(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func nullableRowTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
