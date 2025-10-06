package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	baseURL         = "http://127.0.0.1:5004"
	composeFilePath = "docker_compose/docker_compose.yml"
)

type pagingMetadata struct {
	Cursor string `json:"cursor"`
}
type searchResp struct {
	PagingMetadata pagingMetadata `json:"paging_metadata"`
	Result         []string       `json:"result"`
}
type specificAssetID struct {
	Name              string `json:"name"`
	Value             string `json:"value"`
	ExternalSubjectId string `json:"externalSubjectId,omitempty"`
}

func b64Raw(s string) string { return base64.RawStdEncoding.EncodeToString([]byte(s)) }
func randomHex(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func httpClient() *http.Client { return &http.Client{Timeout: 20 * time.Second} }

func postJSONExpect(t *testing.T, url string, body any, code int) []byte {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest("POST", url, r)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, code, resp.StatusCode, "POST %s expected %d got %d: %s", url, code, resp.StatusCode, string(data))
	return data
}
func getExpect(t *testing.T, url string, code int) []byte {
	t.Helper()
	resp, err := httpClient().Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, code, resp.StatusCode, "GET %s expected %d got %d: %s", url, code, resp.StatusCode, string(data))
	return data
}
func deleteExpect(t *testing.T, url string, code int) []byte {
	t.Helper()
	req, err := http.NewRequest("DELETE", url, nil)
	require.NoError(t, err)
	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, code, resp.StatusCode, "DELETE %s expected %d got %d: %s", url, code, resp.StatusCode, string(data))
	return data
}

func waitHealthy(t *testing.T, url string, maxWait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(maxWait)
	backoff := time.Second
	for {
		resp, err := httpClient().Get(url)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				_ = resp.Body.Close()
				return
			}
			_ = resp.Body.Close()
		}
		if time.Now().After(deadline) {
			require.FailNowf(t, "health timeout", "service not healthy at %s within %s", url, maxWait)
		}
		time.Sleep(backoff)
		if backoff < 5*time.Second {
			backoff += 500 * time.Millisecond
		}
	}
}

func findCompose() (bin string, args []string, err error) {
	if _, e := exec.LookPath("docker"); e == nil {
		return "docker", []string{"compose"}, nil
	}
	if _, e := exec.LookPath("podman"); e == nil {
		return "podman", []string{"compose"}, nil
	}
	return "", nil, errors.New("neither docker nor podman found on PATH")
}
func runCompose(ctx context.Context, base string, args ...string) error {
	cmd := exec.CommandContext(ctx, base, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// optional DB verification -----------------------------------------------------

type dbVerifier struct {
	db *sql.DB
}

// tryConnectDB returns a verifier if DISC_TEST_DSN is present and connection succeeds.
func tryConnectDB(t *testing.T) *dbVerifier {
	dsn := os.Getenv("DISC_TEST_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Log("DISC_TEST_DSN not set; skipping DB verification")
		return nil
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Logf("DB open failed (%v); skipping DB verification", err)
		return nil
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(3)
	if err := db.Ping(); err != nil {
		t.Logf("DB ping failed (%v); skipping DB verification", err)
		return nil
	}
	return &dbVerifier{db: db}
}

// NOTE: This SQL assumes a generic table/view layout where rows are stored per AAS.
// If your schema differs, set DISC_TEST_SQL_COUNT to a working query like:
//
//	SELECT COUNT(*) FROM discovery_specific_asset_ids WHERE aas_id = $1;
//
// and DISC_TEST_SQL_LIST like:
//
//	SELECT name, value FROM discovery_specific_asset_ids WHERE aas_id = $1;
func (v *dbVerifier) countLinks(t *testing.T, aasID string) (int, bool) {
	if v == nil {
		return 0, false
	}
	sqlCount := os.Getenv("DISC_TEST_SQL_COUNT")
	if sqlCount == "" {
		sqlCount = "SELECT COUNT(*) FROM discovery_specific_asset_ids WHERE aas_id = $1"
	}
	var c int
	if err := v.db.QueryRow(sqlCount, aasID).Scan(&c); err != nil {
		t.Logf("countLinks query failed (%v); skipping DB asserts", err)
		return 0, false
	}
	return c, true
}

func (v *dbVerifier) listNameValues(t *testing.T, aasID string) (map[string][]string, bool) {
	if v == nil {
		return nil, false
	}
	sqlList := os.Getenv("DISC_TEST_SQL_LIST")
	if sqlList == "" {
		sqlList = "SELECT name, value FROM discovery_specific_asset_ids WHERE aas_id = $1"
	}
	rows, err := v.db.Query(sqlList, aasID)
	if err != nil {
		t.Logf("listNameValues query failed (%v); skipping DB asserts", err)
		return nil, false
	}
	defer rows.Close()
	m := map[string][]string{}
	for rows.Next() {
		var n, v string
		if err := rows.Scan(&n, &v); err != nil {
			t.Logf("listNameValues scan failed (%v); skipping DB asserts", err)
			return nil, false
		}
		m[n] = append(m[n], v)
	}
	for k := range m {
		sort.Strings(m[k])
	}
	return m, true
}

// helpers for API paths --------------------------------------------------------

func postLinks(t *testing.T, aasID string, links []specificAssetID) {
	url := fmt.Sprintf("%s/lookup/shells/%s", baseURL, b64Raw(aasID))
	_ = postJSONExpect(t, url, links, http.StatusCreated)
}
func getLinks(t *testing.T, aasID string, expect int) []specificAssetID {
	url := fmt.Sprintf("%s/lookup/shells/%s", baseURL, b64Raw(aasID))
	raw := getExpect(t, url, expect)
	if expect != http.StatusOK {
		return nil
	}
	var got []specificAssetID
	require.NoError(t, json.Unmarshal(raw, &got))
	return got
}
func deleteLinks(t *testing.T, aasID string) {
	url := fmt.Sprintf("%s/lookup/shells/%s", baseURL, b64Raw(aasID))
	_ = deleteExpect(t, url, http.StatusNoContent)
}
func searchBy(t *testing.T, pairs []specificAssetID, limit int, cursor string, expect int) searchResp {
	url := fmt.Sprintf("%s/lookup/shellsByAssetLink?limit=%d", baseURL, limit)
	if cursor != "" {
		url += "&cursor=" + cursor
	}
	body := make([]map[string]string, 0, len(pairs))
	for _, p := range pairs {
		body = append(body, map[string]string{"name": p.Name, "value": p.Value})
	}
	raw := postJSONExpect(t, url, body, expect)
	var out searchResp
	if expect == http.StatusOK {
		require.NoError(t, json.Unmarshal(raw, &out))
	}
	return out
}

func ensureContainsAll(t *testing.T, got []specificAssetID, want map[string][]string) {
	// build actual map
	actual := map[string][]string{}
	for _, s := range got {
		actual[s.Name] = append(actual[s.Name], s.Value)
	}
	for k := range actual {
		sort.Strings(actual[k])
	}
	// compare keys + per-key values (ignoring order)
	for k, wantVals := range want {
		gotVals := actual[k]
		sort.Strings(wantVals)
		assert.Equalf(t, wantVals, gotVals, "mismatch for name=%s", k)
	}
	// also assert no extra keys
	assert.Subset(t, keys(actual), keys(want)) // want ⊆ actual
	assert.Subset(t, keys(want), keys(actual)) // actual ⊆ want
}
func keys(m map[string][]string) (ks []string) {
	for k := range m {
		ks = append(ks, k)
	}
	return
}

func TestDiscovery_Suite_Sophisticated(t *testing.T) {
	engine, composeArgs, err := findCompose()
	if err != nil {
		t.Skip("compose engine not found:", err)
	}

	build := os.Getenv("DISC_TEST_BUILD") == "1"
	upArgs := append(composeArgs, "-f", composeFilePath, "up", "-d")
	if build {
		upArgs = append(upArgs, "--build")
	}

	ctxUp, cancelUp := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelUp()
	require.NoError(t, runCompose(ctxUp, engine, upArgs...), "failed to start compose")

	t.Cleanup(func() {
		ctxDown, cancelDown := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancelDown()
		_ = runCompose(ctxDown, engine, append(composeArgs, "-f", composeFilePath, "down")...)
	})

	waitHealthy(t, baseURL+"/health", 2*time.Minute)

	ver := tryConnectDB(t)
	if ver != nil {
		defer ver.db.Close()
	}

	// Prepare three AAS with distinct link profiles
	aasA := "urn:aas:test:" + randomHex(6)
	aasB := "urn:aas:test:" + randomHex(6)
	aasC := "urn:aas:test:" + randomHex(6)

	linksA1 := []specificAssetID{
		{Name: "globalAssetId", Value: "urn:ga:" + randomHex(3)},
		{Name: "serialNumber", Value: "SN-" + randomHex(2)},
		{Name: "plant", Value: "BER"},
	}
	// replacement set for A: different serial + adds "line"
	linksA2 := []specificAssetID{
		{Name: "globalAssetId", Value: linksA1[0].Value},    // keep same GAID
		{Name: "serialNumber", Value: "SN-" + randomHex(2)}, // REPLACED serial
		{Name: "line", Value: "L1"},
	}

	linksB := []specificAssetID{
		{Name: "serialNumber", Value: "SN-B-" + randomHex(2)},
		{Name: "plant", Value: "MUC"},
	}

	linksC := []specificAssetID{
		{Name: "assetTag", Value: "AT-" + randomHex(2)},
	}

	// --- A: create, check, replace, check (and DB-check if possible)
	t.Run("A_create_and_replace", func(t *testing.T) {
		postLinks(t, aasA, linksA1)
		got := getLinks(t, aasA, http.StatusOK)

		wantMap1 := map[string][]string{
			"globalAssetId": {linksA1[0].Value},
			"serialNumber":  {linksA1[1].Value},
			"plant":         {"BER"},
		}
		ensureContainsAll(t, got, wantMap1)

		if ver != nil {
			if cnt, ok := ver.countLinks(t, aasA); ok {
				assert.Equal(t, len(linksA1), cnt, "db should contain initial rows for A")
			}
		}

		// replace
		postLinks(t, aasA, linksA2)
		got2 := getLinks(t, aasA, http.StatusOK)
		// old "plant" must be gone, new "line" must be present, GAID preserved, serial changed
		wantMap2 := map[string][]string{
			"globalAssetId": {linksA2[0].Value},
			"serialNumber":  {linksA2[1].Value},
			"line":          {"L1"},
		}
		ensureContainsAll(t, got2, wantMap2)

		// assert plant is gone
		for _, s := range got2 {
			require.NotEqual(t, "plant", s.Name, "old 'plant' should have been replaced away")
		}

		if ver != nil {
			if cnt, ok := ver.countLinks(t, aasA); ok {
				assert.Equal(t, len(linksA2), cnt, "db should contain replaced row count for A")
			}
			if namevals, ok := ver.listNameValues(t, aasA); ok {
				_, hadPlant := namevals["plant"]
				assert.False(t, hadPlant, "DB: 'plant' row should be gone after replace")
			}
		}
	})

	// --- B & C: create others
	t.Run("B_and_C_create", func(t *testing.T) {
		postLinks(t, aasB, linksB)
		postLinks(t, aasC, linksC)

		gotB := getLinks(t, aasB, http.StatusOK)
		ensureContainsAll(t, gotB, map[string][]string{
			"serialNumber": {linksB[0].Value},
			"plant":        {"MUC"},
		})

		gotC := getLinks(t, aasC, http.StatusOK)
		ensureContainsAll(t, gotC, map[string][]string{
			"assetTag": {linksC[0].Value},
		})
	})

	// --- Search matrix
	t.Run("Search_matrix", func(t *testing.T) {
		// search by A's GAID should return A only
		res := searchBy(t, []specificAssetID{{Name: "globalAssetId", Value: linksA2[0].Value}}, 10, "", http.StatusOK)
		assert.Contains(t, res.Result, aasA)
		assert.NotContains(t, res.Result, aasB)
		assert.NotContains(t, res.Result, aasC)

		// search by A's serial should return A only
		res = searchBy(t, []specificAssetID{{Name: "serialNumber", Value: linksA2[1].Value}}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasA}, res.Result)

		// search by B's plant should return B only
		res = searchBy(t, []specificAssetID{{Name: "plant", Value: "MUC"}}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasB}, res.Result)

		// AND search (two pairs) -> only A matches both GAID & serial
		res = searchBy(t, []specificAssetID{
			{Name: "globalAssetId", Value: linksA2[0].Value},
			{Name: "serialNumber", Value: linksA2[1].Value},
		}, 10, "", http.StatusOK)
		assert.Equal(t, []string{aasA}, res.Result)

		// search for a value that no one has
		res = searchBy(t, []specificAssetID{{Name: "serialNumber", Value: "DOES-NOT-EXIST"}}, 10, "", http.StatusOK)
		assert.Len(t, res.Result, 0)

		// search for name that exists in A and B with different values -> if filtering by name only isn’t supported, expect 400 if server requires both name+value
		// We stick to precise (name,value) pairs here.
	})

	// --- Pagination with multiple matches: give both A & B the same (name,value) on purpose
	t.Run("Pagination_stable", func(t *testing.T) {
		commonTag := "COMMON-" + randomHex(2)

		// add a common link to A (replace to keep other A links intact + add a new one)
		postLinks(t, aasA, append([]specificAssetID{
			{Name: "globalAssetId", Value: linksA2[0].Value},
			{Name: "serialNumber", Value: linksA2[1].Value},
			{Name: "line", Value: "L1"},
		}, specificAssetID{Name: "batch", Value: commonTag}))

		// add common link to B as well (replace)
		postLinks(t, aasB, []specificAssetID{
			{Name: "serialNumber", Value: linksB[0].Value},
			{Name: "plant", Value: "MUC"},
			{Name: "batch", Value: commonTag},
		})

		// Now search for (batch=commonTag) with limit=1 and step through
		res1 := searchBy(t, []specificAssetID{{Name: "batch", Value: commonTag}}, 1, "", http.StatusOK)
		require.Len(t, res1.Result, 1)
		require.NotEmpty(t, res1.PagingMetadata.Cursor)

		res2 := searchBy(t, []specificAssetID{{Name: "batch", Value: commonTag}}, 1, res1.PagingMetadata.Cursor, http.StatusOK)
		require.Len(t, res2.Result, 1)
		require.NotEqual(t, res1.Result[0], res2.Result[0], "page 2 should be a different AAS")

		// page 3 should be empty
		res3 := searchBy(t, []specificAssetID{{Name: "batch", Value: commonTag}}, 1, res2.PagingMetadata.Cursor, http.StatusOK)
		assert.Len(t, res3.Result, 0)
		assert.Equal(t, "", res3.PagingMetadata.Cursor)
	})

	// --- Delete A and verify its disappearance from GET + Search
	t.Run("Delete_and_search_absence", func(t *testing.T) {
		// ensure present first
		_ = getLinks(t, aasA, http.StatusOK)

		deleteLinks(t, aasA)
		_ = getLinks(t, aasA, http.StatusNotFound)

		// Search by A’s GAID should not return A anymore
		res := searchBy(t, []specificAssetID{{Name: "globalAssetId", Value: linksA2[0].Value}}, 10, "", http.StatusOK)
		assert.NotContains(t, res.Result, aasA)
	})
}
