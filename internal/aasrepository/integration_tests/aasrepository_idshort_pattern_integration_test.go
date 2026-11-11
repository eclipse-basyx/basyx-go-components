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

package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAssetAdministrationShellRejectsInvalidIDShort(t *testing.T) {
	createdID := "urn:basyx:integration:aas-invalid-idshort:" + uuid.NewString()
	createdEndpoint := aasRepositoryBaseURL + "/shells/" + base64.RawURLEncoding.EncodeToString([]byte(createdID))
	t.Cleanup(func() { deleteAASForLargeObjectCleanupTest(t, createdEndpoint) })
	requireAASIDShortRejected(t, http.MethodPost, aasRepositoryBaseURL+"/shells", shellWithIDShort(createdID, "Shell;1"))
	requireAASIDShortRejected(t, http.MethodPut, createdEndpoint, shellWithIDShort(createdID, "S"))
	requireAASIDShortRejected(t, http.MethodPost, aasRepositoryBaseURL+"/shells", shellWithIDShort(createdID, ""))
	requireAASIDShortRejected(t, http.MethodPut, createdEndpoint, shellWithIDShort(createdID, ""))
	requireAASStatus(t, createdEndpoint, http.StatusNotFound)

	_, status, _, err := postJSONResponse(aasRepositoryBaseURL+"/shells", mustMarshalShell(t, shellWithIDShort(createdID, "ValidShell")))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	requireAASIDShortRejected(t, http.MethodPut, createdEndpoint, shellWithIDShort(createdID, "Shell Name"))
	requireAASIDShortRejected(t, http.MethodPut, createdEndpoint, shellWithIDShort(createdID, ""))
	stored := requireAASStatus(t, createdEndpoint, http.StatusOK)
	require.Equal(t, "ValidShell", stored["idShort"])
}

func requireAASIDShortRejected(t *testing.T, method string, endpoint string, shell map[string]any) {
	t.Helper()
	send := postJSONResponse
	if method == http.MethodPut {
		send = putJSONResponse
	}
	response, status, _, err := send(endpoint, mustMarshalShell(t, shell))
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, status, "%s %s: %v", method, endpoint, response)
	body, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(body), "-INVALIDIDSHORT")
}

func requireAASStatus(t *testing.T, endpoint string, expectedStatus int) map[string]any {
	t.Helper()
	response, status, err := getJSONResponse(endpoint)
	require.NoError(t, err)
	require.Equal(t, expectedStatus, status, "GET %s: %v", endpoint, response)
	return response
}

func shellWithIDShort(id string, idShort string) map[string]any {
	return map[string]any{
		"modelType":        "AssetAdministrationShell",
		"id":               id,
		"idShort":          idShort,
		"assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": id + ":asset"},
	}
}

func mustMarshalShell(t *testing.T, shell map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(shell)
	require.NoError(t, err)
	return string(encoded)
}
