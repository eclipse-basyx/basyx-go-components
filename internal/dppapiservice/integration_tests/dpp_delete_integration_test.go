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

const deleteUnrelatedContentSpec = "urn:example:semantic:unrelated-delete-content"

type dppDeletionRegressionFixture struct {
	client                                *http.Client
	baseURL, aasBaseURL                   string
	databasePort                          int
	deletedAASID, deletedMetadataID       string
	retainedTechnicalID, retainedCarbonID string
	retainedUnrelatedID                   string
	consumerDPPID, consumerMetadataID     string
	consumerCarbonID                      string
}

func prepareDPPDeletionRegression(
	t *testing.T,
	fixture dppDeletionRegressionFixture,
	idSuffix string,
	now time.Time,
) dppDeletionRegressionFixture {
	t.Helper()
	consumerDPPID := "https://www.example.org/dpp/delete-consumer/" + idSuffix
	consumerProductID := "https://www.example.org/product/delete-consumer/" + idSuffix
	doJSON(t, fixture.client, http.MethodPost, fixture.baseURL+"/v1/dpps", lifecycleDPPDocument(consumerDPPID, consumerProductID, now), http.StatusCreated)

	consumerTechnicalID := submodelIDBySemanticID(t, fixture.databasePort, consumerDPPID, lifecycleTechnicalDataSpec)
	consumerCarbonID := submodelIDBySemanticID(t, fixture.databasePort, consumerDPPID, lifecycleCarbonFootprintSpec)
	replaceAASSubmodelReference(t, fixture.client, fixture.aasBaseURL, consumerDPPID, consumerTechnicalID, fixture.retainedTechnicalID)
	retainedUnrelatedID := addReferencedSubmodelClone(
		t,
		fixture.client,
		fixture.aasBaseURL,
		fixture.deletedAASID,
		fixture.retainedCarbonID,
		"https://www.example.org/submodels/delete-unrelated/"+idSuffix,
		"UnrelatedDeleteContent",
		deleteUnrelatedContentSpec,
	)
	deletedDPPBody := doJSON(t, fixture.client, http.MethodGet, fixture.baseURL+"/v1/dpps/"+encodedPathParam(fixture.deletedAASID), nil, http.StatusOK)
	assertJSONFieldMissing(t, deletedDPPBody, deleteUnrelatedContentSpec)

	consumerBody := doJSON(t, fixture.client, http.MethodGet, fixture.baseURL+"/v1/dpps/"+encodedPathParam(consumerDPPID), nil, http.StatusOK)
	assertDPPSectionPathEquals(t, consumerBody, lifecycleTechnicalDataSpec, "manufacturerName", "Acme Updated GmbH")

	fixture.retainedUnrelatedID = retainedUnrelatedID
	fixture.consumerDPPID = consumerDPPID
	fixture.consumerMetadataID = consumerDPPID + "/submodels/DppMetadata"
	fixture.consumerCarbonID = consumerCarbonID
	return fixture
}

func addReferencedSubmodelClone(
	t *testing.T,
	client *http.Client,
	aasBaseURL string,
	aasID string,
	sourceSubmodelID string,
	newSubmodelID string,
	newIDShort string,
	newSemanticID string,
) string {
	t.Helper()
	unrelatedSubmodel := doJSON(t, client, http.MethodGet, aasBaseURL+"/submodels/"+common.EncodeString(sourceSubmodelID), nil, http.StatusOK)
	unrelatedSubmodel["id"] = newSubmodelID
	unrelatedSubmodel["idShort"] = newIDShort
	unrelatedSubmodel["semanticId"] = map[string]any{
		"type": "ExternalReference",
		"keys": []any{map[string]any{
			"type":  "GlobalReference",
			"value": newSemanticID,
		}},
	}
	doJSON(t, client, http.MethodPost, aasBaseURL+"/submodels", unrelatedSubmodel, http.StatusCreated)

	aas := doJSON(t, client, http.MethodGet, aasBaseURL+"/shells/"+common.EncodeString(aasID), nil, http.StatusOK)
	references, ok := aas["submodels"].([]any)
	if !ok {
		t.Fatalf("AAS %q submodels = %#v, want array", aasID, aas["submodels"])
	}
	aas["submodels"] = append(references, submodelReferencePayload(newSubmodelID))
	doJSON(t, client, http.MethodPut, aasBaseURL+"/shells/"+common.EncodeString(aasID), aas, http.StatusNoContent)
	assertAASReferencesSubmodel(t, client, aasBaseURL, aasID, newSubmodelID, true)
	doJSON(t, client, http.MethodGet, aasBaseURL+"/submodel-descriptors/"+common.EncodeString(newSubmodelID), nil, http.StatusOK)
	return newSubmodelID
}

func replaceAASSubmodelReference(
	t *testing.T,
	client *http.Client,
	aasBaseURL string,
	aasID string,
	currentSubmodelID string,
	replacementSubmodelID string,
) {
	t.Helper()
	aas := doJSON(t, client, http.MethodGet, aasBaseURL+"/shells/"+common.EncodeString(aasID), nil, http.StatusOK)
	references, ok := aas["submodels"].([]any)
	if !ok {
		t.Fatalf("AAS %q submodels = %#v, want array", aasID, aas["submodels"])
	}
	for index, reference := range references {
		if referenceMatchesSubmodel(reference, currentSubmodelID) {
			references[index] = submodelReferencePayload(replacementSubmodelID)
			doJSON(t, client, http.MethodPut, aasBaseURL+"/shells/"+common.EncodeString(aasID), aas, http.StatusNoContent)
			return
		}
	}
	t.Fatalf("AAS %q does not reference submodel %q", aasID, currentSubmodelID)
}

func assertDPPDeletionRegression(t *testing.T, fixture dppDeletionRegressionFixture) {
	t.Helper()
	assertAASIdentifierExists(t, fixture.databasePort, fixture.deletedAASID, false)
	assertSubmodelIdentifierExists(t, fixture.databasePort, fixture.deletedMetadataID, false)
	doJSONAny(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shells/"+common.EncodeString(fixture.deletedAASID), nil, http.StatusNotFound)
	doJSONAny(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(fixture.deletedMetadataID), nil, http.StatusNotFound)
	doJSONAny(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shell-descriptors/"+common.EncodeString(fixture.deletedAASID), nil, http.StatusNotFound)
	doJSONAny(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodel-descriptors/"+common.EncodeString(fixture.deletedMetadataID), nil, http.StatusNotFound)

	assertRetainedSubmodelAndDescriptor(t, fixture, fixture.retainedTechnicalID)
	assertRetainedSubmodelAndDescriptor(t, fixture, fixture.retainedCarbonID)
	assertRetainedSubmodelAndDescriptor(t, fixture, fixture.retainedUnrelatedID)
	assertRetainedSubmodelAndDescriptor(t, fixture, fixture.consumerCarbonID)
	assertSubmodelIdentifierExists(t, fixture.databasePort, fixture.consumerMetadataID, true)
	doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shells/"+common.EncodeString(fixture.consumerDPPID), nil, http.StatusOK)
	doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shell-descriptors/"+common.EncodeString(fixture.consumerDPPID), nil, http.StatusOK)
	doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodel-descriptors/"+common.EncodeString(fixture.consumerMetadataID), nil, http.StatusOK)

	consumerBody := doJSON(t, fixture.client, http.MethodGet, fixture.baseURL+"/v1/dpps/"+encodedPathParam(fixture.consumerDPPID), nil, http.StatusOK)
	assertDPPSectionPathEquals(t, consumerBody, lifecycleTechnicalDataSpec, "manufacturerName", "Acme Updated GmbH")
	assertDPPSectionPathEquals(t, consumerBody, lifecycleCarbonFootprintSpec, "PcfCo2eq", "4180.75")
}

func assertRetainedSubmodelAndDescriptor(t *testing.T, fixture dppDeletionRegressionFixture, submodelID string) {
	t.Helper()
	assertSubmodelIdentifierExists(t, fixture.databasePort, submodelID, true)
	doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(submodelID), nil, http.StatusOK)
	doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodel-descriptors/"+common.EncodeString(submodelID), nil, http.StatusOK)
}
