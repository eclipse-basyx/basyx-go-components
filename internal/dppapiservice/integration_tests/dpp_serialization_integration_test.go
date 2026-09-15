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

	"github.com/stretchr/testify/require"
)

func testDPPCollectionSerialization(t *testing.T, client *http.Client, baseURL, idSuffix string, now time.Time) {
	t.Helper()
	dppID := "urn:example:dpp:serialization:" + idSuffix
	document := lifecycleDPPDocument(dppID, dppID+":product", now)
	technical := document[lifecycleTechnicalDataSpec].(map[string]any)
	parts := []any{map[string]any{"partNumber": "A-1"}}
	classification := map[string]any{"value": "material", "contentType": "classification"}
	technical["parts"] = parts
	technical["classification"] = classification
	doJSON(t, client, http.MethodPost, baseURL+"/v1/dpps", document, http.StatusCreated)
	endpoint := baseURL + "/v1/dpps/" + encodedPathParam(dppID)
	body := doJSON(t, client, http.MethodGet, endpoint, nil, http.StatusOK)
	section := body[lifecycleTechnicalDataSpec].(map[string]any)
	require.Equal(t, parts, section["parts"])
	require.Equal(t, classification, section["classification"])
	elementEndpoint := endpoint + "/elements/" + encodedPathParam(dppElementJSONPath(lifecycleTechnicalDataSpec, "parts"))
	element := doJSONAny(t, client, http.MethodGet, elementEndpoint, nil, http.StatusOK)
	require.Equal(t, parts, element)

	replacement := []any{map[string]any{"partNumber": "B-2"}}
	patched := doJSON(t, client, http.MethodPatch, endpoint, map[string]any{
		lifecycleTechnicalDataSpec: map[string]any{"parts": replacement},
	}, http.StatusOK)
	patchedSection := patched[lifecycleTechnicalDataSpec].(map[string]any)
	require.Equal(t, replacement, patchedSection["parts"])
	require.Equal(t, classification, patchedSection["classification"])
	updatedElement := doJSONAny(t, client, http.MethodPatch, elementEndpoint, parts, http.StatusOK)
	require.Equal(t, parts, updatedElement)
	finalBody := doJSON(t, client, http.MethodGet, endpoint, nil, http.StatusOK)
	finalSection := finalBody[lifecycleTechnicalDataSpec].(map[string]any)
	require.Equal(t, parts, finalSection["parts"])
	require.Equal(t, classification, finalSection["classification"])
}
