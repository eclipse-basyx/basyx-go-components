package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RequestClient centralizes request helpers with endpoint-aligned names.
type RequestClient struct {
	BaseURL string
}

// NewRequestClient creates a new RequestClient with the given base URL.
func NewRequestClient() *RequestClient {
	return &RequestClient{BaseURL: testenv.BaseURL}
}

// PostLookupShellsExpect sends a POST request to /lookup/shells/{aasId}
func (c *RequestClient) PostLookupShellsExpect(t testing.TB, aasID string, links []model.SpecificAssetID, expect int) {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shells/%s", c.BaseURL, common.EncodeString(aasID))
	_ = testenv.PostJSONExpect(t, url, links, expect)
}

// PostLookupShells sends a POST request to /lookup/shells/{aasId}
func (c *RequestClient) PostLookupShells(t testing.TB, aasID string, links []model.SpecificAssetID) {
	t.Helper()
	c.PostLookupShellsExpect(t, aasID, links, http.StatusCreated)
}

// GetLookupShellsExpect sends a GET request to /lookup/shells/{aasId}
func (c *RequestClient) GetLookupShellsExpect(t testing.TB, aasID string, expect int) []model.SpecificAssetID {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shells/%s", c.BaseURL, common.EncodeString(aasID))
	raw := testenv.GetExpect(t, url, expect)
	if expect != http.StatusOK {
		return nil
	}
	var got []model.SpecificAssetID
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal GetLookupShells response: %v", err)
	}
	return got
}

// GetLookupShells retrieves the lookup shell for the given AAS ID.
func (c *RequestClient) GetLookupShells(t testing.TB, aasID string, expect int) []model.SpecificAssetID {
	t.Helper()
	return c.GetLookupShellsExpect(t, aasID, expect)
}

// DeleteLookupShellsExpect sends a DELETE request to /lookup/shells/{aasId}
func (c *RequestClient) DeleteLookupShellsExpect(t testing.TB, aasID string, expect int) {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shells/%s", c.BaseURL, common.EncodeString(aasID))
	_ = testenv.DeleteExpect(t, url, expect)
}

// DeleteLookupShells deletes the lookup shell for the given AAS ID.
func (c *RequestClient) DeleteLookupShells(t testing.TB, aasID string) {
	t.Helper()
	c.DeleteLookupShellsExpect(t, aasID, http.StatusNoContent)
}

// LookupShellIDsByAssetLinkGET queries GET /lookup/shells?assetIds=...&limit=&cursor=
func (c *RequestClient) LookupShellIDsByAssetLinkGET(
	t testing.TB,
	pairs []model.SpecificAssetID,
	limit int,
	cursor string,
	expect int,
) model.GetAllAssetAdministrationShellIdsByAssetLink200Response {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shells?limit=%d", c.BaseURL, limit)
	// add each assetIds as base64url-encoded JSON {"name":"...","value":"..."}
	for _, p := range pairs {
		obj, err := json.Marshal(map[string]string{"name": p.Name, "value": p.Value})
		require.NoError(t, err)
		url += "&assetIds=" + common.EncodeString(string(obj))
	}
	if cursor != "" {
		url += "&cursor=" + cursor
	}

	raw := testenv.GetExpect(t, url, expect)
	var out model.GetAllAssetAdministrationShellIdsByAssetLink200Response
	if expect == http.StatusOK {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal LookupShellIdsByAssetLinkGET response: %v", err)
		}
	}
	return out
}

// LookupShellsByAssetLink sends a POST request to /lookup/shellsByAssetLink?limit=&cursor= (renamed from SearchBy)
func (c *RequestClient) LookupShellsByAssetLink(
	t testing.TB,
	pairs []model.SpecificAssetID,
	limit int,
	cursor string,
	expect int,
) model.GetAllAssetAdministrationShellIdsByAssetLink200Response {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shellsByAssetLink?limit=%d", c.BaseURL, limit)
	if cursor != "" {
		url += "&cursor=" + cursor
	}
	body := make([]map[string]string, 0, len(pairs))
	for _, p := range pairs {
		body = append(body, map[string]string{"name": p.Name, "value": p.Value})
	}
	raw := testenv.PostJSONExpect(t, url, body, expect)
	var out model.GetAllAssetAdministrationShellIdsByAssetLink200Response
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if expect == http.StatusOK {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal LookupShellsByAssetLink response: %v", err)
		}
	}
	return out
}

// PostLookupShellsSearchRawExpect sends a raw POST request to /lookup/shells/search
func PostLookupShellsSearchRawExpect(t *testing.T, body any, expect int) {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shells/search", testenv.BaseURL)
	buf, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()
	assert.Equalf(t, expect, resp.StatusCode, "search raw post got %d body=%s", resp.StatusCode, string(buf))
}

func ensureContainsAll(t *testing.T, got []model.SpecificAssetID, want map[string][]string) {
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

func assertNoNames(t *testing.T, got []model.SpecificAssetID, forbidden ...string) {
	t.Helper()
	set := map[string]struct{}{}
	for _, s := range got {
		set[s.Name] = struct{}{}
	}
	for _, name := range forbidden {
		_, exists := set[name]
		require.Falsef(t, exists, "stale key %q still present; response=%v", name, got)
	}
}

func sortedStrings(in ...string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
