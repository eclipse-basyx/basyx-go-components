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
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func testSelectiveDPPUpdates(
	t *testing.T,
	client *http.Client,
	baseURL string,
	aasBaseURL string,
	databasePort int,
	idSuffix string,
	now time.Time,
) {
	t.Helper()
	fixture := newSelectiveDPPFixture(t, client, baseURL, aasBaseURL, databasePort, idSuffix, now)
	configureSelectiveUpdateFixtures(t, fixture)
	fixture.sharedAASID = createSharedContentAAS(t, fixture, idSuffix)

	assertSelectiveMetadataOnlyUpdate(t, fixture)
	assertOptionalHeaderRemovalAndRequiredHeaderRollback(t, fixture)
	assertSelectiveContentUpdate(t, fixture)
	assertNewDPPSectionAddition(t, &fixture)
	assertMappedDPPHeadersUpdateAASAndRegistries(t, fixture)
	assertContentSpecificationIdentityNormalization(t, fixture)
	assertNewDPPSectionCollisionRollsBack(t, fixture)
	assertRemovedDPPSectionRetainsSharedContent(t, fixture)
}

type selectiveDPPFixture struct {
	client                                 *http.Client
	baseURL, aasBaseURL                    string
	databasePort                           int
	aasID, dppID, encodedDPPID, metadataID string
	technicalDataID, carbonFootprintID     string
	collisionID, sharedAASID               string
	addedSpecificationID                   string
}

func newSelectiveDPPFixture(
	t *testing.T,
	client *http.Client,
	baseURL string,
	aasBaseURL string,
	databasePort int,
	idSuffix string,
	now time.Time,
) selectiveDPPFixture {
	t.Helper()
	fixture := selectiveDPPFixture{
		client:       client,
		baseURL:      baseURL,
		aasBaseURL:   aasBaseURL,
		databasePort: databasePort,
		aasID:        "https://www.example.org/aas/selective/" + idSuffix,
		dppID:        "https://www.example.org/dpp/selective/" + idSuffix,
	}
	fixture.encodedDPPID = encodedPathParam(fixture.dppID)
	productID := "https://www.example.org/product/selective/" + idSuffix
	doJSON(t, fixture.client, http.MethodPost, fixture.baseURL+"/v1/dpps", lifecycleDPPDocument(fixture.aasID, productID, now), http.StatusCreated)
	replaceDPPMetadataIdentifier(t, fixture.client, fixture.aasBaseURL, fixture.aasID+"/submodels/DppMetadata", fixture.dppID)
	fixture.technicalDataID = submodelIDBySemanticID(t, fixture.databasePort, fixture.aasID, lifecycleTechnicalDataSpec)
	fixture.carbonFootprintID = submodelIDBySemanticID(t, fixture.databasePort, fixture.aasID, lifecycleCarbonFootprintSpec)
	fixture.metadataID = fixture.aasID + "/submodels/DppMetadata"
	fixture.collisionID = strings.Replace(fixture.technicalDataID, fixture.aasID, fixture.dppID, 1)
	return fixture
}

func configureSelectiveUpdateFixtures(
	t *testing.T,
	fixture selectiveDPPFixture,
) {
	t.Helper()
	aas := doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shells/"+common.EncodeString(fixture.aasID), nil, http.StatusOK)
	aas["idShort"] = "SelectiveUpdateAAS"
	aas["extensions"] = []any{map[string]any{
		"name":      "selectiveUpdateExtension",
		"valueType": "xs:string",
		"value":     "preserve-me",
	}}
	doJSON(t, fixture.client, http.MethodPut, fixture.aasBaseURL+"/shells/"+common.EncodeString(fixture.aasID), aas, http.StatusNoContent)

	technicalData := doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(fixture.technicalDataID), nil, http.StatusOK)
	configureSelectiveUpdateProperty(t, technicalData)
	doJSON(t, fixture.client, http.MethodPut, fixture.aasBaseURL+"/submodels/"+common.EncodeString(fixture.technicalDataID), technicalData, http.StatusNoContent)

	collision := doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(fixture.technicalDataID), nil, http.StatusOK)
	collision["id"] = fixture.collisionID
	collision["idShort"] = "CollidingTechnicalData"
	setSubmodelPropertyValue(t, collision, "manufacturerName", "collision must survive")
	doJSON(t, fixture.client, http.MethodPost, fixture.aasBaseURL+"/submodels", collision, http.StatusCreated)
}

func configureSelectiveUpdateProperty(t *testing.T, submodel map[string]any) {
	t.Helper()
	property := submodelProperty(t, submodel, "manufacturerName")
	property["valueType"] = "xs:string"
	property["semanticId"] = map[string]any{
		"type": "ExternalReference",
		"keys": []any{map[string]any{
			"type":  "GlobalReference",
			"value": "urn:example:semantic:manufacturer-name",
		}},
	}
	property["qualifiers"] = []any{map[string]any{
		"kind":      "ConceptQualifier",
		"type":      "urn:example:qualifier:manufacturer-name",
		"valueType": "xs:string",
		"value":     "preserve-me",
	}}
}

func createSharedContentAAS(t *testing.T, fixture selectiveDPPFixture, idSuffix string) string {
	t.Helper()
	aasID := "https://www.example.org/aas/shared-content/" + idSuffix
	doJSON(t, fixture.client, http.MethodPost, fixture.aasBaseURL+"/shells", map[string]any{
		"id":        aasID,
		"idShort":   "SharedContentAAS",
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind": "Instance",
		},
		"submodels": []any{submodelReferencePayload(fixture.technicalDataID)},
	}, http.StatusCreated)
	return aasID
}

func assertSelectiveMetadataOnlyUpdate(t *testing.T, fixture selectiveDPPFixture) {
	t.Helper()
	before := selectiveUpdateSnapshot(t, fixture)
	body := doJSON(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		"dppStatus": "deprecated",
	}, http.StatusOK)
	assertJSONPathEquals(t, body, "digitalProductPassportId", fixture.dppID)
	assertJSONPathEquals(t, body, "dppStatus", "deprecated")
	assertDPPSectionPathEquals(t, body, lifecycleTechnicalDataSpec, "manufacturerName", "Acme GmbH")
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.metadataID, before.metadataRevision)
	assertSelectiveSnapshotUnchanged(t, fixture, before, true, true, true)
	assertAASReferencesSubmodel(t, fixture.client, fixture.aasBaseURL, fixture.sharedAASID, fixture.technicalDataID, true)
}

func assertSelectiveContentUpdate(t *testing.T, fixture selectiveDPPFixture) {
	t.Helper()
	before := selectiveUpdateSnapshot(t, fixture)
	body := doJSON(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		lifecycleTechnicalDataSpec: map[string]any{"manufacturerName": "Selective Update GmbH"},
	}, http.StatusOK)
	assertDPPSectionPathEquals(t, body, lifecycleTechnicalDataSpec, "manufacturerName", "Selective Update GmbH")
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.metadataID, before.metadataRevision)
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.technicalDataID, before.technicalDataRevision)
	assertSelectiveSnapshotUnchanged(t, fixture, before, false, true, true)
	technicalData := doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(fixture.technicalDataID), nil, http.StatusOK)
	property := submodelProperty(t, technicalData, "manufacturerName")
	assertJSONEquals(t, property["valueType"], "xs:string")
	assertJSONEquals(t, property["semanticId"], map[string]any{
		"type": "ExternalReference",
		"keys": []any{map[string]any{
			"type":  "GlobalReference",
			"value": "urn:example:semantic:manufacturer-name",
		}},
	})
	assertJSONEquals(t, property["qualifiers"], []any{map[string]any{
		"kind":      "ConceptQualifier",
		"type":      "urn:example:qualifier:manufacturer-name",
		"valueType": "xs:string",
		"value":     "preserve-me",
	}})
}

func assertNewDPPSectionAddition(t *testing.T, fixture *selectiveDPPFixture) {
	t.Helper()
	fixture.addedSpecificationID = "addedData"
	newSectionID := fixture.dppID + "/submodels/AddedData"
	before := selectiveUpdateSnapshot(t, *fixture)
	body := doJSON(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		"contentSpecificationIds": []string{lifecycleTechnicalDataSpec, lifecycleCarbonFootprintSpec, fixture.addedSpecificationID},
		fixture.addedSpecificationID: map[string]any{
			"newValue": "added without rewriting existing sections",
		},
	}, http.StatusOK)
	assertDPPSectionPathEquals(t, body, fixture.addedSpecificationID, "newValue", "added without rewriting existing sections")
	assertStringSliceContains(t, body["contentSpecificationIds"], fixture.addedSpecificationID)
	assertRevisionAdvanced(t, fixture.databasePort, "aas_history", fixture.aasID, before.aasRevision)
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.metadataID, before.metadataRevision)
	assertSelectiveSubmodelsUnchanged(t, *fixture, before, true, true, true)
	assertSubmodelIdentifierExists(t, fixture.databasePort, newSectionID, true)
	assertAASReferencesSubmodel(t, fixture.client, fixture.aasBaseURL, fixture.aasID, newSectionID, true)
	doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodel-descriptors/"+common.EncodeString(newSectionID), nil, http.StatusOK)
}

func assertMappedDPPHeadersUpdateAASAndRegistries(t *testing.T, fixture selectiveDPPFixture) {
	t.Helper()
	updatedProductID := "https://www.example.org/product/selective-updated/" + fixture.dppID
	before := selectiveUpdateSnapshot(t, fixture)
	body := doJSON(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		"uniqueProductIdentifier": updatedProductID,
		"granularity":             "Batch",
	}, http.StatusOK)
	assertJSONPathEquals(t, body, "uniqueProductIdentifier", updatedProductID)
	assertJSONPathEquals(t, body, "granularity", "Batch")
	assertRevisionAdvanced(t, fixture.databasePort, "aas_history", fixture.aasID, before.aasRevision)
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.metadataID, before.metadataRevision)
	assertSelectiveSubmodelsUnchanged(t, fixture, before, true, true, true)
	aas := doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shells/"+common.EncodeString(fixture.aasID), nil, http.StatusOK)
	assertJSONPathEquals(t, aas, "idShort", "SelectiveUpdateAAS")
	assertJSONEquals(t, aas["extensions"], []any{map[string]any{
		"name":      "selectiveUpdateExtension",
		"valueType": "xs:string",
		"value":     "preserve-me",
	}})
	assertJSONPathEquals(t, aas, "assetInformation.globalAssetId", updatedProductID)
	assertJSONPathEquals(t, aas, "assetInformation.assetKind", "Batch")
	descriptor := doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shell-descriptors/"+common.EncodeString(fixture.aasID), nil, http.StatusOK)
	assertJSONPathEquals(t, descriptor, "globalAssetId", updatedProductID)
	assertDiscoveryLink(t, fixture.client, fixture.aasBaseURL, fixture.aasID, updatedProductID)
}

func assertContentSpecificationIdentityNormalization(t *testing.T, fixture selectiveDPPFixture) {
	t.Helper()
	beforeClear := selectiveUpdateSnapshot(t, fixture)
	cleared := doJSON(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		"contentSpecificationIds": nil,
	}, http.StatusOK)
	assertJSONFieldMissing(t, cleared, "contentSpecificationIds")
	idShortSectionName := dppSectionNameForValue(t, cleared, "manufacturerName", "Selective Update GmbH")
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.metadataID, beforeClear.metadataRevision)
	assertSelectiveSnapshotUnchanged(t, fixture, beforeClear, true, true, true)

	beforeRestore := selectiveUpdateSnapshot(t, fixture)
	patch := map[string]any{
		"contentSpecificationIds": []string{lifecycleTechnicalDataSpec, lifecycleCarbonFootprintSpec, fixture.addedSpecificationID},
		lifecycleTechnicalDataSpec: map[string]any{
			"manufacturerName": "Normalized Technical Data GmbH",
		},
	}
	patch[idShortSectionName] = nil
	body := doJSON(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, patch, http.StatusOK)
	assertDPPSectionPathEquals(t, body, lifecycleTechnicalDataSpec, "manufacturerName", "Normalized Technical Data GmbH")
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.metadataID, beforeRestore.metadataRevision)
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.technicalDataID, beforeRestore.technicalDataRevision)
	assertRevisionUnchanged(t, fixture.databasePort, "aas_history", fixture.aasID, beforeRestore.aasRevision)
	assertJSONEquals(t, doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shells/"+common.EncodeString(fixture.aasID), nil, http.StatusOK), beforeRestore.aas)
	assertSelectiveSubmodelsUnchanged(t, fixture, beforeRestore, false, true, true)
	assertSubmodelIdentifierExists(t, fixture.databasePort, fixture.technicalDataID, true)
}

func dppSectionNameForValue(t *testing.T, body map[string]any, elementName string, expected string) string {
	t.Helper()
	for sectionName, rawSection := range body {
		section, ok := rawSection.(map[string]any)
		if !ok {
			continue
		}
		if value, err := valueAtPath(section, elementName); err == nil && value == expected {
			return sectionName
		}
	}
	t.Fatalf("DPP does not contain %s = %q: %#v", elementName, expected, body)
	return ""
}

func assertSelectiveSubmodelsUnchanged(
	t *testing.T,
	fixture selectiveDPPFixture,
	before selectiveUpdateState,
	assertTechnicalData bool,
	assertCarbonFootprint bool,
	assertCollision bool,
) {
	t.Helper()
	if assertTechnicalData {
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_history", submodelIdentifier(before.technicalData), before.technicalDataRevision)
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_descriptor_history", submodelIdentifier(before.technicalData), before.technicalDescriptorRevision)
		assertJSONEquals(t, doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(submodelIdentifier(before.technicalData)), nil, http.StatusOK), before.technicalData)
	}
	if assertCarbonFootprint {
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_history", submodelIdentifier(before.carbonFootprint), before.carbonRevision)
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_descriptor_history", submodelIdentifier(before.carbonFootprint), before.carbonDescriptorRevision)
		assertJSONEquals(t, doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(submodelIdentifier(before.carbonFootprint)), nil, http.StatusOK), before.carbonFootprint)
	}
	if assertCollision {
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_history", submodelIdentifier(before.collision), before.collisionRevision)
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_descriptor_history", submodelIdentifier(before.collision), before.collisionDescriptorRevision)
		assertJSONEquals(t, doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(submodelIdentifier(before.collision)), nil, http.StatusOK), before.collision)
	}
}

func assertDiscoveryLink(t *testing.T, client *http.Client, aasBaseURL string, aasID string, productID string) {
	t.Helper()
	links := doJSONAny(t, client, http.MethodGet, aasBaseURL+"/lookup/shells/"+common.EncodeString(aasID), nil, http.StatusOK)
	items, ok := links.([]any)
	if !ok {
		t.Fatalf("discovery links = %#v, want array", links)
	}
	for _, item := range items {
		link, ok := item.(map[string]any)
		if ok && link["name"] == "globalAssetId" && link["value"] == productID {
			return
		}
	}
	t.Fatalf("discovery links = %#v, want globalAssetId %q", links, productID)
}

func assertOptionalHeaderRemovalAndRequiredHeaderRollback(t *testing.T, fixture selectiveDPPFixture) {
	t.Helper()
	beforeOptionalRemoval := selectiveUpdateSnapshot(t, fixture)
	body := doJSON(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		"facilityId": nil,
	}, http.StatusOK)
	assertJSONFieldMissing(t, body, "facilityId")
	assertJSONPathEquals(t, body, "dppStatus", "deprecated")
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.metadataID, beforeOptionalRemoval.metadataRevision)
	assertSelectiveSnapshotUnchanged(t, fixture, beforeOptionalRemoval, true, true, true)

	beforeRequiredRemoval := selectiveUpdateSnapshot(t, fixture)
	doJSONAny(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		"dppStatus": nil,
	}, http.StatusBadRequest)
	assertSelectiveSnapshotUnchanged(t, fixture, beforeRequiredRemoval, true, true, true)
}

func assertNewDPPSectionCollisionRollsBack(t *testing.T, fixture selectiveDPPFixture) {
	t.Helper()
	newSpecificationID := "newData"
	newSectionID := fixture.dppID + "/submodels/NewData"
	newSection := map[string]any{
		"id":        newSectionID,
		"idShort":   "CollisionNewSection",
		"modelType": "Submodel",
		"submodelElements": []any{map[string]any{
			"idShort":   "collisionValue",
			"modelType": "Property",
			"valueType": "xs:string",
			"value":     "collision must survive",
		}},
	}
	doJSON(t, fixture.client, http.MethodPost, fixture.aasBaseURL+"/submodels", newSection, http.StatusCreated)
	before := selectiveUpdateSnapshot(t, fixture)
	beforeNewSection := submodelSnapshot(t, fixture, newSectionID)
	doJSONAny(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		"contentSpecificationIds": []string{lifecycleTechnicalDataSpec, lifecycleCarbonFootprintSpec, fixture.addedSpecificationID, newSpecificationID},
		newSpecificationID:        map[string]any{"newValue": "must roll back"},
	}, http.StatusConflict)
	assertSelectiveSnapshotUnchanged(t, fixture, before, true, true, true)
	assertSubmodelSnapshotUnchanged(t, fixture, beforeNewSection)
}

func assertRemovedDPPSectionRetainsSharedContent(t *testing.T, fixture selectiveDPPFixture) {
	t.Helper()
	beforeTechnicalData := submodelSnapshot(t, fixture, fixture.technicalDataID)
	beforeCollision := submodelSnapshot(t, fixture, fixture.collisionID)
	beforeAASRevision := historyRevision(t, fixture.databasePort, "aas_history", fixture.aasID)
	beforeMetadataRevision := historyRevision(t, fixture.databasePort, "submodel_history", fixture.metadataID)
	doJSON(t, fixture.client, http.MethodPatch, fixture.baseURL+"/v1/dpps/"+fixture.encodedDPPID, map[string]any{
		lifecycleTechnicalDataSpec: nil,
	}, http.StatusOK)
	assertRevisionAdvanced(t, fixture.databasePort, "aas_history", fixture.aasID, beforeAASRevision)
	assertRevisionAdvanced(t, fixture.databasePort, "submodel_history", fixture.metadataID, beforeMetadataRevision)
	assertSubmodelSnapshotUnchanged(t, fixture, beforeTechnicalData)
	assertSubmodelSnapshotUnchanged(t, fixture, beforeCollision)
	assertAASReferencesSubmodel(t, fixture.client, fixture.aasBaseURL, fixture.aasID, fixture.technicalDataID, false)
	assertAASReferencesSubmodel(t, fixture.client, fixture.aasBaseURL, fixture.sharedAASID, fixture.technicalDataID, true)
	doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodel-descriptors/"+common.EncodeString(fixture.technicalDataID), nil, http.StatusOK)
}

type selectiveUpdateState struct {
	aasRevision                 int64
	metadataRevision            int64
	technicalDataRevision       int64
	carbonRevision              int64
	collisionRevision           int64
	technicalDescriptorRevision int64
	carbonDescriptorRevision    int64
	collisionDescriptorRevision int64
	aas                         map[string]any
	technicalData               map[string]any
	carbonFootprint             map[string]any
	collision                   map[string]any
}

type persistedSubmodelSnapshot struct {
	id       string
	revision int64
	payload  map[string]any
}

func selectiveUpdateSnapshot(t *testing.T, fixture selectiveDPPFixture) selectiveUpdateState {
	t.Helper()
	return selectiveUpdateState{
		aasRevision:                 historyRevision(t, fixture.databasePort, "aas_history", fixture.aasID),
		metadataRevision:            historyRevision(t, fixture.databasePort, "submodel_history", fixture.metadataID),
		technicalDataRevision:       historyRevision(t, fixture.databasePort, "submodel_history", fixture.technicalDataID),
		carbonRevision:              historyRevision(t, fixture.databasePort, "submodel_history", fixture.carbonFootprintID),
		collisionRevision:           historyRevision(t, fixture.databasePort, "submodel_history", fixture.collisionID),
		technicalDescriptorRevision: historyRevision(t, fixture.databasePort, "submodel_descriptor_history", fixture.technicalDataID),
		carbonDescriptorRevision:    historyRevision(t, fixture.databasePort, "submodel_descriptor_history", fixture.carbonFootprintID),
		collisionDescriptorRevision: historyRevision(t, fixture.databasePort, "submodel_descriptor_history", fixture.collisionID),
		aas:                         doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shells/"+common.EncodeString(fixture.aasID), nil, http.StatusOK),
		technicalData:               doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(fixture.technicalDataID), nil, http.StatusOK),
		carbonFootprint:             doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(fixture.carbonFootprintID), nil, http.StatusOK),
		collision:                   doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(fixture.collisionID), nil, http.StatusOK),
	}
}

func submodelSnapshot(
	t *testing.T,
	fixture selectiveDPPFixture,
	submodelID string,
) persistedSubmodelSnapshot {
	t.Helper()
	return persistedSubmodelSnapshot{
		id:       submodelID,
		revision: historyRevision(t, fixture.databasePort, "submodel_history", submodelID),
		payload:  doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(submodelID), nil, http.StatusOK),
	}
}

func assertSelectiveSnapshotUnchanged(
	t *testing.T,
	fixture selectiveDPPFixture,
	before selectiveUpdateState,
	assertTechnicalData bool,
	assertCarbonFootprint bool,
	assertCollision bool,
) {
	t.Helper()
	assertRevisionUnchanged(t, fixture.databasePort, "aas_history", aasIdentifier(before.aas), before.aasRevision)
	assertJSONEquals(t, doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/shells/"+common.EncodeString(aasIdentifier(before.aas)), nil, http.StatusOK), before.aas)
	if assertTechnicalData {
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_history", submodelIdentifier(before.technicalData), before.technicalDataRevision)
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_descriptor_history", submodelIdentifier(before.technicalData), before.technicalDescriptorRevision)
		assertJSONEquals(t, doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(submodelIdentifier(before.technicalData)), nil, http.StatusOK), before.technicalData)
	}
	if assertCarbonFootprint {
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_history", submodelIdentifier(before.carbonFootprint), before.carbonRevision)
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_descriptor_history", submodelIdentifier(before.carbonFootprint), before.carbonDescriptorRevision)
		assertJSONEquals(t, doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(submodelIdentifier(before.carbonFootprint)), nil, http.StatusOK), before.carbonFootprint)
	}
	if assertCollision {
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_history", submodelIdentifier(before.collision), before.collisionRevision)
		assertRevisionUnchanged(t, fixture.databasePort, "submodel_descriptor_history", submodelIdentifier(before.collision), before.collisionDescriptorRevision)
		assertJSONEquals(t, doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(submodelIdentifier(before.collision)), nil, http.StatusOK), before.collision)
	}
}

func assertSubmodelSnapshotUnchanged(
	t *testing.T,
	fixture selectiveDPPFixture,
	before persistedSubmodelSnapshot,
) {
	t.Helper()
	assertRevisionUnchanged(t, fixture.databasePort, "submodel_history", before.id, before.revision)
	assertJSONEquals(t, doJSON(t, fixture.client, http.MethodGet, fixture.aasBaseURL+"/submodels/"+common.EncodeString(before.id), nil, http.StatusOK), before.payload)
}

func historyRevision(t *testing.T, databasePort int, table string, identifier string) int64 {
	t.Helper()
	if table != "aas_history" && table != "submodel_history" && table != "submodel_descriptor_history" {
		t.Fatalf("unsupported history table %q", table)
	}
	db := openDPPIntegrationDatabase(t, databasePort)
	defer func() { _ = db.Close() }()
	query, args, err := goqu.Dialect("postgres").
		From(goqu.T(table)).
		Select(goqu.Func("COALESCE", goqu.MAX("history_id"), 0)).
		Where(goqu.I("identifier").Eq(identifier)).
		ToSQL()
	if err != nil {
		t.Fatalf("build %s revision query: %v", table, err)
	}
	var revision int64
	if err = db.QueryRowContext(t.Context(), query, args...).Scan(&revision); err != nil {
		t.Fatalf("read %s revision for %q: %v", table, identifier, err)
	}
	return revision
}

func assertRevisionAdvanced(t *testing.T, databasePort int, table string, identifier string, before int64) {
	t.Helper()
	if actual := historyRevision(t, databasePort, table, identifier); actual <= before {
		t.Fatalf("%s revision for %q = %d, want greater than %d", table, identifier, actual, before)
	}
}

func assertRevisionUnchanged(t *testing.T, databasePort int, table string, identifier string, expected int64) {
	t.Helper()
	if actual := historyRevision(t, databasePort, table, identifier); actual != expected {
		t.Fatalf("%s revision for %q = %d, want %d", table, identifier, actual, expected)
	}
}

func assertAASReferencesSubmodel(t *testing.T, client *http.Client, aasBaseURL string, aasID string, submodelID string, expected bool) {
	t.Helper()
	aas := doJSON(t, client, http.MethodGet, aasBaseURL+"/shells/"+common.EncodeString(aasID), nil, http.StatusOK)
	actual := referencesSubmodel(aas, submodelID)
	if actual != expected {
		t.Fatalf("AAS %q reference to submodel %q = %t, want %t", aasID, submodelID, actual, expected)
	}
}

func referencesSubmodel(aas map[string]any, submodelID string) bool {
	references, ok := aas["submodels"].([]any)
	if !ok {
		return false
	}
	for _, reference := range references {
		if referenceMatchesSubmodel(reference, submodelID) {
			return true
		}
	}
	return false
}

func referenceMatchesSubmodel(reference any, submodelID string) bool {
	value, ok := reference.(map[string]any)
	if !ok {
		return false
	}
	keys, ok := value["keys"].([]any)
	if !ok || len(keys) == 0 {
		return false
	}
	lastKey, ok := keys[len(keys)-1].(map[string]any)
	return ok && lastKey["value"] == submodelID
}

func submodelReferencePayload(submodelID string) map[string]any {
	return map[string]any{
		"type": "ModelReference",
		"keys": []any{map[string]any{
			"type":  "Submodel",
			"value": submodelID,
		}},
	}
}

func submodelProperty(t *testing.T, submodel map[string]any, idShort string) map[string]any {
	t.Helper()
	elements, ok := submodel["submodelElements"].([]any)
	if !ok {
		t.Fatalf("submodelElements = %#v, want array", submodel["submodelElements"])
	}
	for _, item := range elements {
		property, ok := item.(map[string]any)
		if ok && property["idShort"] == idShort {
			return property
		}
	}
	t.Fatalf("submodelElements does not contain %q: %#v", idShort, elements)
	return nil
}

func setSubmodelPropertyValue(t *testing.T, submodel map[string]any, idShort string, value string) {
	t.Helper()
	submodelProperty(t, submodel, idShort)["value"] = value
}

func aasIdentifier(aas map[string]any) string {
	identifier, _ := aas["id"].(string)
	return identifier
}

func submodelIdentifier(submodel map[string]any) string {
	identifier, _ := submodel["id"].(string)
	return identifier
}

func assertJSONEquals(t *testing.T, actual any, expected any) {
	t.Helper()
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatalf("marshal actual JSON: %v", err)
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("marshal expected JSON: %v", err)
	}
	if !bytes.Equal(actualJSON, expectedJSON) {
		t.Fatalf("JSON = %s, want %s", actualJSON, expectedJSON)
	}
}
