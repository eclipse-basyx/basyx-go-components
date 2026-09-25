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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebacintegration

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func TestInvitationsCreateNormalGrantsForTheRedeemer(t *testing.T) {
	bootstrapCreators(t)
	identifier := unique("cd")
	expectStatus(t, http.StatusCreated, call(t, "alice", http.MethodPost, cdURL+"/concept-descriptions", conceptDescription(identifier), nil), "create CD")
	target := cdURL + "/concept-descriptions/" + enc(identifier)
	invitations := target + "/$access/invitations"
	accept := cdURL + "/security/rebac/invitations/accept"

	expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	expectStatus(t, http.StatusBadRequest, call(t, "alice", http.MethodPost, invitations, map[string]any{"relation": "owner", "expiresAt": expiry}, nil), "owner invitations are not allowed")
	expectStatus(t, http.StatusBadRequest, call(t, "alice", http.MethodPost, invitations, map[string]any{"relation": "viewer"}, nil), "expiry is required")
	expectStatus(t, http.StatusBadRequest, call(t, "alice", http.MethodPost, invitations, map[string]any{"relation": "executor", "expiresAt": expiry}, nil), "no executor on CDs")
	expectStatus(t, http.StatusNotFound, call(t, "bob", http.MethodPost, invitations, map[string]any{"relation": "viewer", "expiresAt": expiry}, nil), "only managers invite")

	created := call(t, "alice", http.MethodPost, invitations, map[string]any{"relation": "viewer", "expiresAt": expiry}, nil)
	expectStatus(t, http.StatusCreated, created, "create invitation")
	invitationToken, _ := created.json(t)["token"].(string)
	require.NotEmpty(t, invitationToken)

	listed := call(t, "alice", http.MethodGet, invitations, nil, nil)
	expectStatus(t, http.StatusOK, listed, "list invitations")
	require.NotContains(t, string(listed.body), invitationToken, "tokens are only visible once")

	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodGet, target, nil, nil), "before redemption")
	expectStatus(t, http.StatusNotFound, call(t, "", http.MethodPost, accept, map[string]any{"token": invitationToken}, nil), "anonymous redemption")
	expectStatus(t, http.StatusOK, call(t, "eve", http.MethodPost, accept, map[string]any{"token": invitationToken}, nil), "redeem")
	expectStatus(t, http.StatusOK, call(t, "eve", http.MethodGet, target, nil, nil), "redeemer reads")
	expectStatus(t, http.StatusNotFound, call(t, "bob", http.MethodPost, accept, map[string]any{"token": invitationToken}, nil), "maxUses defaults to one")

	personal := call(t, "alice", http.MethodPost, invitations, map[string]any{"relation": "viewer", "expiresAt": expiry,
		"expectedPrincipal": map[string]any{"issuer": issuer, "subject": subject(t, "carol")}}, nil)
	expectStatus(t, http.StatusCreated, personal, "invitation for one recipient")
	personalToken, _ := personal.json(t)["token"].(string)
	expectStatus(t, http.StatusNotFound, call(t, "bob", http.MethodPost, accept, map[string]any{"token": personalToken}, nil), "foreign recipient")
	expectStatus(t, http.StatusOK, call(t, "carol", http.MethodPost, accept, map[string]any{"token": personalToken}, nil), "expected recipient")
	require.Equal(t, "no-store", personal.header.Get("Cache-Control"), "tokens are never cached")

	second := call(t, "alice", http.MethodPost, invitations, map[string]any{"relation": "editor", "expiresAt": expiry, "maxUses": 5}, nil)
	expectStatus(t, http.StatusCreated, second, "second invitation")
	secondID, _ := second.json(t)["id"].(string)
	expectStatus(t, http.StatusNoContent, call(t, "alice", http.MethodDelete, invitations+"/"+secondID, nil, nil), "revoke invitation")
	secondToken, _ := second.json(t)["token"].(string)
	expectStatus(t, http.StatusNotFound, call(t, "bob", http.MethodPost, accept, map[string]any{"token": secondToken}, nil), "revoked invitations cannot be redeemed")
	expectStatus(t, http.StatusOK, call(t, "eve", http.MethodGet, target, nil, nil), "redeemed grants survive invitation revocation")
}

func TestAdministratorEndpointsAreHiddenFromOthers(t *testing.T) {
	bootstrapCreators(t)
	repository := submodelURL + "/security/rebac/repositories/submodel/$access"
	expectStatus(t, http.StatusOK, call(t, "dave", http.MethodGet, repository, nil, nil), "administrator reads repository grants")
	expectStatus(t, http.StatusNotFound, call(t, "alice", http.MethodGet, repository, nil, nil), "repository grants are hidden")
	expectStatus(t, http.StatusNotFound, call(t, "alice", http.MethodPost, submodelURL+"/security/rebac/admin/reconcile", nil, nil), "reconcile is hidden")
	reconciled := call(t, "dave", http.MethodPost, submodelURL+"/security/rebac/admin/reconcile", nil, nil)
	expectStatus(t, http.StatusOK, reconciled, "administrator reconcile")
	require.Equal(t, "no-store", reconciled.header.Get("Cache-Control"), "management responses are never cached")
}

func TestConcurrentGrantChangesAllowOneWriter(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "race")
	accessURL := submodelAccess(submodelURL, identifier)
	grants, etag := readAccess(t, "alice", accessURL)
	statuses := make([]int, 2)
	var wait sync.WaitGroup
	for index, user := range []string{"bob", "carol"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			body := map[string]any{"grants": append(append([]grant{}, grants...), userGrant(t, "viewer", user))}
			statuses[index] = call(t, "alice", http.MethodPut, accessURL+"/grants", body, map[string]string{"If-Match": etag}).status
		}()
	}
	wait.Wait()
	require.ElementsMatch(t, []int{http.StatusOK, http.StatusPreconditionFailed}, statuses)
}

func TestApprovedLinksRequireALiveReference(t *testing.T) {
	bootstrapCreators(t)
	submodelID := createSubmodel(t, environmentURL, "alice", "live-reference")
	shellID := unique("aas")
	expectStatus(t, http.StatusCreated, call(t, "alice", http.MethodPost, environmentURL+"/shells", shell(shellID, submodelID), nil), "create shell")
	addGrants(t, "alice", environmentURL+"/shells/"+enc(shellID)+"/$access", userGrant(t, "viewer", "eve"))
	accessURL := submodelAccess(environmentURL, submodelID)
	_, etag := readAccess(t, "alice", accessURL)
	expectStatus(t, http.StatusOK, call(t, "alice", http.MethodPut, accessURL+"/inheritance",
		map[string]any{"aasIds": []string{shellID}}, map[string]string{"If-Match": etag}), "approve link")
	target := environmentURL + "/submodels/" + enc(submodelID)
	expectStatus(t, http.StatusOK, call(t, "eve", http.MethodGet, target, nil, nil), "linked read")

	db := openDB(t)
	references := goqu.Dialect("postgres").From("aas_submodel_reference_key").Select("reference_id").
		Where(goqu.C("value").Eq(submodelID))
	removal, args, err := goqu.Dialect("postgres").Delete("aas_submodel_reference").
		Where(goqu.C("id").In(references)).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), removal, args...)
	require.NoError(t, err, "simulates a reference removed while ReBAC was disabled")
	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodGet, target, nil, nil), "a link without reference grants nothing")
}

func TestReenableReconciliationRemovesStateOfResourcesDeletedMeanwhile(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "orphan")
	addGrants(t, "alice", submodelAccess(submodelURL, identifier), userGrant(t, "viewer", "bob"))
	db := openDB(t)
	var authUUID string
	lookup, args, err := goqu.Dialect("postgres").From("submodel").Select(goqu.L("auth_uuid::text")).
		Where(goqu.C("submodel_identifier").Eq(identifier)).ToSQL()
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(t.Context(), lookup, args...).Scan(&authUUID))

	deleteRow, args, err := goqu.Dialect("postgres").Delete("submodel").Where(goqu.C("submodel_identifier").Eq(identifier)).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), deleteRow, args...)
	require.NoError(t, err, "simulates a deletion while ReBAC was disabled")

	compose(t, "restart", "submodel-repository")
	require.NoError(t, testenv.WaitHealthyURL(submodelURL+"/health", healthTimeout))

	count, args, err := goqu.Dialect("postgres").From("rebac_grant").Select(goqu.COUNT("*")).
		Where(goqu.L("object_uuid = ?::uuid", authUUID)).ToSQL()
	require.NoError(t, err)
	var remaining int
	require.NoError(t, db.QueryRowContext(t.Context(), count, args...).Scan(&remaining))
	require.Zero(t, remaining, "grants of resources deleted meanwhile are removed before readiness")

	expectStatus(t, http.StatusCreated, call(t, "admin", http.MethodPost, submodelURL+"/submodels", submodel(identifier, "recreated"), nil), "recreate identifier")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, submodelURL+"/submodels/"+enc(identifier), nil, nil), "a recreated resource never inherits old grants")
}

func TestGrantsAreSharedAcrossServicesOfOneScope(t *testing.T) {
	identifier := createSubmodel(t, environmentURL, "alice", "cross-service", property("p", "1"))
	addGrants(t, "alice", submodelAccess(submodelURL, identifier), userGrant(t, "viewer", "bob"))
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, environmentURL+"/submodels/"+enc(identifier), nil, nil), "AAS Environment")
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, submodelURL+"/submodels/"+enc(identifier), nil, nil), "Submodel repository")
	setGrants(t, "alice", submodelAccess(submodelURL, identifier), []grant{userGrant(t, "owner", "alice")})
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, environmentURL+"/submodels/"+enc(identifier), nil, nil),
		"revocations take effect at commit in every service")

	shellID := unique("aas")
	expectStatus(t, http.StatusCreated, call(t, "carol", http.MethodPost, aasURL+"/shells", shell(shellID), nil), "create shell in AAS repository")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, environmentURL+"/shells/"+enc(shellID), nil, nil), "ungranted shell")
	addGrants(t, "carol", aasURL+"/shells/"+enc(shellID)+"/$access", userGrant(t, "editor", "bob"))
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, environmentURL+"/shells/"+enc(shellID), nil, nil), "shell grant in environment")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodDelete, aasURL+"/shells/"+enc(shellID), nil, nil), "editors cannot delete")
	expectStatus(t, http.StatusNoContent, call(t, "carol", http.MethodDelete, aasURL+"/shells/"+enc(shellID), nil, nil), "owner deletes")
}

func TestEffectiveRightsReportSourcesWithoutRevealingExistence(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "effective")
	addGrants(t, "alice", submodelAccess(submodelURL, identifier), userGrant(t, "editor", "bob"))
	effective := call(t, "bob", http.MethodGet, submodelAccess(submodelURL, identifier)+"/effective", nil, nil)
	expectStatus(t, http.StatusOK, effective, "effective rights")
	sources := map[string]string{}
	for _, right := range effective.json(t)["rights"].([]any) {
		entry := right.(map[string]any)
		sources[entry["action"].(string)] = entry["source"].(string)
	}
	require.Equal(t, map[string]string{"read": "rebac", "update": "rebac", "delete": "none", "execute": "none", "manage": "none"}, sources)
	expectStatus(t, http.StatusNotFound, call(t, "eve", http.MethodGet, submodelAccess(submodelURL, identifier)+"/effective", nil, nil), "no rights, no existence")
	adminEffective := call(t, "admin", http.MethodGet, submodelAccess(submodelURL, identifier)+"/effective", nil, nil)
	expectStatus(t, http.StatusOK, adminEffective, "ABAC administrator")
	require.Contains(t, string(adminEffective.body), `"source":"abac"`)
}

func TestAccessChangesAreAuditedInAVerifiableChain(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "audited")
	addGrants(t, "alice", submodelAccess(submodelURL, identifier), userGrant(t, "viewer", "bob"))

	auditURL := submodelURL + "/security/rebac/admin/audit"
	expectStatus(t, http.StatusNotFound, call(t, "bob", http.MethodGet, auditURL, nil, nil), "the audit trail is for administrators only")
	listed := call(t, "dave", http.MethodGet, auditURL+"?limit=1000", nil, nil)
	expectStatus(t, http.StatusOK, listed, "list the audit trail")
	var page struct {
		Events []struct {
			Type    string          `json:"type"`
			Actor   string          `json:"actor"`
			Object  string          `json:"object"`
			Details json.RawMessage `json:"details"`
			Hash    string          `json:"hash"`
		} `json:"events"`
	}
	require.NoError(t, json.Unmarshal(listed.body, &page))
	found := false
	for _, event := range page.Events {
		if event.Type == "grants_changed" && strings.Contains(string(event.Details), "viewer") && strings.HasPrefix(event.Object, "submodel:") {
			found = found || strings.HasPrefix(event.Actor, "user:")
		}
	}
	require.True(t, found, "sharing is audited with its actor")

	verified := call(t, "dave", http.MethodGet, auditURL+"/verify", nil, nil)
	expectStatus(t, http.StatusOK, verified, "verify the audit trail")
	var report struct {
		Valid            bool   `json:"valid"`
		Checked          int    `json:"checked"`
		HeadHash         string `json:"headHash"`
		EvidenceVerified int    `json:"evidenceVerified"`
	}
	require.NoError(t, json.Unmarshal(verified.body, &report))
	require.True(t, report.Valid, string(verified.body))
	require.Positive(t, report.Checked)
	require.Positive(t, report.EvidenceVerified, "changes through the Submodel repository are archived in the WORM store")

	stale := call(t, "dave", http.MethodGet, auditURL+"/verify?expectedHead="+strings.Repeat("0", 64), nil, nil)
	require.Contains(t, string(stale.body), `"valid":false`, "a different retained head reveals removed events")
}
