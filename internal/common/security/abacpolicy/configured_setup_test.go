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
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

func TestSetupConfiguredSecurityLegacyModeUsesUnchangedABACSetup(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock setup failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	policyScope := "legacy-abac-proof"
	active := testPolicyVersion(11, StatusActive, testMaterializedPolicy(t))
	active.ServiceScope = policyScope
	cfg := setupPolicyScopeConfig(t, policyScope)
	router := chi.NewRouter()

	mock.ExpectQuery(`FROM "abac_policy_versions".*"service_scope" = 'legacy-abac-proof'.*"status" = 'active'`).
		WillReturnRows(policyVersionRows(active))
	mock.ExpectQuery(`FROM "abac_policy_rules".*"service_scope" = 'legacy-abac-proof'.*"version_id" = 11`).
		WillReturnRows(policyRuleRows())

	repo, err := SetupConfiguredSecurity(t.Context(), cfg, router, db, "aasregistryservice")
	if err != nil {
		t.Fatalf("setup configured legacy ABAC failed: %v", err)
	}
	if repo == nil || repo.serviceScope != policyScope {
		t.Fatalf("expected legacy ABAC repository scope %q", policyScope)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
