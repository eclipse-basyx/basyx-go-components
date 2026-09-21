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
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func testDPPAttachmentAndAASHistory(t *testing.T, client *http.Client, baseURL, aasBaseURL string, databasePort int, idSuffix string, now time.Time) {
	t.Helper()
	dppID := "https://www.example.org/dpp/attachment-history/" + idSuffix
	productID := "https://www.example.org/product/attachment-history/" + idSuffix
	dppPath := encodedPathParam(dppID)
	document := lifecycleDPPDocument(dppID, productID, now)
	doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", document, http.StatusCreated)

	submodelID := submodelIDBySemanticID(t, databasePort, dppID, lifecycleTechnicalDataSpec)
	attachmentURL := fmt.Sprintf("%s/submodels/%s/submodel-elements/manual/attachment", aasBaseURL, common.EncodeString(submodelID))
	firstBytes := []byte("first attachment bytes")
	secondBytes := []byte("replacement attachment bytes")
	uploadAttachment(t, client, attachmentURL, "manual.txt", firstBytes)
	assertAttachmentDownload(t, client, attachmentURL, firstBytes)
	beforeReplacement := latestDPPHistoryTimestamp(t, databasePort, dppID)
	uploadAttachment(t, client, attachmentURL, "manual.txt", secondBytes)
	currentAfterReplacement := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+dppPath, nil, http.StatusOK)
	assertDPPSectionPathEquals(t, currentAfterReplacement, lifecycleTechnicalDataSpec, "manual.url", attachmentURL)
	historical := doJSON(t, client, http.MethodGet, historyURL(baseURL, dppPath, beforeReplacement, "compressed"), nil, http.StatusOK)
	assertDPPSectionPathEquals(t, historical, lifecycleTechnicalDataSpec, "manual.url", attachmentURL)
	assertAttachmentDownload(t, client, attachmentURL, secondBytes)

	lastUpdate := currentAfterReplacement["lastUpdate"]
	beforeAAS := latestDPPHistoryTimestamp(t, databasePort, dppID)
	elementURL := aasBaseURL + "/submodels/" + common.EncodeString(submodelID) + "/submodel-elements/manufacturerName/$value"
	doJSONAny(t, client, http.MethodPatch, elementURL, "Direct AAS Update GmbH", http.StatusNoContent)
	currentAfterAAS := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+dppPath, nil, http.StatusOK)
	assertDPPSectionPathEquals(t, currentAfterAAS, lifecycleTechnicalDataSpec, "manufacturerName", "Direct AAS Update GmbH")
	if currentAfterAAS["lastUpdate"] != lastUpdate {
		t.Fatalf("lastUpdate changed after direct AAS edit: before=%#v after=%#v", lastUpdate, currentAfterAAS["lastUpdate"])
	}
	beforeAASBody := doJSON(t, client, http.MethodGet, historyURL(baseURL, dppPath, beforeAAS, "compressed"), nil, http.StatusOK)
	assertDPPSectionPathEquals(t, beforeAASBody, lifecycleTechnicalDataSpec, "manufacturerName", "Acme GmbH")
	latestAASBody := doJSON(t, client, http.MethodGet, historyURL(baseURL, dppPath, latestDPPHistoryTimestamp(t, databasePort, dppID), "compressed"), nil, http.StatusOK)
	assertDPPSectionPathEquals(t, latestAASBody, lifecycleTechnicalDataSpec, "manufacturerName", "Direct AAS Update GmbH")

	doJSON(t, client, http.MethodDelete, baseURL+"/v1/dpps/"+dppPath, nil, http.StatusNoContent)
}
