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

//nolint:all
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

type cursorVisibilityPage struct {
	PagingMetadata struct {
		Cursor string `json:"cursor"`
	} `json:"paging_metadata"`
	Result []struct {
		ID string `json:"id"`
	} `json:"result"`
}

// TestSubmodelDescriptorListCursorSkipsHiddenDescriptors checks that the next
// cursor of an anonymous list never names a descriptor the caller may not read.
func TestSubmodelDescriptorListCursorSkipsHiddenDescriptors(t *testing.T) {
	provider := testenv.NewPasswordGrantTokenProvider(securityKeycloakTokenURL, "basyx-ui", 10*time.Second)
	token, err := provider.GetAccessToken(&testenv.TokenCredentials{User: "admin", Password: "pwd"})
	require.NoError(t, err)
	admin := map[string]string{"Authorization": "Bearer " + token}

	prefix := fmt.Sprintf("urn:cursor-visibility:%d", time.Now().UnixNano())
	publicFirst := prefix + ":a"
	hidden := prefix + ":b"
	publicLast := prefix + ":c"
	for id, semanticID := range map[string]string{publicFirst: "PUBLIC_READABLE", hidden: "HIDDEN", publicLast: "PUBLIC_READABLE"} {
		createCursorVisibilityDescriptor(t, admin, id, semanticID)
	}

	first := listCursorVisibilityPage(t, common.EncodeString(publicFirst))
	require.Len(t, first.Result, 1)
	require.Equal(t, publicFirst, first.Result[0].ID)
	require.Equal(t, common.EncodeString(publicLast), first.PagingMetadata.Cursor)

	second := listCursorVisibilityPage(t, first.PagingMetadata.Cursor)
	require.Len(t, second.Result, 1)
	require.Equal(t, publicLast, second.Result[0].ID)
}

func createCursorVisibilityDescriptor(t *testing.T, admin map[string]string, id string, semanticID string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id": id,
		"semanticId": map[string]any{
			"type": "ExternalReference",
			"keys": []any{map[string]any{"type": "GlobalReference", "value": semanticID}},
		},
		"endpoints": []any{map[string]any{
			"interface":           "SUBMODEL-3.0",
			"protocolInformation": map[string]any{"href": "https://example.org/submodels"},
		}},
	})
	require.NoError(t, err)
	created := testenv.DoHTTP(t, http.MethodPost, securityBaseURL+"/submodel-descriptors", body, admin)
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	t.Cleanup(func() {
		testenv.DoHTTP(t, http.MethodDelete, securityBaseURL+"/submodel-descriptors/"+common.EncodeString(id), nil, admin)
	})
}

func listCursorVisibilityPage(t *testing.T, cursor string) cursorVisibilityPage {
	t.Helper()
	parameters := url.Values{"limit": {"1"}, "cursor": {cursor}}
	response := testenv.DoHTTP(t, http.MethodGet, securityBaseURL+"/submodel-descriptors?"+parameters.Encode(), nil, nil)
	require.Equal(t, http.StatusOK, response.Status, string(response.Body))
	var page cursorVisibilityPage
	require.NoError(t, json.Unmarshal(response.Body, &page))
	return page
}
