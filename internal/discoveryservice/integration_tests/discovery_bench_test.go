package bench

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	mrand "math/rand"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	testenv "github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

var seedFlag = flag.Int64("seed", 1, "rng seed for discovery bench determinism")

type discoveryState struct {
	rng         *mrand.Rand
	aasToLinks  map[string][]model.SpecificAssetID
	aasList     []string
	cursorByAAS map[string]string
	reusePool   []model.SpecificAssetID // reused name/value pairs to simulate overlap
}

func newDiscoveryState(seed int64) *discoveryState {
	return &discoveryState{
		rng:         mrand.New(mrand.NewSource(seed)),
		aasToLinks:  make(map[string][]model.SpecificAssetID),
		cursorByAAS: make(map[string]string),
		reusePool:   make([]model.SpecificAssetID, 0, 512),
	}
}

func (s *discoveryState) randHex(nBytes int) string {
	b := make([]byte, nBytes)
	for i := range b {
		b[i] = byte(s.rng.Intn(256))
	}
	return hex.EncodeToString(b)
}

const (
	opPost   = 50
	opGet    = 25
	opDelete = 5
	opSearch = 20

	useStateAAS  = 70
	minLinks     = 1
	maxLinks     = 10
	minPairs     = 0
	maxPairs     = 2
	searchLimit  = 200
	reusePctPost = 50
	reusePoolCap = 1000000
)

func (s *discoveryState) pct(p int) bool { return s.rng.Intn(100) < p }

func (s *discoveryState) boundedRand(minIncl, maxIncl int) int {
	if maxIncl <= minIncl {
		return minIncl
	}
	return minIncl + s.rng.Intn(maxIncl-minIncl+1)
}

func (s *discoveryState) pickWeightedOp() string {
	x := s.rng.Intn(100)
	if x < opPost {
		return "post"
	}
	if x < opPost+opGet {
		return "get"
	}
	if x < opPost+opGet+opDelete {
		return "del"
	}
	return "search"
}

func (s *discoveryState) add(aasID string, links []model.SpecificAssetID) {
	if _, ok := s.aasToLinks[aasID]; ok {
		return
	}
	s.aasToLinks[aasID] = links
	s.aasList = append(s.aasList, aasID)

	s.reusePool = append(s.reusePool, links...)
	if len(s.reusePool) > reusePoolCap {
		s.reusePool = s.reusePool[len(s.reusePool)-reusePoolCap:]
	}
}

func (s *discoveryState) remove(aasID string) {
	if _, ok := s.aasToLinks[aasID]; !ok {
		return
	}
	delete(s.aasToLinks, aasID)
	delete(s.cursorByAAS, aasID)
	for i, id := range s.aasList {
		if id == aasID {
			s.aasList = append(s.aasList[:i], s.aasList[i+1:]...)
			break
		}
	}
}

func (s *discoveryState) randomAAS() (string, bool) {
	if len(s.aasList) == 0 {
		return "", false
	}
	return s.aasList[s.rng.Intn(len(s.aasList))], true
}

func (s *discoveryState) randomLinks(n int) []model.SpecificAssetID {
	out := make([]model.SpecificAssetID, n)
	for i := 0; i < n; i++ {
		if len(s.reusePool) > 0 && s.pct(reusePctPost) {
			out[i] = s.reusePool[s.rng.Intn(len(s.reusePool))]
		} else {
			out[i] = model.SpecificAssetID{

				Name:  "n_" + s.randHex(6),
				Value: "v_" + s.randHex(6),
			}
		}
	}
	return out
}

// ----- bench driver -----

type DiscoveryBench struct{ st *discoveryState }

func NewDiscoveryBench(seed int64) *DiscoveryBench {
	return &DiscoveryBench{st: newDiscoveryState(seed)}
}

func (d *DiscoveryBench) Name() string { return "discovery" }

func (d *DiscoveryBench) DoOne(iter int) testenv.ComponentResult {
	st := d.st

	switch st.pickWeightedOp() {
	case "post":
		// Keep length behavior compatible with prior helper: 8 bytes -> 16 hex chars.
		aasID := "aas_" + st.randHex(8)
		links := st.randomLinks(st.boundedRand(minLinks, maxLinks))

		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aasID))
		reqBody, _ := json.Marshal(links)

		start := time.Now()
		_, code, err := testenv.PostJSONRaw(url, links)
		dur := time.Since(start).Microseconds()
		if code == 201 && err == nil {
			st.add(aasID, links)
		}
		return testenv.ComponentResult{
			Op:         "post",
			DurationMs: dur,
			Code:       code,
			OK:         code == 201,
			Error:      err,
			Method:     "POST",
			URL:        url,
			Request:    reqBody,
			Extra: map[string]any{
				"iter":       iter,
				"aas_id":     aasID,
				"linksCount": len(links),
			},
		}

	case "get":
		var aasID string
		fromState := false
		if st.pct(useStateAAS) {
			if a, ok := st.randomAAS(); ok {
				aasID = a
				fromState = true
			}
		}
		if aasID == "" {
			aasID = "aas_" + st.randHex(8)
		}
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aasID))

		start := time.Now()
		_, code, err := testenv.GetRaw(url)
		dur := time.Since(start).Microseconds()

		_, existed := st.aasToLinks[aasID]
		ok := (fromState && code == 200) || (!fromState && !existed && code == 404)

		return testenv.ComponentResult{
			Op:         "get",
			DurationMs: dur,
			Code:       code,
			OK:         ok,
			Error:      err,
			Method:     "GET",
			URL:        url,
			Extra: map[string]any{
				"iter":      iter,
				"aas_id":    aasID,
				"usedState": fromState,
			},
		}

	case "del":
		var aasID string
		fromState := false
		if st.pct(useStateAAS) {
			if a, ok := st.randomAAS(); ok {
				aasID = a
				fromState = true
			}
		}
		if aasID == "" {
			aasID = "aas_" + st.randHex(8)
		}
		url := fmt.Sprintf("%s/lookup/shells/%s", testenv.BaseURL, common.EncodeString(aasID))

		start := time.Now()
		_, code, err := testenv.DeleteRaw(url)
		dur := time.Since(start).Microseconds()
		if fromState && code == 204 && err == nil {
			st.remove(aasID)
		}
		ok := (fromState && code == 204) || (!fromState && code == 404)

		return testenv.ComponentResult{
			Op:         "del",
			DurationMs: dur,
			Code:       code,
			OK:         ok,
			Error:      err,
			Method:     "DELETE",
			URL:        url,
			Extra: map[string]any{
				"iter":      iter,
				"aas_id":    aasID,
				"usedState": fromState,
			},
		}

	default: // search using only state-backed pairs
		k := st.boundedRand(minPairs, maxPairs)
		if len(st.aasList) == 0 {
			return testenv.ComponentResult{
				Op:         "search",
				DurationMs: 0,
				Code:       0,
				OK:         true,
				Error:      nil,
				Method:     "POST",
				URL:        "skipped: no state-backed pairs",
				Extra: map[string]any{
					"iter":       iter,
					"pairsCount": 0,
					"skipped":    true,
				},
			}
		}
		pairs := make([]model.SpecificAssetID, k)
		for i := 0; i < k; i++ {
			if len(st.aasList) == 0 {
				break
			}
			aas := st.aasList[st.rng.Intn(len(st.aasList))]
			links := st.aasToLinks[aas]
			if len(links) == 0 {
				continue
			}
			pairs[i] = links[st.rng.Intn(len(links))]
		}

		url := fmt.Sprintf("%s/lookup/shellsByAssetLink?limit=%d", testenv.BaseURL, searchLimit)
		body := make([]map[string]string, 0, len(pairs))
		for _, p := range pairs {
			body = append(body, map[string]string{"name": p.Name, "value": p.Value})
		}
		reqBody, _ := json.Marshal(map[string]any{"body": body, "limit": searchLimit})

		start := time.Now()
		raw, code, err := testenv.PostJSONRaw(url, body)
		dur := time.Since(start).Microseconds()

		var resp struct {
			Result []any `json:"result"`
		}
		_ = json.Unmarshal(raw, &resp)
		resultCount := 0
		if resp.Result != nil {
			resultCount = len(resp.Result)
		}
		respSnap, _ := json.Marshal(map[string]any{"result_count": resultCount})

		return testenv.ComponentResult{
			Op:         "search",
			DurationMs: dur,
			Code:       code,
			OK:         code == 200,
			Error:      err,
			Method:     "POST",
			URL:        url,
			Request:    reqBody,
			Response:   respSnap,
			Extra: map[string]any{
				"iter":        iter,
				"pairsCount":  len(pairs),
				"resultCount": resultCount,
			},
		}
	}
}

// example execution: (log levels: full, name, basic) Use full for testing purposes and basic for benchmarking
// result is stored in benchmark_results in root directory
// $env:LOG_DETAIL = "full" go test -bench BenchmarkDiscovery -run ^$ -benchtime=100x -benchmem

func BenchmarkDiscovery(b *testing.B) {
	mustHaveCompose(b)
	waitUntilHealthy(b)

	comp := NewDiscoveryBench(*seedFlag)
	testenv.BenchmarkComponent(b, comp)
}
