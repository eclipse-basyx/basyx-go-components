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

package testenv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

// RunAASReferenceTimeConformance verifies shell-reference timestamp filters and pagination.
func RunAASReferenceTimeConformance(t *testing.T, baseURL string) {
	t.Helper()

	idShort := fmt.Sprintf("ReferenceTimes%d", time.Now().UnixNano())
	prefix := "urn:conformance:aas:" + idShort
	assetID := prefix + ":asset"
	ids := []string{prefix + ":0", prefix + ":1", prefix + ":2"}
	createAASTimeFixture(t, baseURL, ids[0], idShort, assetID, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	createAASTimeFixture(t, baseURL, ids[1], idShort, assetID, "2026-01-03T00:00:00Z", "2026-01-03T00:00:00Z")
	createAASTimeFixture(t, baseURL, ids[2], idShort, assetID, "2026-01-01T00:00:00Z", "2026-01-04T00:00:00Z")
	createAASTimeFixture(t, baseURL, prefix+":other", idShort+"Other", assetID+":other", "2026-01-04T00:00:00Z", "2026-01-04T00:00:00Z")
	assetSelector, err := json.Marshal(map[string]string{"name": "globalAssetId", "value": assetID})
	require.NoError(t, err)

	for _, test := range []struct {
		name        string
		createdFrom string
		updatedFrom string
		want        []string
	}{
		{name: "unfiltered", want: ids},
		{name: "created_inclusive", createdFrom: "2026-01-03T00:00:00Z", want: ids[1:2]},
		{name: "updated_inclusive", updatedFrom: "2026-01-04T00:00:00Z", want: ids[2:]},
		{name: "created_one_nanosecond_later", createdFrom: "2026-01-03T00:00:00.000000001Z", want: []string{}},
		{name: "updated_one_nanosecond_later", updatedFrom: "2026-01-04T00:00:00.000000001Z", want: []string{}},
		{name: "created_excluded", createdFrom: "2100-01-01T00:00:00Z", want: []string{}},
		{name: "updated_excluded", updatedFrom: "2100-01-01T00:00:00Z", want: []string{}},
		{name: "or_filters", createdFrom: "2026-01-03T00:00:00Z", updatedFrom: "2026-01-04T00:00:00Z", want: ids[1:]},
		{name: "year_one_or_future", createdFrom: "0001-01-01T00:00:00Z", updatedFrom: "2100-01-01T00:00:00Z", want: ids},
		{name: "utc_offset", createdFrom: "2026-01-03T02:00:00+02:00", want: ids[1:2]},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := url.Values{"idShort": {idShort}, "assetIds": {common.EncodeString(string(assetSelector))}, "limit": {"1"}}
			if test.createdFrom != "" {
				query.Set("createdFrom", test.createdFrom)
			}
			if test.updatedFrom != "" {
				query.Set("updatedFrom", test.updatedFrom)
			}
			requireAASReferenceTimePages(t, baseURL+"/shells/$reference", query, test.want)
		})
	}
	for _, parameter := range []string{"createdFrom", "updatedFrom"} {
		t.Run("invalid_"+parameter, func(t *testing.T) {
			query := url.Values{"idShort": {idShort}, parameter: {"not-a-date"}}
			conformanceRequest(t, http.MethodGet, baseURL+"/shells/$reference?"+query.Encode(), nil, http.StatusBadRequest)
		})
	}
	t.Run("fractional_payload_timestamps", func(t *testing.T) {
		runAASFractionalTimestampConformance(t, baseURL, idShort+"Fractional", prefix+":fractional", assetID)
	})
}

func runAASFractionalTimestampConformance(t *testing.T, baseURL, idShort, id, assetID string) {
	t.Helper()
	createAASTimeFixture(t, baseURL, id, idShort, assetID, "2026-01-03T00:00:00.0000001001Z", "2026-01-03T00:00:00.999999900100Z")
	for _, endpoint := range []string{"/shells", "/shells/$reference"} {
		for _, test := range []struct {
			name, parameter, cutoff string
			count                   int
		}{
			{name: "created_equal", parameter: "createdFrom", cutoff: "2026-01-03T02:00:00.000000100100+02:00", count: 1},
			{name: "created_earlier_subnanosecond", parameter: "createdFrom", cutoff: "2026-01-03T00:00:00.00000010001Z", count: 1},
			{name: "created_later_subnanosecond", parameter: "createdFrom", cutoff: "2026-01-03T00:00:00.0000001002Z"},
			{name: "created_one_nanosecond_later", parameter: "createdFrom", cutoff: "2026-01-03T00:00:00.000000101Z"},
			{name: "updated_equal", parameter: "updatedFrom", cutoff: "2026-01-03T00:00:00.9999999001Z", count: 1},
			{name: "updated_later_subnanosecond", parameter: "updatedFrom", cutoff: "2026-01-03T00:00:00.9999999002Z"},
			{name: "updated_next_second", parameter: "updatedFrom", cutoff: "2026-01-03T00:00:01Z"},
		} {
			t.Run(endpoint+"/"+test.name, func(t *testing.T) {
				query := url.Values{"idShort": {idShort}, "limit": {"1"}, test.parameter: {test.cutoff}}
				page := conformanceRequest(t, http.MethodGet, baseURL+endpoint+"?"+query.Encode(), nil, http.StatusOK)
				require.Len(t, page["result"], test.count)
			})
		}
	}
}

func createAASTimeFixture(t *testing.T, baseURL, id, idShort, assetID, createdAt, updatedAt string) {
	t.Helper()
	conformanceRequest(t, http.MethodPost, baseURL+"/shells", map[string]any{
		"modelType": "AssetAdministrationShell",
		"id":        id,
		"idShort":   idShort,
		"administration": map[string]any{
			"createdAt": createdAt,
			"updatedAt": updatedAt,
		},
		"assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": assetID},
	}, http.StatusCreated)
	t.Cleanup(func() {
		conformanceRequest(t, http.MethodDelete, baseURL+"/shells/"+common.EncodeString(id), nil, http.StatusNoContent)
	})
}

func requireAASReferenceTimePages(t *testing.T, endpoint string, query url.Values, expected []string) {
	t.Helper()
	collected := []string{}
	for pageCount := 0; pageCount <= len(expected); pageCount++ {
		page := conformanceRequest(t, http.MethodGet, endpoint+"?"+query.Encode(), nil, http.StatusOK)
		result, ok := page["result"].([]any)
		require.True(t, ok, "result must be an array: %v", page)
		require.LessOrEqual(t, len(result), 1)
		for _, item := range result {
			reference, ok := item.(map[string]any)
			require.True(t, ok)
			require.Equal(t, "ModelReference", reference["type"])
			keys, ok := reference["keys"].([]any)
			require.True(t, ok)
			require.Len(t, keys, 1)
			key, ok := keys[0].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "AssetAdministrationShell", key["type"])
			id, ok := key["value"].(string)
			require.True(t, ok)
			collected = append(collected, id)
		}
		metadata, ok := page["paging_metadata"].(map[string]any)
		require.True(t, ok)
		cursor, _ := metadata["cursor"].(string)
		if cursor == "" {
			require.Equal(t, expected, collected)
			return
		}
		require.Len(t, result, 1, "intermediate page must be full")
		query.Set("cursor", cursor)
	}
	t.Fatal("shell-reference pagination did not terminate")
}
