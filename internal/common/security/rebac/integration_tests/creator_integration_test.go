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
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func creatorReference(value string) map[string]any {
	return map[string]any{
		"type": "ExternalReference",
		"keys": []any{map[string]any{"type": "GlobalReference", "value": value}},
	}
}

// principalCreator is the creator BaSyx fills in for a test user.
func principalCreator(t *testing.T, user string) map[string]any {
	t.Helper()
	return creatorReference(issuer + "|" + subject(t, user))
}

func creatorOf(t *testing.T, document map[string]any) any {
	t.Helper()
	administration, _ := document["administration"].(map[string]any)
	return administration["creator"]
}

func readCreator(t *testing.T, user string, url string) any {
	t.Helper()
	result := call(t, user, http.MethodGet, url, nil, nil)
	expectStatus(t, http.StatusOK, result, "read "+url)
	return creatorOf(t, result.json(t))
}

func createAndExpectCreator(t *testing.T, user string, collectionURL string, resourceURL string, body map[string]any, expected any) {
	t.Helper()
	created := call(t, user, http.MethodPost, collectionURL, body, nil)
	expectStatus(t, http.StatusCreated, created, "create "+collectionURL)
	require.Equal(t, expected, creatorOf(t, created.json(t)), "the create response carries the creator")
	require.Equal(t, expected, readCreator(t, user, resourceURL), "the stored resource carries the creator")
}

func TestCreatorIsFilledForCreatedIdentifiables(t *testing.T) {
	bootstrapCreators(t)
	expected := principalCreator(t, "alice")

	shellID, submodelID, cdID := unique("creator-aas"), unique("creator-sm"), unique("creator-cd")
	createAndExpectCreator(t, "alice", aasURL+"/shells", aasURL+"/shells/"+enc(shellID), shell(shellID), expected)
	createAndExpectCreator(t, "alice", submodelURL+"/submodels", submodelURL+"/submodels/"+enc(submodelID), submodel(submodelID, "creator"), expected)
	createAndExpectCreator(t, "alice", cdURL+"/concept-descriptions", cdURL+"/concept-descriptions/"+enc(cdID), conceptDescription(cdID), expected)

	versioned := unique("creator-versioned")
	body := submodel(versioned, "versioned")
	body["administration"] = map[string]any{"version": "2", "revision": "1"}
	createAndExpectCreator(t, "alice", submodelURL+"/submodels", submodelURL+"/submodels/"+enc(versioned), body, expected)
	administration := call(t, "alice", http.MethodGet, submodelURL+"/submodels/"+enc(versioned), nil, nil).json(t)["administration"].(map[string]any)
	require.Equal(t, "2", administration["version"], "other administrative information is kept")
	require.Equal(t, "1", administration["revision"], "other administrative information is kept")
}

func TestClientProvidedCreatorIsKept(t *testing.T) {
	bootstrapCreators(t)
	provided := creatorReference("urn:example:creator:" + unique("provided"))
	submodelID := unique("creator-provided")
	body := submodel(submodelID, "provided")
	body["administration"] = map[string]any{"creator": provided}
	createAndExpectCreator(t, "alice", submodelURL+"/submodels", submodelURL+"/submodels/"+enc(submodelID), body, provided)
}

func TestUpdatesNeverTouchTheCreator(t *testing.T) {
	bootstrapCreators(t)
	submodelURLOf := func(identifier string) string { return submodelURL + "/submodels/" + enc(identifier) }

	upserted := unique("creator-upsert")
	created := call(t, "alice", http.MethodPut, submodelURLOf(upserted), submodel(upserted, "upserted"), nil)
	require.Contains(t, []int{http.StatusCreated, http.StatusNoContent}, created.status, string(created.body))
	require.Equal(t, principalCreator(t, "alice"), readCreator(t, "alice", submodelURLOf(upserted)), "an upsert that creates fills the creator")

	expectStatus(t, http.StatusNoContent, call(t, "alice", http.MethodPut, submodelURLOf(upserted), submodel(upserted, "cleared"), nil), "replace without administration")
	require.Nil(t, readCreator(t, "alice", submodelURLOf(upserted)), "updates may remove the creator")

	changed := creatorReference("urn:example:creator:" + unique("changed"))
	replacement := submodel(upserted, "changed")
	replacement["administration"] = map[string]any{"creator": changed}
	expectStatus(t, http.StatusNoContent, call(t, "alice", http.MethodPut, submodelURLOf(upserted), replacement, nil), "replace with another creator")
	require.Equal(t, changed, readCreator(t, "alice", submodelURLOf(upserted)), "updates may change the creator")
}

func TestCreatorIsFilledForRegisteredDescriptors(t *testing.T) {
	bootstrapCreators(t)
	expected := principalCreator(t, "alice")

	shellID, embeddedID := unique("creator-registered"), unique("creator-embedded")
	createAndExpectCreator(t, "alice", environmentURL+"/shell-descriptors", shellDescriptorURL(shellID), shellDescriptor(shellID, embeddedID), expected)
	require.Equal(t, expected, readCreator(t, "alice", shellDescriptorURL(shellID)+"/submodel-descriptors/"+enc(embeddedID)), "embedded descriptors created with the shell descriptor carry the creator")

	nestedID := unique("creator-nested")
	nested := map[string]any{"id": nestedID, "idShort": "nested", "endpoints": descriptorEndpoint("submodels")}
	createAndExpectCreator(t, "alice", shellDescriptorURL(shellID)+"/submodel-descriptors", shellDescriptorURL(shellID)+"/submodel-descriptors/"+enc(nestedID), nested, expected)

	standaloneID := unique("creator-standalone")
	standalone := map[string]any{"id": standaloneID, "idShort": "standalone", "endpoints": descriptorEndpoint("submodels")}
	createAndExpectCreator(t, "alice", environmentURL+"/submodel-descriptors", environmentURL+"/submodel-descriptors/"+enc(standaloneID), standalone, expected)
}

func TestSynchronizedDescriptorsMirrorTheirSource(t *testing.T) {
	shellID, submodelID := createShellWithSubmodel(t, "alice")
	expected := principalCreator(t, "alice")
	require.Equal(t, expected, readCreator(t, "alice", environmentURL+"/shells/"+enc(shellID)), "the shell carries its creator")
	require.Equal(t, expected, readCreator(t, "alice", shellDescriptorURL(shellID)), "the synchronized descriptor mirrors the shell")
	require.Equal(t, expected, readCreator(t, "alice", environmentURL+"/submodel-descriptors/"+enc(submodelID)), "the synchronized descriptor mirrors the Submodel")

	expectStatus(t, http.StatusNoContent, call(t, "alice", http.MethodPut, environmentURL+"/shells/"+enc(shellID), shell(shellID, submodelID), nil), "replace the shell without administration")
	require.Nil(t, readCreator(t, "alice", shellDescriptorURL(shellID)), "the synchronized descriptor is not filled on its own")
}

func TestUploadsAndBulkRegistrationFillTheCreator(t *testing.T) {
	bootstrapCreators(t)
	expected := principalCreator(t, "alice")

	uploaded := unique("creator-uploaded")
	environment := `{"assetAdministrationShells":[],"conceptDescriptions":[],"submodels":[` +
		`{"modelType":"Submodel","id":"` + uploaded + `","idShort":"uploaded"}]}`
	expectStatus(t, http.StatusOK, uploadFile(t, "alice", http.MethodPost, environmentURL+"/upload", "environment.json", []byte(environment)), "upload an environment")
	require.Equal(t, expected, readCreator(t, "alice", environmentURL+"/submodels/"+enc(uploaded)), "uploaded resources carry the creator")

	shellID := unique("creator-bulk")
	started := call(t, "alice", http.MethodPost, environmentURL+"/bulk/shell-descriptors", []any{shellDescriptor(shellID, unique("creator-bulk-embedded"))}, nil)
	require.Contains(t, []int{http.StatusAccepted, http.StatusOK}, started.status, string(started.body))
	eventually(t, "bulk registration", func() bool {
		return call(t, "alice", http.MethodGet, shellDescriptorURL(shellID), nil, nil).status == http.StatusOK
	})
	require.Equal(t, expected, readCreator(t, "alice", shellDescriptorURL(shellID)), "bulk-registered descriptors carry the creator")
}
