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
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

type dppInvalidElementKey struct {
	key       string
	errorCode string
}

// testDPPRejectsElementsWithoutValidIDShort covers element JSON keys, which
// become idShorts. It expects 409 because DPP maps the 400 from persistence to
// its fallback status.
func testDPPRejectsElementsWithoutValidIDShort(
	t *testing.T,
	client *http.Client,
	baseURL, aasBaseURL, idSuffix string,
	now time.Time,
) {
	t.Helper()
	invalidKeys := []dppInvalidElementKey{
		{key: "", errorCode: "-MISSINGIDSHORT"},
		{key: "outer width", errorCode: "-INVALIDIDSHORT"},
		{key: "w", errorCode: "-INVALIDIDSHORT"},
	}
	for index, invalidKey := range invalidKeys {
		rejectedID := "https://www.example.org/dpp/invalid-idshort/rejected/" + strconv.Itoa(index) + "/" + idSuffix
		rejectedDocument := lifecycleDPPDocument(rejectedID, rejectedID+"/product", now)
		lifecycleDimensions(rejectedDocument)[invalidKey.key] = 5
		requireDPPErrorCode(t, doJSONAny(t, client, http.MethodPost, baseURL+"/v1/dpps", rejectedDocument, http.StatusConflict), invalidKey.errorCode)
		doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedPathParam(rejectedID), nil, http.StatusNotFound)
	}

	testDPPRejectsSectionWithoutValidIDShort(t, client, baseURL, idSuffix, now)

	dppID := "https://www.example.org/dpp/invalid-idshort/" + idSuffix
	document := lifecycleDPPDocument(dppID, dppID+"/product", now)
	doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", document, http.StatusCreated)
	elementURL := baseURL + "/v1/dpps/" + encodedPathParam(dppID) + "/elements/" +
		encodedPathParam(dppElementJSONPath(lifecycleTechnicalDataSpec, "dimensions"))
	before := doJSONAny(t, client, http.MethodGet, elementURL, nil, http.StatusOK)

	for _, invalidKey := range invalidKeys {
		patch := map[string]any{invalidKey.key: 5}
		for key, value := range lifecycleDimensions(document) {
			patch[key] = value
		}
		requireDPPErrorCode(t, doJSONAny(t, client, http.MethodPatch, elementURL, patch, http.StatusConflict), invalidKey.errorCode)
	}
	require.Equal(t, before, doJSONAny(t, client, http.MethodGet, elementURL, nil, http.StatusOK))

	deleteDPPWithContentSubmodels(t, client, baseURL, aasBaseURL, dppID)
}

// testDPPRejectsSectionWithoutValidIDShort covers a section whose name gives
// its content Submodel an invalid idShort: DPP rejects it before writing.
func testDPPRejectsSectionWithoutValidIDShort(t *testing.T, client *http.Client, baseURL, idSuffix string, now time.Time) {
	t.Helper()
	const invalidSectionSpec = "urn:example:semantic:1a"
	dppID := "https://www.example.org/dpp/invalid-idshort/section/" + idSuffix
	document := lifecycleDPPDocument(dppID, dppID+"/product", now)
	document["contentSpecificationIds"] = append(document["contentSpecificationIds"].([]string), invalidSectionSpec)
	document[invalidSectionSpec] = map[string]any{"remark": "rejected"}
	requireDPPErrorCode(t, doJSONAny(t, client, http.MethodPost, baseURL+"/v1/dpps", document, http.StatusBadRequest), "DPP-BUILDSM-SECTIONIDSHORT")
	doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedPathParam(dppID), nil, http.StatusNotFound)
}

// deleteDPPWithContentSubmodels also deletes the content submodels, which the
// DPP delete keeps.
func deleteDPPWithContentSubmodels(t *testing.T, client *http.Client, baseURL, aasBaseURL, dppID string) {
	t.Helper()
	submodelIDs := shellSubmodelIDs(t, client, aasBaseURL, dppID)
	doJSON(t, client, http.MethodDelete, baseURL+"/v1/dpps/"+encodedPathParam(dppID), nil, http.StatusNoContent)
	for _, submodelID := range submodelIDs {
		if submodelID == dppID+"/submodels/DppMetadata" {
			continue
		}
		doJSON(t, client, http.MethodDelete, aasBaseURL+"/submodels/"+common.EncodeString(submodelID), nil, http.StatusNoContent)
	}
}

func shellSubmodelIDs(t *testing.T, client *http.Client, aasBaseURL, aasID string) []string {
	t.Helper()
	shell := doJSON(t, client, http.MethodGet, aasBaseURL+"/shells/"+common.EncodeString(aasID), nil, http.StatusOK)
	references, ok := shell["submodels"].([]any)
	require.True(t, ok, "AAS %q submodels = %#v, want array", aasID, shell["submodels"])
	submodelIDs := make([]string, 0, len(references))
	for _, reference := range references {
		keys, _ := reference.(map[string]any)["keys"].([]any)
		require.Len(t, keys, 1, "submodel reference = %#v", reference)
		value, _ := keys[0].(map[string]any)["value"].(string)
		submodelIDs = append(submodelIDs, value)
	}
	return submodelIDs
}

func lifecycleDimensions(document map[string]any) map[string]any {
	return document[lifecycleTechnicalDataSpec].(map[string]any)["dimensions"].(map[string]any)
}

func requireDPPErrorCode(t *testing.T, response any, errorCode string) {
	t.Helper()
	body, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(body), errorCode)
}
