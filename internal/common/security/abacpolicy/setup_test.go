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
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/go-chi/chi/v5"
)

func TestSetupSecurityWithABACRepositoryUsesConfiguredPolicyScope(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock setup failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	policyScope := "aasregistry-internal"
	active := testPolicyVersion(7, StatusActive, testMaterializedPolicy(t))
	active.ServiceScope = policyScope
	cfg := setupPolicyScopeConfig(t, policyScope)
	router := chi.NewRouter()

	mock.ExpectQuery(`FROM "abac_policy_versions".*"service_scope" = 'aasregistry-internal'.*"status" = 'active'`).
		WillReturnRows(policyVersionRows(active))
	mock.ExpectQuery(`FROM "abac_policy_rules".*"service_scope" = 'aasregistry-internal'.*"version_id" = 7`).
		WillReturnRows(policyRuleRows())

	repo, err := SetupSecurityWithABACRepository(t.Context(), cfg, router, db, "aasregistryservice")
	if err != nil {
		t.Fatalf("setup ABAC repository failed: %v", err)
	}
	if repo.serviceScope != policyScope {
		t.Fatalf("expected repository scope %q, got %q", policyScope, repo.serviceScope)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestManagementRoutesEnabledAllowsExplicitDigitalTwinRegistryOptIn(t *testing.T) {
	t.Parallel()

	cfg := managementAPIEnabledConfig()
	cfg.ABAC.PolicyScope = "shared-public-policy"

	if !ManagementRoutesEnabled(cfg, "digitaltwinregistryservice") {
		t.Fatal("expected Digital Twin Registry management routes to be enabled by explicit opt-in")
	}
	if !ManagementRoutesEnabled(cfg, "aasregistryservice") {
		t.Fatal("expected non-DTR management routes to be enabled by explicit opt-in")
	}
}

func setupPolicyScopeConfig(t *testing.T, policyScope string) *common.Config {
	t.Helper()
	trustlistPath := t.TempDir() + "/trustlist.json"
	if err := os.WriteFile(trustlistPath, []byte(`[]`), 0o600); err != nil {
		t.Fatalf("write trustlist: %v", err)
	}
	return &common.Config{
		General: common.GeneralConfig{
			EnableImplicitCasts: true,
		},
		OIDC: common.OIDCConfig{
			TrustlistPath: trustlistPath,
		},
		ABAC: common.ABACConfig{
			Enabled:          true,
			PolicyFileImport: common.ABACPolicyFileImportNever,
			PolicyScope:      policyScope,
		},
	}
}
