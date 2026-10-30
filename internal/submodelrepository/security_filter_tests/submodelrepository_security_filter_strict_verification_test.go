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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

// nolint:all Is only a test file
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const hiddenListMemberSubmodelID = "urn:example:submodel:hidden-list-member"

func TestStrictVerificationChecksListMembersHiddenFromTheCaller(t *testing.T) {
	waitForStrictService(t)
	tokens := testenv.NewPasswordGrantTokenProvider(securityFilterKeycloakTokenURL, "basyx-ui", 10*time.Second)
	adminToken := accessToken(t, tokens, "admin")
	editorToken := accessToken(t, tokens, "userx")
	submodelURL := securityFilterStrictBaseURL + "/submodels/" + common.EncodeString(hiddenListMemberSubmodelID)
	listURL := submodelURL + "/submodel-elements/Readings"

	status, body := sendWithToken(t, http.MethodPost, securityFilterStrictBaseURL+"/submodels", adminToken, hiddenListMemberSubmodel())
	require.Equal(t, http.StatusCreated, status, body)
	t.Cleanup(func() { sendWithToken(t, http.MethodDelete, submodelURL, adminToken, nil) })

	status, body = sendWithToken(t, http.MethodGet, listURL, editorToken, nil)
	require.Equal(t, http.StatusOK, status, body)
	var editorView map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &editorView))
	visibleMembers, _ := editorView["value"].([]any)
	require.Empty(t, visibleMembers, "the list member must be hidden from the editor: %s", body)

	status, body = sendWithToken(t, http.MethodPatch, listURL+"/$metadata", editorToken, map[string]any{
		"modelType":            "SubmodelElementList",
		"valueTypeListElement": "xs:long",
	})
	require.Equal(t, http.StatusBadRequest, status, body)
	require.Contains(t, body, "violates metamodel constraints")
	require.NotContains(t, body, "xs:int")

	status, body = sendWithToken(t, http.MethodGet, listURL+"/$metadata", adminToken, nil)
	require.Equal(t, http.StatusOK, status, body)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &metadata))
	require.Equal(t, "xs:int", metadata["valueTypeListElement"])
}

func hiddenListMemberSubmodel() map[string]any {
	return map[string]any{
		"id":        hiddenListMemberSubmodelID,
		"idShort":   "HiddenListMember",
		"kind":      "Instance",
		"modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{
				"idShort":              "Readings",
				"modelType":            "SubmodelElementList",
				"semanticId":           map[string]any{"type": "ExternalReference", "keys": []any{map[string]any{"type": "GlobalReference", "value": "urn:example:semantic:visible-list"}}},
				"typeValueListElement": "Property",
				"valueTypeListElement": "xs:int",
				"value":                []any{map[string]any{"modelType": "Property", "valueType": "xs:int", "value": "1"}},
			},
		},
	}
}

func waitForStrictService(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(securityFilterStrictBaseURL + "/health")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("strict verification service did not become healthy")
}

func accessToken(t *testing.T, tokens *testenv.PasswordGrantTokenProvider, user string) string {
	t.Helper()
	token, err := tokens.GetAccessToken(&testenv.TokenCredentials{User: user, Password: "pwd"})
	require.NoError(t, err)
	require.NotEmpty(t, token)
	return token
}

func sendWithToken(t *testing.T, method string, endpoint string, token string, payload any) (int, string) {
	t.Helper()
	var requestBody io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, endpoint, requestBody)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, string(responseBody)
}
