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
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

// neverGrantedUser holds no ReBAC grant and no ABAC right in the suite.
const neverGrantedUser = "frank"

// hiddenResources are resources of alice that nobody else may read. marker
// appears in every identifier, idShort and value of them.
type hiddenResources struct {
	marker  string
	shellID string
	assetID string
}

type listRequest struct {
	method string
	url    string
	body   any
}

func TestListsWithoutGrantsAreEmptyInsteadOfDenied(t *testing.T) {
	hidden := createHiddenResources(t, "alice")
	for _, list := range listRequests(hidden) {
		step := list.method + " " + list.url
		owner := call(t, "alice", list.method, list.url, list.body, nil)
		expectStatus(t, http.StatusOK, owner, "owner: "+step)
		require.Containsf(t, string(owner.body), hidden.marker, "the owner finds the resource: %s", step)

		stranger := call(t, neverGrantedUser, list.method, list.url, list.body, nil)
		expectStatus(t, http.StatusOK, stranger, "caller without grants: "+step)
		require.NotContainsf(t, string(stranger.body), hidden.marker, "lists must not expose unshared resources: %s", step)
	}

	expectStatus(t, http.StatusForbidden, call(t, "", http.MethodGet, environmentURL+"/shells", nil, nil), "anonymous lists keep the ABAC denial")
	expectStatus(t, http.StatusForbidden, call(t, neverGrantedUser, http.MethodGet, environmentURL+"/shells/"+enc(hidden.shellID), nil, nil),
		"single resources without grants keep the ABAC denial")
}

// createHiddenResources lets owner create a Submodel, a shell referencing it
// with derived descriptors and discovery entry, a Concept Description and an
// AASX package.
func createHiddenResources(t *testing.T, owner string) hiddenResources {
	t.Helper()
	bootstrapCreators(t)
	marker := fmt.Sprintf("hidden%d", time.Now().UnixNano())
	hidden := hiddenResources{
		marker:  marker,
		shellID: "urn:rebac-it:" + marker + ":aas",
		assetID: "urn:rebac-it:" + marker + ":asset",
	}
	submodelID := "urn:rebac-it:" + marker + ":sm"
	hiddenShell := shell(hidden.shellID, submodelID)
	hiddenShell["idShort"] = marker
	hiddenShell["assetInformation"] = map[string]any{"assetKind": "Instance", "globalAssetId": hidden.assetID}
	hiddenCD := conceptDescription("urn:rebac-it:" + marker + ":cd")
	hiddenCD["idShort"] = marker
	for _, resource := range []struct {
		collection string
		body       map[string]any
	}{
		{"/submodels", submodel(submodelID, marker, property(marker, marker))},
		{"/shells", hiddenShell},
		{"/concept-descriptions", hiddenCD},
	} {
		created := call(t, owner, http.MethodPost, environmentURL+resource.collection, resource.body, nil)
		expectStatus(t, http.StatusCreated, created, "create "+resource.collection)
	}
	uploadHiddenPackage(t, owner, marker)
	return hidden
}

func uploadHiddenPackage(t *testing.T, owner string, marker string) {
	t.Helper()
	content, err := os.ReadFile(samplePackage)
	require.NoError(t, err)
	created := uploadFile(t, owner, http.MethodPost, packagesURL+"/packages", marker+".aasx", content)
	expectStatus(t, http.StatusCreated, created, "upload package")
}

// listRequests returns a request for every list route ReBAC covers.
func listRequests(hidden hiddenResources) []listRequest {
	requests := []listRequest{
		{http.MethodGet, packagesURL + "/packages?limit=500", nil},
	}
	for _, path := range []string{
		"/shells", "/shells/$reference", "/concept-descriptions", "/shell-descriptors", "/submodel-descriptors",
		"/submodels", "/submodels/$metadata", "/submodels/$value", "/submodels/$reference", "/submodels/$path",
	} {
		requests = append(requests, listRequest{http.MethodGet, environmentURL + path + "?limit=1000", nil})
	}
	for path, field := range map[string]string{
		"/query/shells": "$aas#idShort", "/query/submodels": "$sm#idShort", "/query/concept-descriptions": "$cd#idShort",
		"/query/shell-descriptors": "$aasdesc#idShort", "/query/submodel-descriptors": "$smdesc#idShort",
	} {
		requests = append(requests, listRequest{http.MethodPost, environmentURL + path, idShortQuery(field, hidden.marker)})
	}
	assetLink := map[string]string{"name": common.GlobalAssetIDAssetLinkName, "value": hidden.assetID}
	encodedAssetLink, _ := json.Marshal(assetLink)
	return append(requests,
		listRequest{http.MethodGet, environmentURL + "/lookup/shells?assetIds=" + common.EncodeString(string(encodedAssetLink)), nil},
		listRequest{http.MethodPost, environmentURL + "/lookup/shellsByAssetLink", []any{assetLink}},
	)
}

func idShortQuery(field string, idShort string) map[string]any {
	return map[string]any{"$condition": map[string]any{
		"$eq": []any{map[string]any{"$field": field}, map[string]any{"$strVal": idShort}},
	}}
}
