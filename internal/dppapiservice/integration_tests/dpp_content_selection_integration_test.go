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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package integration_tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const selectionUnrelatedContentSpec = "urn:example:semantic:selection-unrelated-content"

type dppContentSelectionFixture struct {
	client                   *http.Client
	baseURL, aasBaseURL      string
	databasePort             int
	dppID, encodedDPPID      string
	technicalID, carbonID    string
	unrelatedID              string
	aas                      map[string]any
	aasRevision              int64
	contentSubmodelSnapshots []persistedSubmodelSnapshot
}

func testDPPContentSpecificationSelection(
	t *testing.T,
	client *http.Client,
	baseURL string,
	aasBaseURL string,
	databasePort int,
	idSuffix string,
	now time.Time,
) {
	t.Helper()
	fixture := newDPPContentSelectionFixture(t, client, baseURL, aasBaseURL, databasePort, idSuffix, now)
	initialSelectionDate := latestDPPHistoryTimestamp(t, databasePort, fixture.dppID)
	assertCurrentDPPSelection(t, fixture, lifecycleTechnicalDataSpec)

	patchDPPContentSelection(t, fixture, []string{lifecycleCarbonFootprintSpec})
	carbonSelectionDate := latestDPPHistoryTimestamp(t, databasePort, fixture.dppID)
	assertCurrentDPPSelection(t, fixture, lifecycleCarbonFootprintSpec)
	assertHistoricalDPPSelection(t, fixture, initialSelectionDate, lifecycleTechnicalDataSpec)

	patchDPPContentSelection(t, fixture, nil)
	assertCurrentDPPSelection(t, fixture)
	assertHistoricalDPPSelection(t, fixture, carbonSelectionDate, lifecycleCarbonFootprintSpec)

	patchDPPContentSelection(t, fixture, []string{})
	assertCurrentDPPSelection(t, fixture)
	patchDPPContentSelection(t, fixture, []string{lifecycleTechnicalDataSpec, lifecycleCarbonFootprintSpec})
	assertCurrentDPPSelection(t, fixture, lifecycleTechnicalDataSpec, lifecycleCarbonFootprintSpec)
	assertDPPContentSelectionResourcesUnchanged(t, fixture)
}

func newDPPContentSelectionFixture(
	t *testing.T,
	client *http.Client,
	baseURL string,
	aasBaseURL string,
	databasePort int,
	idSuffix string,
	now time.Time,
) dppContentSelectionFixture {
	t.Helper()
	dppID := "https://www.example.org/dpp/content-selection/" + idSuffix
	document := lifecycleDPPDocument(dppID, "https://www.example.org/product/content-selection/"+idSuffix, now)
	doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", document, http.StatusCreated)
	technicalID := submodelIDBySemanticID(t, databasePort, dppID, lifecycleTechnicalDataSpec)
	carbonID := submodelIDBySemanticID(t, databasePort, dppID, lifecycleCarbonFootprintSpec)
	unrelatedID := addReferencedSubmodelClone(
		t,
		client,
		aasBaseURL,
		dppID,
		carbonID,
		"https://www.example.org/submodels/selection-unrelated/"+idSuffix,
		"SelectionUnrelatedContent",
		selectionUnrelatedContentSpec,
	)
	fixture := dppContentSelectionFixture{
		client:       client,
		baseURL:      baseURL,
		aasBaseURL:   aasBaseURL,
		databasePort: databasePort,
		dppID:        dppID,
		encodedDPPID: encodedPathParam(dppID),
		technicalID:  technicalID,
		carbonID:     carbonID,
		unrelatedID:  unrelatedID,
	}
	fixture.aas = doJSON(t, client, http.MethodGet, aasBaseURL+"/shells/"+common.EncodeString(dppID), nil, http.StatusOK)
	fixture.aasRevision = historyRevision(t, databasePort, "aas_history", dppID)
	snapshotFixture := selectiveDPPFixture{client: client, aasBaseURL: aasBaseURL, databasePort: databasePort}
	fixture.contentSubmodelSnapshots = []persistedSubmodelSnapshot{
		submodelSnapshot(t, snapshotFixture, technicalID),
		submodelSnapshot(t, snapshotFixture, carbonID),
		submodelSnapshot(t, snapshotFixture, unrelatedID),
	}
	patchDPPContentSelection(t, fixture, []string{lifecycleTechnicalDataSpec})
	return fixture
}

func patchDPPContentSelection(t *testing.T, fixture dppContentSelectionFixture, specificationIDs []string) {
	t.Helper()
	doJSON(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		"contentSpecificationIds": specificationIDs,
	}, http.StatusOK)
}

func assertCurrentDPPSelection(t *testing.T, fixture dppContentSelectionFixture, selected ...string) {
	t.Helper()
	selectedSet := make(map[string]struct{}, len(selected))
	for _, specificationID := range selected {
		selectedSet[specificationID] = struct{}{}
	}
	endpoint := fixture.baseURL + "/v1/dpps/" + fixture.encodedDPPID
	body := doJSON(t, fixture.client, http.MethodGet, endpoint, nil, http.StatusOK)
	assertCompressedSelection(t, body, selectedSet)
	fullBody := doJSON(t, fixture.client, http.MethodGet, endpoint+"?representation=full", nil, http.StatusOK)
	assertFullSelection(t, fullBody, selectedSet)
	assertSelectionElementAccess(t, fixture, lifecycleTechnicalDataSpec, "manufacturerName", selectedSet)
	assertSelectionElementAccess(t, fixture, lifecycleCarbonFootprintSpec, "PcfCo2eq", selectedSet)
	assertSelectionElementAccess(t, fixture, selectionUnrelatedContentSpec, "PcfCo2eq", selectedSet)
}

func assertHistoricalDPPSelection(
	t *testing.T,
	fixture dppContentSelectionFixture,
	date time.Time,
	selected ...string,
) {
	t.Helper()
	selectedSet := make(map[string]struct{}, len(selected))
	for _, specificationID := range selected {
		selectedSet[specificationID] = struct{}{}
	}
	compressed := doJSON(
		t, fixture.client, http.MethodGet, historyURL(fixture.baseURL, fixture.encodedDPPID, date, "compressed"), nil, http.StatusOK,
	)
	assertCompressedSelection(t, compressed, selectedSet)
	full := doJSON(
		t, fixture.client, http.MethodGet, historyURL(fixture.baseURL, fixture.encodedDPPID, date, "full"), nil, http.StatusOK,
	)
	assertFullSelection(t, full, selectedSet)
}

func assertCompressedSelection(t *testing.T, body map[string]any, selected map[string]struct{}) {
	t.Helper()
	for _, specificationID := range []string{lifecycleTechnicalDataSpec, lifecycleCarbonFootprintSpec, selectionUnrelatedContentSpec} {
		_, expected := selected[specificationID]
		_, present := body[specificationID]
		if present != expected {
			t.Fatalf("compressed DPP section %q present = %t, want %t", specificationID, present, expected)
		}
	}
}

func assertFullSelection(t *testing.T, body map[string]any, selected map[string]struct{}) {
	t.Helper()
	elements, ok := body["elements"].([]any)
	if !ok {
		t.Fatalf("full DPP elements = %#v, want array", body["elements"])
	}
	if len(elements) != len(selected) {
		t.Fatalf("full DPP elements = %#v, want %d selected sections", elements, len(selected))
	}
	for specificationID := range selected {
		if _, found := findFullElementByDictionaryReference(elements, specificationID); !found {
			t.Fatalf("full DPP elements do not contain selected section %q: %#v", specificationID, elements)
		}
	}
}

func assertSelectionElementAccess(
	t *testing.T,
	fixture dppContentSelectionFixture,
	specificationID string,
	elementID string,
	selected map[string]struct{},
) {
	t.Helper()
	status := http.StatusNotFound
	if _, expected := selected[specificationID]; expected {
		status = http.StatusOK
	}
	path := encodedPathParam(dppElementJSONPath(specificationID, elementID))
	doJSONAny(t, fixture.client, http.MethodGet, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID+"/elements/"+path, nil, status)
}

func assertDPPContentSelectionResourcesUnchanged(t *testing.T, fixture dppContentSelectionFixture) {
	t.Helper()
	assertRevisionUnchanged(t, fixture.databasePort, "aas_history", fixture.dppID, fixture.aasRevision)
	currentAAS := doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shells/"+common.EncodeString(fixture.dppID), nil, http.StatusOK)
	assertJSONEquals(t, currentAAS, fixture.aas)
	for _, submodelID := range []string{fixture.technicalID, fixture.carbonID, fixture.unrelatedID} {
		assertAASReferencesSubmodel(t, fixture.client, fixture.aasBaseURL, fixture.dppID, submodelID, true)
		doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodel-descriptors/"+common.EncodeString(submodelID), nil, http.StatusOK)
	}
	snapshotFixture := selectiveDPPFixture{client: fixture.client, aasBaseURL: fixture.aasBaseURL, databasePort: fixture.databasePort}
	for _, snapshot := range fixture.contentSubmodelSnapshots {
		assertSubmodelSnapshotUnchanged(t, snapshotFixture, snapshot)
	}
}
