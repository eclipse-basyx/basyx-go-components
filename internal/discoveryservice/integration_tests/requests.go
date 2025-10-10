package bench

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

// ---- endpoint helpers used by tests ----

func PostLinksExpect(t testing.TB, aasID string, links []model.SpecificAssetId, expect int) {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aasID))
	_ = testenv.PostJSONExpect(t, url, links, expect)
}

// Keeps the default as before (Created)
func PostLinks(t testing.TB, aasID string, links []model.SpecificAssetId) {
	t.Helper()
	PostLinksExpect(t, aasID, links, http.StatusCreated)
}

func GetLinksExpect(t testing.TB, aasID string, expect int) []model.SpecificAssetId {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aasID))
	raw := testenv.GetExpect(t, url, expect)
	if expect != http.StatusOK {
		return nil
	}
	var got []model.SpecificAssetId
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal GetLinks response: %v", err)
	}
	return got
}

// Keeps old default
func GetLinks(t testing.TB, aasID string, expect int) []model.SpecificAssetId {
	t.Helper()
	return GetLinksExpect(t, aasID, expect)
}

func DeleteLinksExpect(t testing.TB, aasID string, expect int) {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aasID))
	_ = testenv.DeleteExpect(t, url, expect)
}

// Keeps old default (NoContent)
func DeleteLinks(t testing.TB, aasID string) {
	t.Helper()
	DeleteLinksExpect(t, aasID, http.StatusNoContent)
}

func SearchBy(t testing.TB, pairs []model.SpecificAssetId, limit int, cursor string, expect int) model.GetAllAssetAdministrationShellIdsByAssetLink200Response {
	t.Helper()
	url := fmt.Sprintf("%s/lookup/shellsByAssetLink?limit=%d", testenv.BaseURL, limit)
	if cursor != "" {
		url += "&cursor=" + cursor
	}
	body := make([]map[string]string, 0, len(pairs))
	for _, p := range pairs {
		body = append(body, map[string]string{"name": p.Name, "value": p.Value})
	}
	raw := testenv.PostJSONExpect(t, url, body, expect)
	var out model.GetAllAssetAdministrationShellIdsByAssetLink200Response
	if expect == http.StatusOK {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal SearchBy response: %v", err)
		}
	}
	return out
}
