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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type aasDescriptorQueryPage struct {
	PagingMetadata struct {
		Cursor string `json:"cursor"`
	} `json:"paging_metadata"`
	Result []struct {
		ID string `json:"id"`
	} `json:"result"`
}

func TestQueryShellDescriptorsFollowsCursorPages(t *testing.T) {
	idShort, descriptorIDs := createQueryPaginationDescriptors(t, 5)
	query := queryPaginationIDShortCondition(idShort)

	for _, limit := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			require.Equal(t, descriptorIDs, collectQueriedShellDescriptorIDs(t, query, limit))
		})
	}
}

func TestQueryShellDescriptorsStartsAtClientCursor(t *testing.T) {
	idShort, descriptorIDs := createQueryPaginationDescriptors(t, 5)
	cursor := base64.RawURLEncoding.EncodeToString([]byte(descriptorIDs[2]))

	page := queryShellDescriptorPage(t, queryPaginationIDShortCondition(idShort), 10, cursor)

	require.Equal(t, descriptorIDs[2:], queryPageIDs(page))
	require.Empty(t, page.PagingMetadata.Cursor)
}

func createQueryPaginationDescriptors(t *testing.T, count int) (string, []string) {
	t.Helper()
	runID := time.Now().UnixNano()
	idShort := fmt.Sprintf("QueryPaging%d", runID)
	descriptorIDs := make([]string, 0, count)
	for index := 1; index <= count; index++ {
		descriptorID := fmt.Sprintf("https://example.com/ids/aasdesc/query-paging-%d-%d", runID, index)
		createAASDescriptor(t, map[string]any{"id": descriptorID, "idShort": idShort}, http.StatusCreated)
		t.Cleanup(func() { cleanupAASDescriptor(t, aasRegistryBaseURL, descriptorID) })
		descriptorIDs = append(descriptorIDs, descriptorID)
	}
	return idShort, descriptorIDs
}

func queryPaginationIDShortCondition(idShort string) map[string]any {
	return map[string]any{
		"$condition": map[string]any{
			"$eq": []any{
				map[string]any{"$field": "$aasdesc#idShort"},
				map[string]any{"$strVal": idShort},
			},
		},
	}
}

func collectQueriedShellDescriptorIDs(t *testing.T, query map[string]any, limit int) []string {
	t.Helper()
	var collected []string
	cursor := ""
	for pageCount := 0; pageCount <= 10; pageCount++ {
		page := queryShellDescriptorPage(t, query, limit, cursor)
		require.NotEmpty(t, page.Result, "page after cursor %q must not be empty", cursor)
		collected = append(collected, queryPageIDs(page)...)
		cursor = page.PagingMetadata.Cursor
		if cursor == "" {
			return collected
		}
	}
	t.Fatalf("AASREG-QUERYPAGING-COLLECT-TOOMANYPAGES limit=%d", limit)
	return nil
}

func queryShellDescriptorPage(t *testing.T, query map[string]any, limit int, cursor string) aasDescriptorQueryPage {
	t.Helper()
	parameters := url.Values{"limit": []string{fmt.Sprint(limit)}}
	if cursor != "" {
		parameters.Set("cursor", cursor)
	}
	status, body, _ := doAASRequest(t, aasNoRedirectClient, http.MethodPost, aasRegistryBaseURL+"/query/shell-descriptors?"+parameters.Encode(), query)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	var page aasDescriptorQueryPage
	require.NoError(t, json.Unmarshal(body, &page))
	return page
}

func queryPageIDs(page aasDescriptorQueryPage) []string {
	ids := make([]string, 0, len(page.Result))
	for _, item := range page.Result {
		ids = append(ids, item.ID)
	}
	return ids
}
