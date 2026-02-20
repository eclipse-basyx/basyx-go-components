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
package bench

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscovery_Suite_Sophisticated(t *testing.T) {
	rc := NewRequestClient()

	aasA := "urn:aas:test:assembler-1"
	aasB := "urn:aas:test:oil-refinery"
	aasC := "urn:aas:test:rail-signal"

	linksA1 := make([]types.ISpecificAssetID, 3)
	linksA1[0] = types.NewSpecificAssetID("globalAssetId", "urn:ga:green-circuit")
	linksA1[1] = types.NewSpecificAssetID("serialNumber", "SN-iron-gear")
	linksA1[2] = types.NewSpecificAssetID("plant", "NAUVIS")
	linksA2 := make([]types.ISpecificAssetID, 3)
	linksA2[0] = types.NewSpecificAssetID("globalAssetId", linksA1[0].Value())
	linksA2[1] = types.NewSpecificAssetID("serialNumber", "SN-red-circuit")
	linksA2[2] = types.NewSpecificAssetID("line", "L1")
	linksB := make([]types.ISpecificAssetID, 2)
	linksB[0] = types.NewSpecificAssetID("serialNumber", "SN-engine-unit")
	linksB[1] = types.NewSpecificAssetID("plant", "SPIDERTRON-YARD")
	linksC := make([]types.ISpecificAssetID, 1)
	linksC[0] = types.NewSpecificAssetID("assetTag", "belt-yellow")

	t.Run("LookupShellsByAssetLink/Pagination_empty_set_returns_empty_and_no_cursor", func(t *testing.T) {
		res := rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{}, 5, "", http.StatusOK)
		assert.Empty(t, res.Result, "empty dataset should yield no AAS IDs; got=%v", res.Result)
		assert.Empty(t, res.PagingMetadata.Cursor, "empty dataset should not produce a cursor; got=%q", res.PagingMetadata.Cursor)
	})

	t.Run("LookupShells/POST_then_replace_and_validate_via_GET", func(t *testing.T) {
		rc.PostLookupShells(t, aasA, linksA1)
		got := rc.GetLookupShells(t, aasA, http.StatusOK)

		wantMap1 := map[string][]string{
			"globalAssetId": {linksA1[0].Value()},
			"serialNumber":  {linksA1[1].Value()},
			"plant":         {"NAUVIS"},
		}
		ensureContainsAll(t, got, wantMap1)

		rc.PostLookupShells(t, aasA, linksA2)
		got2 := rc.GetLookupShells(t, aasA, http.StatusOK)

		wantMap2 := map[string][]string{
			"globalAssetId": {linksA2[0].Value()},
			"serialNumber":  {linksA2[1].Value()},
			"line":          {"L1"},
		}
		ensureContainsAll(t, got2, wantMap2)
		assertNoNames(t, got2, "plant")
	})

	t.Run("LookupShells/POST_create_multiple_AAS", func(t *testing.T) {
		rc.PostLookupShells(t, aasB, linksB)
		rc.PostLookupShells(t, aasC, linksC)

		gotB := rc.GetLookupShells(t, aasB, http.StatusOK)
		ensureContainsAll(t, gotB, map[string][]string{
			"serialNumber": {linksB[0].Value()},
			"plant":        {"SPIDERTRON-YARD"},
		})

		gotC := rc.GetLookupShells(t, aasC, http.StatusOK)
		ensureContainsAll(t, gotC, map[string][]string{
			"assetTag": {linksC[0].Value()},
		})
	})

	t.Run("LookupShellsByAssetLink/Search_matrix", func(t *testing.T) {
		said := types.NewSpecificAssetID("globalAssetId", linksA2[0].Value())
		res := rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{said}, 10, "", http.StatusOK)
		assert.Contains(t, res.Result, aasA)
		assert.NotContains(t, res.Result, aasB)
		assert.NotContains(t, res.Result, aasC)

		res = rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID("serialNumber", linksA2[1].Value())}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasA}, res.Result)

		res = rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID("plant", "SPIDERTRON-YARD")}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasB}, res.Result)

		res = rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{
			types.NewSpecificAssetID("globalAssetId", linksA2[0].Value()),
			types.NewSpecificAssetID("serialNumber", linksA2[1].Value()),
		}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasA}, res.Result)

		res = rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID("serialNumber", "SN-does-not-exist")}, 10, "", http.StatusOK)
		assert.Len(t, res.Result, 0)
	})

	t.Run("LookupShells/DELETE_and_absent_in_search", func(t *testing.T) {
		_ = rc.GetLookupShells(t, aasA, http.StatusOK)

		rc.DeleteLookupShells(t, aasA)
		_ = rc.GetLookupShells(t, aasA, http.StatusNotFound)

		res := rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID("globalAssetId", linksA2[0].Value())}, 10, "", http.StatusOK)
		assert.NotContains(t, res.Result, aasA)
	})

	t.Run("LookupShells/POST_two_AAS_with_shared_and_unique_pairs", func(t *testing.T) {
		aasD := "urn:aas:test:copper-plate"
		aasE := "urn:aas:test:iron-gear"

		shared := types.NewSpecificAssetID("sharedTag", "train-signal")
		uniqD := types.NewSpecificAssetID("uniqueD", "uranium-fuel-cell")
		uniqE := types.NewSpecificAssetID("uniqueE", "rocket-control-unit")

		rc.PostLookupShells(t, aasD, []types.ISpecificAssetID{shared, uniqD})
		rc.PostLookupShells(t, aasE, []types.ISpecificAssetID{shared, uniqE})

		resShared := rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID(shared.Name(), shared.Value())}, 10, "", http.StatusOK)
		assert.Contains(t, resShared.Result, aasD)
		assert.Contains(t, resShared.Result, aasE)
		assert.Len(t, resShared.Result, 2)

		resUniqD := rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID(uniqD.Name(), uniqD.Value())}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasD}, resUniqD.Result)

		resUniqE := rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID(uniqE.Name(), uniqE.Value())}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasE}, resUniqE.Result)

		resNone := rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID("nonexistent", "biters-don't-index")}, 10, "", http.StatusOK)
		assert.Empty(t, resNone.Result)
	})

	t.Run("LookupShells/POST_replace_removes_old_pairs_and_double_delete", func(t *testing.T) {
		aasX := "urn:aas:test:blue-science"

		pairs1 := []types.ISpecificAssetID{
			types.NewSpecificAssetID("alpha", "steam-power"),
			types.NewSpecificAssetID("beta", "coal-burner"),
		}
		rc.PostLookupShells(t, aasX, pairs1)

		got1 := rc.GetLookupShells(t, aasX, http.StatusOK)
		ensureContainsAll(t, got1, map[string][]string{
			"alpha": {"steam-power"},
			"beta":  {"coal-burner"},
		})

		pairs2 := []types.ISpecificAssetID{
			types.NewSpecificAssetID("gamma", "solar-array"),
			types.NewSpecificAssetID("delta", "accumulator-bank"),
		}
		rc.PostLookupShells(t, aasX, pairs2)

		got2 := rc.GetLookupShells(t, aasX, http.StatusOK)
		ensureContainsAll(t, got2, map[string][]string{
			"gamma": {"solar-array"},
			"delta": {"accumulator-bank"},
		})
		assertNoNames(t, got2, "alpha", "beta")

		rc.DeleteLookupShells(t, aasX)
		_ = rc.GetLookupShells(t, aasX, http.StatusNotFound)

		rc.DeleteLookupShellsExpect(t, aasX, http.StatusNotFound)
		_ = rc.GetLookupShells(t, aasX, http.StatusNotFound)
	})

	t.Run("LookupShells/BadRequest_when_aas_not_base64_encoded", func(t *testing.T) {
		rawAAS := "urn:aas:not-encoded:crude-oil"
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, rawAAS)

		_ = testenv.GetExpect(t, url, http.StatusBadRequest)

		body := []types.ISpecificAssetID{types.NewSpecificAssetID("foo", "barrel")}
		_ = testenv.PostJSONExpect(t, url, body, http.StatusBadRequest)

		_ = testenv.DeleteExpect(t, url, http.StatusBadRequest)
	})

	t.Run("LookupShellsByAssetLink/Pagination_two_items_limit1_cursor_points_to_next", func(t *testing.T) {
		aasP1 := "urn:aas:test:copper"
		aasP2 := "urn:aas:test:iron"

		pageTag := "science-pack-3"
		sharedPair := types.NewSpecificAssetID("pageGroup", pageTag)

		rc.PostLookupShells(t, aasP1, []types.ISpecificAssetID{sharedPair})
		rc.PostLookupShells(t, aasP2, []types.ISpecificAssetID{sharedPair})

		expected := sortedStrings(aasP1, aasP2)
		firstID, secondID := expected[0], expected[1]

		res1 := rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID(sharedPair.Name(), sharedPair.Value())}, 1, "", http.StatusOK)
		require.Len(t, res1.Result, 1)
		assert.Equal(t, firstID, res1.Result[0])
		require.NotEmpty(t, res1.PagingMetadata.Cursor)

		dec, err := common.DecodeString(res1.PagingMetadata.Cursor)
		require.NoError(t, err)
		assert.Equal(t, secondID, dec)

		res2 := rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID(sharedPair.Name(), sharedPair.Value())}, 1, res1.PagingMetadata.Cursor, http.StatusOK)
		require.Len(t, res2.Result, 1)
		assert.Equal(t, secondID, res2.Result[0])
		assert.Empty(t, res2.PagingMetadata.Cursor)
	})

	t.Run("LookupShells/BadRequest_POST_links_with_malformed_values", func(t *testing.T) {
		aas := "urn:aas:test:bad-values-post"
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aas))

		body3 := []map[string]any{
			{"name": ""},
			{"value": ""},
			{"name": 123, "value": true},
		}
		_ = testenv.PostJSONExpect(t, url, body3, http.StatusBadRequest)
	})

	t.Run("LookupShells/BadRequest_POST_links_with_wrong_json_shape", func(t *testing.T) {
		aas := "urn:aas:test:wrong-shape-post"
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aas))

		badObj := map[string]any{"name": "x", "value": "y"}
		_ = testenv.PostJSONExpect(t, url, badObj, http.StatusBadRequest)

		_ = testenv.PostJSONExpect(t, url, "not-an-array", http.StatusBadRequest)
		_ = testenv.PostJSONExpect(t, url, nil, http.StatusBadRequest)
	})

	t.Run("LookupShells/search/BadRequest_with_wrong_json_shape", func(t *testing.T) {
		PostLookupShellsSearchRawExpect(t, map[string]any{
			"assetLinks": "not-an-array",
			"limit":      10,
		}, http.StatusBadRequest)

		PostLookupShellsSearchRawExpect(t, map[string]any{
			"assetLinks": []any{
				map[string]any{"name": 123, "value": true},
				map[string]any{"name": "ok"},
				map[string]any{"value": "ok"},
			},
			"limit": 5,
		}, http.StatusBadRequest)

		PostLookupShellsSearchRawExpect(t, []any{
			map[string]any{"name": "foo", "value": "bar"},
		}, http.StatusBadRequest)

		PostLookupShellsSearchRawExpect(t, nil, http.StatusBadRequest)
	})

	t.Run("LookupShells/BadRequest_when_body_is_plain_string_or_encoded_values", func(t *testing.T) {
		aas := "urn:aas:test:encoded-and-string"
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aas))

		rawBody := []byte(`"this-is-a-plain-string"`)
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(rawBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() {
			err := resp.Body.Close()
			require.NoError(t, err)
		}()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		PostLookupShellsSearchRawExpect(t, map[string]any{
			"assetLinks": []any{
				map[string]any{"name": "encoded%20name", "value": "ok"},
				map[string]any{"name": "ok", "value": "YmFkLXZhbHVl"},
			},
			"limit": 10,
		}, http.StatusBadRequest)
	})

	t.Run("LookupShells/BadRequest_POST_links_with_wrong_key", func(t *testing.T) {
		aas := "urn:aas:test:wrong-key"
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aas))

		body := []map[string]any{
			{"foo": "abc", "bar": "xyz"},
		}

		_ = testenv.PostJSONExpect(t, url, body, http.StatusBadRequest)
	})

	t.Run("LookupShells/BadRequest_POST_links_with_empty_name_or_value", func(t *testing.T) {
		aas := "urn:aas:test:empty-fields"
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aas))

		body := []map[string]any{
			{"name": "", "value": "some-value"},
			{"name": "serialNumber", "value": ""},
		}

		_ = testenv.PostJSONExpect(t, url, body, http.StatusBadRequest)
	})

	t.Run("LookupShellsByAssetLink/BadRequest_when_limit_is_negative", func(t *testing.T) {
		body := map[string]any{
			"assetLinks": []any{
				map[string]any{"name": "serialNumber", "value": "SN-red-circuit"},
			},
			"limit": -5,
		}

		PostLookupShellsSearchRawExpect(t, body, http.StatusBadRequest)
	})

	t.Run("LookupShellsByAssetLink/BadRequest_when_body_is_missing", func(t *testing.T) {
		url := fmt.Sprintf("%s/lookup/shellsByAssetLink", testenv.BaseURL)

		req, err := http.NewRequest(http.MethodPost, url, nil)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() {
			err := resp.Body.Close()
			require.NoError(t, err)
		}()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "expected 400 when no body is sent")
	})

	// New tests for GET /lookup/shells (GetAllAssetAdministrationShellIdsByAssetLink)
	t.Run("LookupShellIdsByAssetLinkGET/Single_pair_returns_expected_AAS", func(t *testing.T) {
		// Use data created earlier: aasB has plant=SPIDERTRON-YARD
		res := rc.LookupShellIDsByAssetLinkGET(t, []types.ISpecificAssetID{types.NewSpecificAssetID("plant", "SPIDERTRON-YARD")}, 10, "", http.StatusOK)
		assert.Contains(t, res.Result, "urn:aas:test:oil-refinery")
	})

	t.Run("LookupShellIdsByAssetLinkGET/Multiple_pairs_repeated_params", func(t *testing.T) {
		pairs := []types.ISpecificAssetID{types.NewSpecificAssetID("plant", "SPIDERTRON-YARD"), types.NewSpecificAssetID("serialNumber", "SN-engine-unit")}
		res := rc.LookupShellIDsByAssetLinkGET(t, pairs, 10, "", http.StatusOK)
		assert.Equal(t, []string{"urn:aas:test:oil-refinery"}, res.Result)
	})

	t.Run("LookupShellIdsByAssetLinkGET/Multiple_pairs_single_param_comma_separated", func(t *testing.T) {
		pairs := []types.ISpecificAssetID{types.NewSpecificAssetID("plant", "SPIDERTRON-YARD"), types.NewSpecificAssetID("serialNumber", "SN-engine-unit")}
		res := rc.LookupShellIDsByAssetLinkGET(t, pairs, 10, "", http.StatusOK)
		assert.Equal(t, []string{"urn:aas:test:oil-refinery"}, res.Result)
	})

	t.Run("LookupShellIdsByAssetLinkGET/Pagination_two_items_limit1_cursor_points_to_next", func(t *testing.T) {
		// Reuse dataset created in the pagination POST test: two AAS share pageGroup=science-pack-3
		res1 := rc.LookupShellIDsByAssetLinkGET(t, []types.ISpecificAssetID{types.NewSpecificAssetID("pageGroup", "science-pack-3")}, 1, "", http.StatusOK)
		require.Len(t, res1.Result, 1)
		require.NotEmpty(t, res1.PagingMetadata.Cursor)

		dec, err := common.DecodeString(res1.PagingMetadata.Cursor)
		require.NoError(t, err)
		assert.NotEqual(t, res1.Result[0], dec)

		res2 := rc.LookupShellIDsByAssetLinkGET(t, []types.ISpecificAssetID{types.NewSpecificAssetID("pageGroup", "science-pack-3")}, 1, res1.PagingMetadata.Cursor, http.StatusOK)
		require.Len(t, res2.Result, 1)
		assert.NotEqual(t, res1.Result[0], res2.Result[0])
		assert.Empty(t, res2.PagingMetadata.Cursor)
	})

	t.Run("LookupShellIdsByAssetLinkGET/BadRequest_when_cursor_not_base64", func(t *testing.T) {
		_ = testenv.GetExpect(t, fmt.Sprintf("%s/lookup/shells?limit=1&assetIds=%s&cursor=%s", testenv.BaseURL, common.EncodeString(`{"name":"plant","value":"SPIDERTRON-YARD"}`), "not-base64!!!"), http.StatusBadRequest)
	})

	t.Run("LookupShellsByAssetLink/BadRequest_when_cursor_not_base64", func(t *testing.T) {
		_ = rc.LookupShellsByAssetLink(t, []types.ISpecificAssetID{types.NewSpecificAssetID("plant", "SPIDERTRON-YARD")}, 1, "not-base64!!!", http.StatusBadRequest)
	})

	t.Run("LookupShellIdsByAssetLinkGET/BadRequest_when_assetIds_malformed", func(t *testing.T) {
		// malformed base64
		url := fmt.Sprintf("%s/lookup/shells?limit=10&assetIds=%s", testenv.BaseURL, "%%ZZ-invalid")
		_ = testenv.GetExpect(t, url, http.StatusBadRequest)
	})

}
