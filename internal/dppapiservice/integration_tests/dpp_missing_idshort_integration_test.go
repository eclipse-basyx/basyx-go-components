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
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

// testDPPRejectsElementsWithoutIDShort expects 409 because DPP maps the 400
// from persistence to its fallback status; correcting that mapping is planned
// separately.
func testDPPRejectsElementsWithoutIDShort(
	t *testing.T,
	client *http.Client,
	baseURL, aasBaseURL, idSuffix string,
	now time.Time,
) {
	t.Helper()
	rejectedID := "https://www.example.org/dpp/missing-idshort/rejected/" + idSuffix
	rejectedDocument := lifecycleDPPDocument(rejectedID, rejectedID+"/product", now)
	lifecycleDimensions(rejectedDocument)[""] = 5
	requireMissingIDShortResponse(t, doJSONAny(t, client, http.MethodPost, baseURL+"/v1/dpps", rejectedDocument, http.StatusConflict))
	doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedPathParam(rejectedID), nil, http.StatusNotFound)

	dppID := "https://www.example.org/dpp/missing-idshort/" + idSuffix
	document := lifecycleDPPDocument(dppID, dppID+"/product", now)
	doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", document, http.StatusCreated)
	elementURL := baseURL + "/v1/dpps/" + encodedPathParam(dppID) + "/elements/" +
		encodedPathParam(dppElementJSONPath(lifecycleTechnicalDataSpec, "dimensions"))
	before := doJSONAny(t, client, http.MethodGet, elementURL, nil, http.StatusOK)

	namelessPatch := map[string]any{"": 5}
	for key, value := range lifecycleDimensions(document) {
		namelessPatch[key] = value
	}
	requireMissingIDShortResponse(t, doJSONAny(t, client, http.MethodPatch, elementURL, namelessPatch, http.StatusConflict))
	require.Equal(t, before, doJSONAny(t, client, http.MethodGet, elementURL, nil, http.StatusOK))

	deleteDPPWithContentSubmodels(t, client, baseURL, aasBaseURL, dppID)
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

func requireMissingIDShortResponse(t *testing.T, response any) {
	t.Helper()
	body, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(body), "-MISSINGIDSHORT")
}
