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

package integration_tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func testDPPIdentifierInvariant(
	t *testing.T,
	client *http.Client,
	baseURL, aasBaseURL string,
	databasePort int,
	idSuffix string,
	now time.Time,
) {
	t.Helper()
	aasID := "https://www.example.org/aas/identity-owner/" + idSuffix
	claimedDPPID := "https://www.example.org/dpp/identity-claim/" + idSuffix
	productID := "https://www.example.org/product/identity/" + idSuffix
	doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", lifecycleDPPDocument(aasID, productID, now), http.StatusCreated)
	createdVersionDate := latestDPPHistoryTimestamp(t, databasePort, aasID)
	metadataID := aasID + "/submodels/DppMetadata"
	technicalDataID := submodelIDBySemanticID(t, databasePort, aasID, lifecycleTechnicalDataSpec)
	replaceDPPMetadataIdentifier(t, client, aasBaseURL, metadataID, claimedDPPID)
	mismatchedVersionDate := latestDPPHistoryTimestamp(t, databasePort, aasID)

	ownerPath := encodedPathParam(aasID)
	claimedPath := encodedPathParam(claimedDPPID)
	elementPath := encodedPathParam(dppElementJSONPath(lifecycleTechnicalDataSpec, "manufacturerName"))
	assertDPPIDRoutesNotFound(t, client, baseURL, ownerPath, elementPath)
	assertDPPIDRoutesNotFound(t, client, baseURL, claimedPath, elementPath)

	historical := doJSON(t, client, http.MethodGet, historyURL(baseURL, ownerPath, createdVersionDate, "compressed"), nil, http.StatusOK)
	assertJSONPathEquals(t, historical, "digitalProductPassportId", aasID)
	assertDPPSectionPathEquals(t, historical, lifecycleTechnicalDataSpec, "manufacturerName", "Acme GmbH")
	doJSONAny(t, client, http.MethodGet, historyURL(baseURL, ownerPath, mismatchedVersionDate, "compressed"), nil, http.StatusNotFound)
	doJSONAny(t, client, http.MethodGet, historyURL(baseURL, claimedPath, createdVersionDate, "compressed"), nil, http.StatusNotFound)

	productBody := doJSON(t, client, http.MethodGet, baseURL+"/v1/dppsByProductId/"+encodedPathParam(productID), nil, http.StatusOK)
	assertJSONPathEquals(t, productBody, "digitalProductPassportId", claimedDPPID)

	matchingProductID := productID + "/matching-owner"
	doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", lifecycleDPPDocument(claimedDPPID, matchingProductID, now.Add(time.Second)), http.StatusCreated)
	matchingBody := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+claimedPath, nil, http.StatusOK)
	assertJSONPathEquals(t, matchingBody, "digitalProductPassportId", claimedDPPID)
	originalProductBody := doJSON(t, client, http.MethodGet, baseURL+"/v1/dppsByProductId/"+encodedPathParam(productID), nil, http.StatusOK)
	assertJSONPathEquals(t, originalProductBody, "uniqueProductIdentifier", productID)
	doJSON(t, client, http.MethodDelete, baseURL+"/v1/dpps/"+claimedPath, nil, http.StatusNoContent)

	createUnrelatedAAS(t, client, aasBaseURL, claimedDPPID)
	assertDPPIDRoutesNotFound(t, client, baseURL, claimedPath, elementPath)
	doJSONAny(t, client, http.MethodPost, baseURL+"/v1/dpps", lifecycleDPPDocument(claimedDPPID, matchingProductID, now.Add(2*time.Second)), http.StatusConflict)
	doJSON(t, client, http.MethodGet, aasBaseURL+"/shells/"+common.EncodeString(aasID), nil, http.StatusOK)
	collision := doJSON(t, client, http.MethodGet, aasBaseURL+"/shells/"+common.EncodeString(claimedDPPID), nil, http.StatusOK)
	assertJSONPathEquals(t, collision, "idShort", "UnrelatedIdentityCollision")
	technicalData := doJSON(t, client, http.MethodGet, aasBaseURL+"/submodels/"+common.EncodeString(technicalDataID), nil, http.StatusOK)
	assertJSONEquals(t, submodelProperty(t, technicalData, "manufacturerName")["value"], "Acme GmbH")

	doJSON(t, client, http.MethodDelete, aasBaseURL+"/shells/"+common.EncodeString(aasID), nil, http.StatusNoContent)
	deletedOwnerHistory := doJSON(t, client, http.MethodGet, historyURL(baseURL, ownerPath, createdVersionDate, "compressed"), nil, http.StatusOK)
	assertJSONPathEquals(t, deletedOwnerHistory, "digitalProductPassportId", aasID)

	testMalformedProductMetadataIsNotDiscoverable(t, client, baseURL, aasBaseURL, idSuffix, now)
}

func assertDPPIDRoutesNotFound(t *testing.T, client *http.Client, baseURL, dppPath, elementPath string) {
	t.Helper()
	dppURL := baseURL + "/v1/dpps/" + dppPath
	doJSONAny(t, client, http.MethodGet, dppURL, nil, http.StatusNotFound)
	doJSONAny(t, client, http.MethodPatch, dppURL, map[string]any{"dppStatus": "deprecated"}, http.StatusNotFound)
	doJSONAny(t, client, http.MethodDelete, dppURL, nil, http.StatusNotFound)
	doJSONAny(t, client, http.MethodGet, dppURL+"/elements/"+elementPath, nil, http.StatusNotFound)
	doJSONAny(t, client, http.MethodPatch, dppURL+"/elements/"+elementPath, "must not be applied", http.StatusNotFound)
}

func createUnrelatedAAS(t *testing.T, client *http.Client, aasBaseURL, aasID string) {
	t.Helper()
	doJSON(t, client, http.MethodPost, aasBaseURL+"/shells", map[string]any{
		"id":        aasID,
		"idShort":   "UnrelatedIdentityCollision",
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind": "Instance",
		},
	}, http.StatusCreated)
}

func testMalformedProductMetadataIsNotDiscoverable(t *testing.T, client *http.Client, baseURL, aasBaseURL, idSuffix string, now time.Time) {
	t.Helper()
	dppID := "https://www.example.org/dpp/malformed-product-metadata/" + idSuffix
	productID := "https://www.example.org/product/malformed-product-metadata/" + idSuffix
	metadataID := dppID + "/submodels/DppMetadata"
	doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", lifecycleDPPDocument(dppID, productID, now), http.StatusCreated)

	replaceDPPMetadataIdentifier(t, client, aasBaseURL, metadataID, "")
	assertProductHasNoDPP(t, client, baseURL, productID)

	doJSONAny(t, client, http.MethodDelete, aasBaseURL+"/submodels/"+common.EncodeString(metadataID)+"/submodel-elements/digitalProductPassportId", nil, http.StatusNoContent)
	assertProductHasNoDPP(t, client, baseURL, productID)
}

func assertProductHasNoDPP(t *testing.T, client *http.Client, baseURL, productID string) {
	t.Helper()
	doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dppsByProductId/"+encodedPathParam(productID), nil, http.StatusNotFound)
	body := doJSON(t, client, http.MethodPost, baseURL+"/v1/dppsByProductIds", map[string]any{
		"productIds": []string{productID},
	}, http.StatusOK)
	items, ok := body["items"].([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("product lookup items = %#v, want empty array", body["items"])
	}
}
