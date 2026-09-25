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
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

func shellDescriptorURL(identifier string) string {
	return environmentURL + "/shell-descriptors/" + enc(identifier)
}

func discoveryEntryURL(identifier string) string {
	return environmentURL + "/lookup/shells/" + enc(identifier)
}

func descriptorEndpoint(path string) []any {
	return []any{map[string]any{
		"interface":           "AAS-3.0",
		"protocolInformation": map[string]any{"href": "https://example.com/" + path},
	}}
}

func shellDescriptor(identifier string, submodelID string) map[string]any {
	return map[string]any{
		"id": identifier, "idShort": "registered", "endpoints": descriptorEndpoint("shells"),
		"submodelDescriptors": []any{map[string]any{"id": submodelID, "idShort": "embedded", "endpoints": descriptorEndpoint("submodels")}},
	}
}

func assetLinks(value string) []any {
	return []any{map[string]any{"name": "serialNumber", "value": value}}
}

// createShellWithSubmodel creates a Submodel and a shell referencing it in
// the AAS Environment, whose registry synchronization derives descriptors
// and a discovery entry from them.
func createShellWithSubmodel(t *testing.T, owner string) (string, string) {
	t.Helper()
	submodelID := createSubmodel(t, environmentURL, owner, "synchronized")
	shellID := unique("aas")
	result := call(t, owner, http.MethodPost, environmentURL+"/shells", shell(shellID, submodelID), nil)
	expectStatus(t, http.StatusCreated, result, "create synchronized shell")
	return shellID, submodelID
}

func descriptorIDShort(t *testing.T, user string, url string) string {
	t.Helper()
	result := call(t, user, http.MethodGet, url, nil, nil)
	expectStatus(t, http.StatusOK, result, "read descriptor")
	idShort, _ := result.json(t)["idShort"].(string)
	return idShort
}

func TestSynchronizedDescriptorsFollowTheirSource(t *testing.T) {
	shellID, submodelID := createShellWithSubmodel(t, "alice")
	descriptorURL := shellDescriptorURL(shellID)

	expectStatus(t, http.StatusOK, call(t, "alice", http.MethodGet, descriptorURL, nil, nil), "shell owner reads the derived descriptor")
	expectStatus(t, http.StatusOK, call(t, "alice", http.MethodGet, discoveryEntryURL(shellID), nil, nil), "shell owner reads the derived discovery entry")
	expectStatus(t, http.StatusOK, call(t, "alice", http.MethodGet, environmentURL+"/submodel-descriptors/"+enc(submodelID), nil, nil), "Submodel owner reads its descriptor")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, descriptorURL, nil, nil), "unshared descriptor")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, discoveryEntryURL(shellID), nil, nil), "unshared discovery entry")

	access := call(t, "alice", http.MethodGet, descriptorURL+"/$access", nil, nil)
	expectStatus(t, http.StatusOK, access, "manage derived descriptor through the shell")
	derivedFrom, _ := access.json(t)["derivedFrom"].(map[string]any)
	require.Equal(t, map[string]any{"type": "aas", "id": shellID}, derivedFrom)

	shellAccess := environmentURL + "/shells/" + enc(shellID) + "/$access"
	addGrants(t, "alice", shellAccess, userGrant(t, "editor", "bob"))
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, descriptorURL, nil, nil), "shell editors read the descriptor")
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, discoveryEntryURL(shellID), nil, nil), "discovery entries follow the descriptor")
	expectStatus(t, http.StatusNotFound, call(t, "bob", http.MethodGet, descriptorURL+"/$access", nil, nil), "editors cannot manage")

	renamed := shell(shellID, submodelID)
	renamed["idShort"] = "renamed"
	expectStatus(t, http.StatusNoContent, call(t, "bob", http.MethodPut, environmentURL+"/shells/"+enc(shellID), renamed, nil), "editor updates the shell")
	require.Equal(t, "renamed", descriptorIDShort(t, "alice", descriptorURL), "the synchronized descriptor follows the shell")

	setGrants(t, "alice", shellAccess, []grant{userGrant(t, "owner", "alice")})
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, descriptorURL, nil, nil), "revocation reaches the descriptor")

	descriptors := goqu.Dialect("postgres").From("aas_descriptor").Where(goqu.C("id").Eq(shellID))
	derivations := goqu.Dialect("postgres").From("rebac_derivation").
		Where(goqu.C("object_uuid").In(goqu.Dialect("postgres").From(goqu.T("aas_descriptor").As("ad")).
			InnerJoin(goqu.T("descriptor").As("d"), goqu.On(goqu.I("d.id").Eq(goqu.I("ad.descriptor_id")))).
			Select(goqu.I("d.auth_uuid")).Where(goqu.I("ad.id").Eq(shellID))))
	require.Equal(t, 1, countRows(t, derivations), "the synchronized descriptor is derived from the shell")
	expectStatus(t, http.StatusNoContent, call(t, "alice", http.MethodDelete, environmentURL+"/shells/"+enc(shellID), nil, nil), "owner deletes the shell")
	require.Zero(t, countRows(t, descriptors), "the descriptor is deleted with the shell")
}

func countRows(t *testing.T, ds *goqu.SelectDataset) int {
	t.Helper()
	query, args, err := ds.Select(goqu.COUNT(goqu.Star())).ToSQL()
	require.NoError(t, err)
	var count int
	require.NoError(t, openDB(t).QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count
}

func TestSubmodelEditorsRefreshEmbeddedDescriptorsOnly(t *testing.T) {
	shellID, submodelID := createShellWithSubmodel(t, "alice")
	addGrants(t, "alice", submodelAccess(environmentURL, submodelID), userGrant(t, "editor", "bob"))

	renamed := submodel(submodelID, "refreshed")
	expectStatus(t, http.StatusNoContent, call(t, "bob", http.MethodPut, environmentURL+"/submodels/"+enc(submodelID), renamed, nil), "Submodel editor updates the Submodel")

	embedded := call(t, "alice", http.MethodGet, shellDescriptorURL(shellID)+"/submodel-descriptors/"+enc(submodelID), nil, nil)
	expectStatus(t, http.StatusOK, embedded, "read embedded descriptor")
	require.Equal(t, "refreshed", embedded.json(t)["idShort"], "the embedded descriptor follows the Submodel")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, shellDescriptorURL(shellID), nil, nil), "the synchronization grants no read on the shell descriptor")
}

func TestRegisteredDescriptorsBelongToTheirRegistrant(t *testing.T) {
	bootstrapCreators(t)
	shellID, submodelID := unique("registered"), unique("embedded")
	expectStatus(t, http.StatusCreated, call(t, "alice", http.MethodPost, environmentURL+"/shell-descriptors", shellDescriptor(shellID, submodelID), nil), "register descriptor")
	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodPost, environmentURL+"/shell-descriptors", shellDescriptor(unique("registered"), submodelID), nil), "registration needs the creator relation")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, shellDescriptorURL(shellID), nil, nil), "unshared descriptor")

	addGrants(t, "alice", shellDescriptorURL(shellID)+"/$access", userGrant(t, "viewer", "bob"))
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, shellDescriptorURL(shellID), nil, nil), "shared descriptor")
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, shellDescriptorURL(shellID)+"/submodel-descriptors/"+enc(submodelID), nil, nil), "embedded descriptors belong to the shell descriptor")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodDelete, shellDescriptorURL(shellID)+"/submodel-descriptors/"+enc(submodelID), nil, nil), "viewers cannot edit")

	listed := call(t, "bob", http.MethodGet, environmentURL+"/shell-descriptors?limit=1000", nil, nil)
	expectStatus(t, http.StatusOK, listed, "list readable descriptors")
	var page struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(listed.body, &page))
	found := false
	for _, descriptor := range page.Result {
		found = found || descriptor.ID == shellID
	}
	require.True(t, found, "the shared descriptor is listed")

	conflicting := call(t, "carol", http.MethodPost, environmentURL+"/shells", shell(shellID), nil)
	require.NotEqual(t, http.StatusCreated, conflicting.status, "a shell with the same identifier never takes over a registered descriptor")
	expectStatus(t, http.StatusForbidden, call(t, "carol", http.MethodGet, shellDescriptorURL(shellID), nil, nil), "identifier matches grant nothing")
}

func TestDiscoveryEntriesBelongToTheirCreator(t *testing.T) {
	bootstrapCreators(t)
	shellID, value := unique("linked"), unique("serial")
	expectStatus(t, http.StatusCreated, call(t, "alice", http.MethodPost, discoveryEntryURL(shellID), assetLinks(value), nil), "create discovery entry")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, discoveryEntryURL(shellID), nil, nil), "unshared entry")
	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodPost, discoveryEntryURL(shellID), assetLinks("taken"), nil), "strangers cannot replace links")

	addGrants(t, "alice", discoveryEntryURL(shellID)+"/$access", userGrant(t, "viewer", "bob"))
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, discoveryEntryURL(shellID), nil, nil), "shared entry")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodPost, discoveryEntryURL(shellID), assetLinks("changed"), nil), "viewers cannot replace links")

	search := call(t, "bob", http.MethodPost, environmentURL+"/lookup/shellsByAssetLink", assetLinks(value), nil)
	expectStatus(t, http.StatusOK, search, "search by asset link")
	require.Contains(t, string(search.body), shellID, "shared entries are found by their links")
	hidden := call(t, "eve", http.MethodPost, environmentURL+"/lookup/shellsByAssetLink", assetLinks(value), nil)
	require.Contains(t, []int{http.StatusOK, http.StatusForbidden}, hidden.status, "callers see their own entries or the ABAC denial")
	require.NotContains(t, string(hidden.body), shellID, "unshared entries are never found")

	expectStatus(t, http.StatusNoContent, call(t, "alice", http.MethodDelete, discoveryEntryURL(shellID), nil, nil), "owner deletes the entry")
	expectStatus(t, http.StatusNotFound, call(t, "alice", http.MethodGet, discoveryEntryURL(shellID)+"/$access", nil, nil), "access state is removed with the entry")
}
