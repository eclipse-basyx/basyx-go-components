package bench

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ensureContainsAll(t *testing.T, got []model.SpecificAssetId, want map[string][]string) {
	actual := testenv.BuildNameValuesMap(got)
	for k, wantVals := range want {
		gotVals := actual[k]
		assert.Equalf(t, wantVals, gotVals, "mismatch for name=%s", k)
	}
	assert.Subset(t, keys(actual), keys(want), "response contained extra names not expected; got=%v want=%v", keys(actual), keys(want))
	assert.Subset(t, keys(want), keys(actual), "response missing expected names; got=%v want=%v", keys(actual), keys(want))
}

func keys(m map[string][]string) (ks []string) {
	for k := range m {
		ks = append(ks, k)
	}
	return
}

type DBVerifier struct{ DB *sql.DB }

// TryConnectDB attempts to connect to the test database if DSN is provided.
func TryConnectDB(t testing.TB) *DBVerifier {
	t.Helper()
	dsn := os.Getenv("DISC_TEST_DSN")
	if strings.TrimSpace(dsn) == "" {
		return nil
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(3)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil
	}
	return &DBVerifier{DB: db}
}

func (v *DBVerifier) CountLinks(aasID string) (int, bool) {
	if v == nil {
		return 0, false
	}
	sqlCount := os.Getenv("DISC_TEST_SQL_COUNT")
	if sqlCount == "" {
		sqlCount = "SELECT COUNT(*) FROM discovery_specific_asset_ids WHERE aas_id = $1"
	}
	var c int
	if err := v.DB.QueryRow(sqlCount, aasID).Scan(&c); err != nil {
		return 0, false
	}
	return c, true
}

func (v *DBVerifier) ListNameValues(aasID string) (map[string][]string, bool) {
	if v == nil {
		return nil, false
	}
	sqlList := os.Getenv("DISC_TEST_SQL_LIST")
	if sqlList == "" {
		sqlList = "SELECT name, value FROM discovery_specific_asset_ids WHERE aas_id = $1"
	}
	rows, err := v.DB.Query(sqlList, aasID)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	m := map[string][]string{}
	for rows.Next() {
		var n, v string
		if err := rows.Scan(&n, &v); err != nil {
			return nil, false
		}
		m[n] = append(m[n], v)
	}
	for k := range m {
		sort.Strings(m[k])
	}
	return m, true
}

func TestDiscovery_Suite_Sophisticated(t *testing.T) {
	mustHaveCompose(t)
	waitUntilHealthy(t)

	ver := TryConnectDB(t)
	if ver != nil {
		defer ver.DB.Close()
	}

	aasA := "urn:aas:test:assembler-1"
	aasB := "urn:aas:test:oil-refinery"
	aasC := "urn:aas:test:rail-signal"

	linksA1 := []model.SpecificAssetId{
		{Name: "globalAssetId", Value: "urn:ga:green-circuit"},
		{Name: "serialNumber", Value: "SN-iron-gear"},
		{Name: "plant", Value: "NAUVIS"},
	}
	linksA2 := []model.SpecificAssetId{
		{Name: "globalAssetId", Value: linksA1[0].Value},
		{Name: "serialNumber", Value: "SN-red-circuit"},
		{Name: "line", Value: "L1"},
	}

	linksB := []model.SpecificAssetId{
		{Name: "serialNumber", Value: "SN-engine-unit"},
		{Name: "plant", Value: "SPIDERTRON-YARD"},
	}

	linksC := []model.SpecificAssetId{
		{Name: "assetTag", Value: "belt-yellow"},
	}

	t.Run("Pagination_empty_set_returns_empty_and_no_cursor", func(t *testing.T) {

		res := SearchBy(t, []model.SpecificAssetId{}, 5, "", http.StatusOK)

		assert.Empty(t, res.Result,
			"empty dataset should yield no AAS IDs; got=%v", res.Result)

		assert.Empty(t, res.PagingMetadata.Cursor,
			"empty dataset should not produce a cursor; got=%q", res.PagingMetadata.Cursor)
	})

	t.Run("A_create_and_replace", func(t *testing.T) {
		PostLinks(t, aasA, linksA1)
		got := GetLinks(t, aasA, http.StatusOK)

		wantMap1 := map[string][]string{
			"globalAssetId": {linksA1[0].Value},
			"serialNumber":  {linksA1[1].Value},
			"plant":         {"NAUVIS"},
		}
		ensureContainsAll(t, got, wantMap1)

		if ver != nil {
			if cnt, ok := ver.CountLinks(aasA); ok {
				assert.Equal(t, len(linksA1), cnt, "DB row count mismatch for %q after initial POST: want=%d got=%d", aasA, len(linksA1), cnt)
			}
		}

		PostLinks(t, aasA, linksA2)
		got2 := GetLinks(t, aasA, http.StatusOK)

		wantMap2 := map[string][]string{
			"globalAssetId": {linksA2[0].Value},
			"serialNumber":  {linksA2[1].Value},
			"line":          {"L1"},
		}
		ensureContainsAll(t, got2, wantMap2)

		for _, s := range got2 {
			require.NotEqual(t, "plant", s.Name, "stale key 'plant' still present after replace for %q; response=%v", aasA, got2)
		}

		if ver != nil {
			if cnt, ok := ver.CountLinks(aasA); ok {
				assert.Equal(t, len(linksA2), cnt, "DB row count mismatch for %q after replace: want=%d got=%d", aasA, len(linksA2), cnt)
			}
			if namevals, ok := ver.ListNameValues(aasA); ok {
				_, hadPlant := namevals["plant"]
				assert.False(t, hadPlant, "DB still contains 'plant' for %q after replace; rows=%v", aasA, namevals)
			}
		}
	})

	t.Run("B_and_C_create", func(t *testing.T) {
		PostLinks(t, aasB, linksB)
		PostLinks(t, aasC, linksC)

		gotB := GetLinks(t, aasB, http.StatusOK)
		ensureContainsAll(t, gotB, map[string][]string{
			"serialNumber": {linksB[0].Value},
			"plant":        {"SPIDERTRON-YARD"},
		})

		gotC := GetLinks(t, aasC, http.StatusOK)
		ensureContainsAll(t, gotC, map[string][]string{
			"assetTag": {linksC[0].Value},
		})
	})

	t.Run("Search_matrix", func(t *testing.T) {
		// Single-criteria searches
		res := SearchBy(t, []model.SpecificAssetId{{Name: "globalAssetId", Value: linksA2[0].Value}}, 10, "", http.StatusOK)
		assert.Contains(t, res.Result, aasA, "search by globalAssetId should include %q; got=%v", aasA, res.Result)
		assert.NotContains(t, res.Result, aasB, "search by globalAssetId should not include %q; got=%v", aasB, res.Result)
		assert.NotContains(t, res.Result, aasC, "search by globalAssetId should not include %q; got=%v", aasC, res.Result)

		res = SearchBy(t, []model.SpecificAssetId{{Name: "serialNumber", Value: linksA2[1].Value}}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasA}, res.Result, "search by serialNumber=%q should return only %q; got=%v", linksA2[1].Value, aasA, res.Result)

		res = SearchBy(t, []model.SpecificAssetId{{Name: "plant", Value: "SPIDERTRON-YARD"}}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasB}, res.Result, "search by plant=SPIDERTRON-YARD should return only %q; got=%v", aasB, res.Result)

		// Multi-criteria (AND) search
		res = SearchBy(t, []model.SpecificAssetId{
			{Name: "globalAssetId", Value: linksA2[0].Value},
			{Name: "serialNumber", Value: linksA2[1].Value},
		}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasA}, res.Result, "AND search should match only %q; got=%v", aasA, res.Result)

		// Nonexistent value
		res = SearchBy(t, []model.SpecificAssetId{{Name: "serialNumber", Value: "SN-does-not-exist"}}, 10, "", http.StatusOK)
		assert.Len(t, res.Result, 0, "search for nonexistent serialNumber should be empty; got=%v", res.Result)
	})

	t.Run("Delete_and_search_absence", func(t *testing.T) {
		_ = GetLinks(t, aasA, http.StatusOK)

		DeleteLinks(t, aasA)
		_ = GetLinks(t, aasA, http.StatusNotFound)

		// A should no longer appear in search
		res := SearchBy(t, []model.SpecificAssetId{{Name: "globalAssetId", Value: linksA2[0].Value}}, 10, "", http.StatusOK)
		assert.NotContains(t, res.Result, aasA, "deleted AAS %q unexpectedly present in search results; got=%v", aasA, res.Result)
	})

	t.Run("Shared_and_unique_pairs_across_two_AAS", func(t *testing.T) {
		// Two new AAS identifiers for isolation
		aasD := "urn:aas:test:copper-plate"
		aasE := "urn:aas:test:iron-gear"

		// One shared pair (same for both AAS) + one unique pair each
		shared := model.SpecificAssetId{Name: "sharedTag", Value: "train-signal"}
		uniqD := model.SpecificAssetId{Name: "uniqueD", Value: "uranium-fuel-cell"}
		uniqE := model.SpecificAssetId{Name: "uniqueE", Value: "rocket-control-unit"}

		// Post exactly two pairs per AAS (shared + unique)
		PostLinks(t, aasD, []model.SpecificAssetId{shared, uniqD})
		PostLinks(t, aasE, []model.SpecificAssetId{shared, uniqE})

		// 1) Search by the shared pair: should return both AAS
		resShared := SearchBy(t, []model.SpecificAssetId{{Name: shared.Name, Value: shared.Value}}, 10, "", http.StatusOK)
		assert.Contains(t, resShared.Result, aasD, "shared search (%s=%s) must include %q; got=%v", shared.Name, shared.Value, aasD, resShared.Result)
		assert.Contains(t, resShared.Result, aasE, "shared search (%s=%s) must include %q; got=%v", shared.Name, shared.Value, aasE, resShared.Result)
		assert.Len(t, resShared.Result, 2, "shared search (%s=%s) should return exactly 2 AAS; got=%v", shared.Name, shared.Value, resShared.Result)

		// 2) Search by uniqD: should return only aasD
		resUniqD := SearchBy(t, []model.SpecificAssetId{{Name: uniqD.Name, Value: uniqD.Value}}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasD}, resUniqD.Result, "unique search (%s=%s) should return only %q; got=%v", uniqD.Name, uniqD.Value, aasD, resUniqD.Result)

		// 3) Search by uniqE: should return only aasE
		resUniqE := SearchBy(t, []model.SpecificAssetId{{Name: uniqE.Name, Value: uniqE.Value}}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasE}, resUniqE.Result, "unique search (%s=%s) should return only %q; got=%v", uniqE.Name, uniqE.Value, aasE, resUniqE.Result)

		// 4) Search by a pair that was never added: should return empty
		resNone := SearchBy(t, []model.SpecificAssetId{{Name: "nonexistent", Value: "biters-don't-index"}}, 10, "", http.StatusOK)
		assert.Empty(t, resNone.Result, "search for nonexistent pair should return empty; got=%v", resNone.Result)
	})

	t.Run("Replace_links_removes_old_pairs_and_delete_twice", func(t *testing.T) {
		aasX := "urn:aas:test:blue-science"

		// Initial pairs
		pairs1 := []model.SpecificAssetId{
			{Name: "alpha", Value: "steam-power"},
			{Name: "beta", Value: "coal-burner"},
		}
		PostLinks(t, aasX, pairs1)

		got1 := GetLinks(t, aasX, http.StatusOK)
		ensureContainsAll(t, got1, map[string][]string{
			"alpha": {"steam-power"},
			"beta":  {"coal-burner"},
		})

		// Replace with entirely new pairs (same AAS identifier, new pairs)
		pairs2 := []model.SpecificAssetId{
			{Name: "gamma", Value: "solar-array"},
			{Name: "delta", Value: "accumulator-bank"},
		}
		PostLinks(t, aasX, pairs2)

		got2 := GetLinks(t, aasX, http.StatusOK)
		ensureContainsAll(t, got2, map[string][]string{
			"gamma": {"solar-array"},
			"delta": {"accumulator-bank"},
		})
		// Ensure old names are gone after replacement
		for _, s := range got2 {
			require.NotEqual(t, "alpha", s.Name, "stale key 'alpha' still present after replace for %q; response=%v", aasX, got2)
			require.NotEqual(t, "beta", s.Name, "stale key 'beta' still present after replace for %q; response=%v", aasX, got2)
		}

		// Optional DB checks if DSN provided
		if ver := TryConnectDB(t); ver != nil {
			defer ver.DB.Close()

			if cnt, ok := ver.CountLinks(aasX); ok {
				assert.Equal(t, len(pairs2), cnt, "DB row count mismatch for %q after replace: want=%d got=%d", aasX, len(pairs2), cnt)
			}
			if namevals, ok := ver.ListNameValues(aasX); ok {
				_, hadAlpha := namevals["alpha"]
				_, hadBeta := namevals["beta"]
				assert.False(t, hadAlpha, "DB still contains 'alpha' for %q after replace; rows=%v", aasX, namevals)
				assert.False(t, hadBeta, "DB still contains 'beta' for %q after replace; rows=%v", aasX, namevals)
				assert.Contains(t, namevals, "gamma", "DB missing expected 'gamma' for %q after replace; rows=%v", aasX, namevals)
				assert.Contains(t, namevals, "delta", "DB missing expected 'delta' for %q after replace; rows=%v", aasX, namevals)
			}
		}

		// Delete once -> subsequent GET should be 404
		DeleteLinks(t, aasX)
		_ = GetLinks(t, aasX, http.StatusNotFound)

		// Delete again -> use new helper with explicit expectation
		DeleteLinksExpect(t, aasX, http.StatusNotFound)

		// Still absent
		_ = GetLinks(t, aasX, http.StatusNotFound)
	})

	t.Run("BadRequest_when_aas_not_base64_encoded", func(t *testing.T) {
		// Construct a raw, NOT-base64-encoded AAS ID and bypass helper encoding.
		rawAAS := "urn:aas:not-encoded:crude-oil"

		// GET should return 400 Bad Request
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, rawAAS)
		_ = testenv.GetExpect(t, url, http.StatusBadRequest)

		// POST with raw path should also return 400
		body := []model.SpecificAssetId{
			{Name: "foo", Value: "barrel"},
		}
		_ = testenv.PostJSONExpect(t, url, body, http.StatusBadRequest)

		// DELETE with raw path should also return 400
		_ = testenv.DeleteExpect(t, url, http.StatusBadRequest)
	})

	t.Run("Pagination_two_items_limit1_cursor_points_to_next", func(t *testing.T) {
		// Two AAS sharing a common tag → exactly 2 results.
		aasP1 := "urn:aas:test:copper"
		aasP2 := "urn:aas:test:iron"

		pageTag := "science-pack-3"
		sharedPair := model.SpecificAssetId{Name: "pageGroup", Value: pageTag}

		PostLinks(t, aasP1, []model.SpecificAssetId{sharedPair})
		PostLinks(t, aasP2, []model.SpecificAssetId{sharedPair})

		// Determine expected order (API orders by aasId ASC).
		expected := []string{aasP1, aasP2}
		sort.Strings(expected)
		firstID, secondID := expected[0], expected[1]

		// Page 1 (limit=1): must return firstID, and the cursor must point to secondID (encoded).
		res1 := SearchBy(t, []model.SpecificAssetId{{Name: sharedPair.Name, Value: sharedPair.Value}}, 1, "", http.StatusOK)
		require.Len(t, res1.Result, 1, "first page should contain 1 result for %s=%s; got=%v", sharedPair.Name, sharedPair.Value, res1.Result)
		assert.Equal(t, firstID, res1.Result[0], "first page should return first AAS lexicographically; want=%q got=%q", firstID, res1.Result[0])
		require.NotEmpty(t, res1.PagingMetadata.Cursor, "first page should provide a next cursor for %s=%s", sharedPair.Name, sharedPair.Value)

		// Cursor should equal the *next* AAS id (after decoding).
		dec, err := common.DecodeString(res1.PagingMetadata.Cursor)
		require.NoError(t, err, "cursor must be a valid encoded string; got=%q", res1.PagingMetadata.Cursor)
		assert.Equal(t, secondID, dec, "cursor should point to the next AAS id; want=%q got=%q", secondID, dec)

		// Page 2 (limit=1, using cursor from page 1): must return secondID and no further cursor.
		res2 := SearchBy(t, []model.SpecificAssetId{{Name: sharedPair.Name, Value: sharedPair.Value}}, 1, res1.PagingMetadata.Cursor, http.StatusOK)
		require.Len(t, res2.Result, 1, "second page should contain the remaining 1 result for %s=%s; got=%v", sharedPair.Name, sharedPair.Value, res2.Result)
		assert.Equal(t, secondID, res2.Result[0], "second page should return the second AAS; want=%q got=%q", secondID, res2.Result[0])
		assert.Empty(t, res2.PagingMetadata.Cursor, "second page should not provide a cursor (no more pages); got=%q", res2.PagingMetadata.Cursor)
	})

	t.Run("BadRequest_POST_links_with_malformed_values", func(t *testing.T) {
		aas := "urn:aas:test:bad-values-post"
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aas))

		// missing required fields (wrong JSON inside array) – name empty + value empty should fail validation if enforced
		body3 := []map[string]any{
			{"name": ""},                 // missing value
			{"value": ""},                // missing name
			{"name": 123, "value": true}, // wrong types
		}
		_ = testenv.PostJSONExpect(t, url, body3, http.StatusBadRequest)
	})

	t.Run("BadRequest_POST_links_with_wrong_json_shape", func(t *testing.T) {
		aas := "urn:aas:test:wrong-shape-post"
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aas))

		// object instead of []SpecificAssetId
		badObj := map[string]any{"name": "x", "value": "y"}
		_ = testenv.PostJSONExpect(t, url, badObj, http.StatusBadRequest)

		// string instead of array
		_ = testenv.PostJSONExpect(t, url, "not-an-array", http.StatusBadRequest)

		// null instead of array
		_ = testenv.PostJSONExpect(t, url, nil, http.StatusBadRequest)
	})

	t.Run("BadRequest_SEARCH_with_wrong_json_shape", func(t *testing.T) {
		// assetLinks is required to be an array → send wrong shapes

		// 1) assetLinks as string
		testenv.PostSearchRawExpect(t, map[string]any{
			"assetLinks": "not-an-array",
			"limit":      10,
		}, http.StatusBadRequest)

		// 2) assetLinks entries with wrong types
		testenv.PostSearchRawExpect(t, map[string]any{
			"assetLinks": []any{
				map[string]any{"name": 123, "value": true}, // wrong types
				map[string]any{"name": "ok"},               // missing value
				map[string]any{"value": "ok"},              // missing name
			},
			"limit": 5,
		}, http.StatusBadRequest)

		// 3) whole body is a plain array instead of object
		testenv.PostSearchRawExpect(t, []any{
			map[string]any{"name": "foo", "value": "bar"},
		}, http.StatusBadRequest)

		// 4) null body
		testenv.PostSearchRawExpect(t, nil, http.StatusBadRequest)
	})

	t.Run("BadRequest_when_body_is_plain_string_or_encoded_values", func(t *testing.T) {
		aas := "urn:aas:test:encoded-and-string"
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aas))

		// 1) Body is literally a plain string — not JSON at all.
		rawBody := []byte(`"this-is-a-plain-string"`)
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(rawBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"expected 400 Bad Request when sending plain string body; got=%d", resp.StatusCode)

		// 2) Repeat for search endpoint with encoded names/values
		testenv.PostSearchRawExpect(t, map[string]any{
			"assetLinks": []any{
				map[string]any{"name": "encoded%20name", "value": "ok"},
				map[string]any{"name": "ok", "value": "YmFkLXZhbHVl"},
			},
			"limit": 10,
		}, http.StatusBadRequest)
	})

}
