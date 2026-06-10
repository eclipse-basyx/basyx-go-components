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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
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
