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
	"bytes"
	"mime/multipart"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const samplePackage = "../../../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx"

// uploadFile posts a file as multipart form field "file".
func uploadFile(t *testing.T, user string, method string, url string, fileName string, content []byte) response {
	t.Helper()
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	part, err := writer.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return callRaw(t, user, method, url, &payload, writer.FormDataContentType(), nil)
}

func eventually(t *testing.T, step string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !condition() {
		require.Truef(t, time.Now().Before(deadline), "timed out: %s", step)
		time.Sleep(200 * time.Millisecond)
	}
}

func TestPackagesBelongToTheirUploader(t *testing.T) {
	bootstrapCreators(t)
	content, err := os.ReadFile(samplePackage)
	require.NoError(t, err)
	expectStatus(t, http.StatusForbidden, uploadFile(t, "eve", http.MethodPost, packagesURL+"/packages", "motor.aasx", content), "uploads need the creator relation")
	created := uploadFile(t, "alice", http.MethodPost, packagesURL+"/packages", "motor.aasx", content)
	expectStatus(t, http.StatusCreated, created, "upload package")
	packageID, _ := created.json(t)["packageId"].(string)
	require.NotEmpty(t, packageID)
	packageURL := packagesURL + "/packages/" + packageID

	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, packageURL, nil, nil), "unshared package")
	require.NotContains(t, string(call(t, "bob", http.MethodGet, packagesURL+"/packages", nil, nil).body), packageID, "unshared packages are not listed")

	addGrants(t, "alice", packageURL+"/$access", userGrant(t, "viewer", "bob"))
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, packageURL, nil, nil), "shared package")
	listed := call(t, "bob", http.MethodGet, packagesURL+"/packages", nil, nil)
	expectStatus(t, http.StatusOK, listed, "list shared packages")
	require.Contains(t, string(listed.body), packageID, "the shared package is listed")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodDelete, packageURL, nil, nil), "viewers cannot delete")

	expectStatus(t, http.StatusNoContent, call(t, "alice", http.MethodDelete, packageURL, nil, nil), "owner deletes the package")
	expectStatus(t, http.StatusNotFound, call(t, "alice", http.MethodGet, packageURL+"/$access", nil, nil), "access state is removed with the package")
}

func TestSerializationContainsOnlyReadableResources(t *testing.T) {
	shared := createSubmodel(t, environmentURL, "alice", "serialized-shared")
	hidden := createSubmodel(t, environmentURL, "alice", "serialized-hidden")
	addGrants(t, "alice", submodelAccess(environmentURL, shared), userGrant(t, "viewer", "bob"))

	serialize := func(identifier string) response {
		query := neturl.Values{"submodelIds": {enc(identifier)}, "includeConceptDescriptions": {"false"}}
		return call(t, "bob", http.MethodGet, environmentURL+"/serialization?"+query.Encode(), nil, map[string]string{"Accept": "application/json"})
	}
	readable := serialize(shared)
	expectStatus(t, http.StatusOK, readable, "serialize a shared Submodel")
	require.Contains(t, string(readable.body), shared)
	denied := serialize(hidden)
	require.NotEqual(t, http.StatusOK, denied.status, "an unshared Submodel is never serialized")
}

func TestUploadCreatesOwnedResources(t *testing.T) {
	bootstrapCreators(t)
	identifier := unique("uploaded")
	environment := `{"assetAdministrationShells":[],"conceptDescriptions":[],"submodels":[` +
		`{"modelType":"Submodel","id":"` + identifier + `","idShort":"uploaded"}]}`
	expectStatus(t, http.StatusForbidden, uploadFile(t, "eve", http.MethodPost, environmentURL+"/upload", "environment.json", []byte(environment)), "uploads need the creator relation")
	expectStatus(t, http.StatusOK, uploadFile(t, "alice", http.MethodPost, environmentURL+"/upload", "environment.json", []byte(environment)), "creator uploads an environment")

	grants, _ := readAccess(t, "alice", submodelAccess(environmentURL, identifier))
	require.Equal(t, []grant{userGrant(t, "owner", "alice")}, grants, "the uploader owns uploaded resources")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, environmentURL+"/submodels/"+enc(identifier), nil, nil), "uploaded resources are private")
}

func TestBulkRegistrationFollowsCreatorsAndOwners(t *testing.T) {
	bootstrapCreators(t)
	shellID := unique("bulk")
	payload := []any{shellDescriptor(shellID, unique("bulk-embedded"))}
	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodPost, environmentURL+"/bulk/shell-descriptors", payload, nil), "bulk registration needs the creator relation")
	started := call(t, "alice", http.MethodPost, environmentURL+"/bulk/shell-descriptors", payload, nil)
	require.Contains(t, []int{http.StatusAccepted, http.StatusOK}, started.status, string(started.body))
	eventually(t, "bulk registration", func() bool {
		return call(t, "alice", http.MethodGet, shellDescriptorURL(shellID), nil, nil).status == http.StatusOK
	})
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, shellDescriptorURL(shellID), nil, nil), "bulk-registered descriptors are private")
	deleted := call(t, "bob", http.MethodDelete, environmentURL+"/bulk/shell-descriptors", []string{shellID}, nil)
	require.NotEqual(t, http.StatusAccepted, deleted.status, "strangers cannot delete in bulk")
}

func TestPassportsFollowTheirShell(t *testing.T) {
	bootstrapCreators(t)
	passportID := "urn:example:dpp:" + strings.ReplaceAll(unique("passport"), ":", "-")
	document := map[string]any{
		"digitalProductPassportId":            passportID,
		"uniqueProductIdentifier":             passportID + ":product",
		"granularity":                         "Item",
		"dppSchemaVersion":                    "1.0.0",
		"dppStatus":                           "active",
		"lastUpdate":                          time.Now().UTC().Format(time.RFC3339Nano),
		"economicOperatorId":                  "operator-123",
		"contentSpecificationIds":             []string{"urn:example:semantic:technical-data"},
		"urn:example:semantic:technical-data": map[string]any{"manufacturerName": "Acme GmbH"},
	}
	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodPost, passportsURL+"/v1/dpps", document, nil), "passports need the creator relations")
	expectStatus(t, http.StatusCreated, call(t, "alice", http.MethodPost, passportsURL+"/v1/dpps", document, nil), "creator creates a passport")

	passportURL := passportsURL + "/v1/dpps/" + neturl.PathEscape(passportID)
	expectStatus(t, http.StatusOK, call(t, "alice", http.MethodGet, passportURL, nil, nil), "owner reads the passport")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, passportURL, nil, nil), "unshared passport")

	addGrants(t, "alice", aasURL+"/shells/"+enc(passportID)+"/$access", userGrant(t, "viewer", "bob"))
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, passportURL, nil, nil), "passport Submodels are linked to the passport shell")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodDelete, passportURL, nil, nil), "viewers cannot delete passports")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, passportsURL+"/v1/dppsByIdAndDate/"+neturl.PathEscape(passportID)+"?date="+neturl.QueryEscape(time.Now().UTC().Format(time.RFC3339)), nil, nil), "historical passports stay ABAC-only")
}
