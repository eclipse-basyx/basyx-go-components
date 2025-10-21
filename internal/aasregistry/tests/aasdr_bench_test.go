package bench

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	mrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	testenv "github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

var seedFlag = flag.Int64("seed", 1, "rng seed for discovery bench determinism")

// must returns the value or panics if err != nil.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// must0 panics if err != nil (useful for funcs that only return error).
func must0(err error) {
	if err != nil {
		panic(err)
	}
}

type discoveryState struct {
	rng         *mrand.Rand
	aasToLinks  map[string][]model.SpecificAssetId
	aasList     []string
	cursorByAAS map[string]string
	reusePool   []model.SpecificAssetId // reused name/value pairs to simulate overlap

	// new state for filtering and getById
	assetTypes []string
	assetKinds []string
}

func newDiscoveryState(seed int64) *discoveryState {
	return &discoveryState{
		rng:         mrand.New(mrand.NewSource(seed)),
		aasToLinks:  make(map[string][]model.SpecificAssetId),
		cursorByAAS: make(map[string]string),
		reusePool:   make([]model.SpecificAssetId, 0, 512),
		assetTypes:  make([]string, 0, 16),
		assetKinds:  make([]string, 0, 3),
	}
}

func (s *discoveryState) randHex(nBytes int) string {
	b := make([]byte, nBytes)
	for i := range b {
		b[i] = byte(s.rng.Intn(256))
	}
	return hex.EncodeToString(b)
}

// weights (percent)
const (
	opPostPct    = 40
	opSearchPct  = 40
	opGetByIdPct = 20
)

// pickWeightedOp returns "post", "search", or "getById"
func (s *discoveryState) pickWeightedOp() string {
	x := s.rng.Intn(100)
	switch {
	case x < opPostPct:
		return "post"
	case x < opPostPct+opSearchPct:
		return "search"
	default:
		return "getById"
	}
}

// ----- bench driver -----

type DiscoveryBench struct {
	st       *discoveryState
	client   *http.Client
	baseURL  string
	template []byte // raw JSON read once
}

func NewDiscoveryBench(seed int64) *DiscoveryBench {
	// Read template once up-front; fail fast if it is missing/invalid.
	tpl := must(os.ReadFile("example_data.json"))

	return &DiscoveryBench{
		st:       newDiscoveryState(seed),
		client:   &http.Client{Timeout: 10 * time.Second},
		baseURL:  "http://127.0.0.1:5004/shell-descriptors",
		template: tpl,
	}
}

func (d *DiscoveryBench) Name() string { return "descriptor" }

// --- helpers ---

// chooseAssetType returns an assetType according to the 10% new / 90% reuse rule and records it if new.
func (d *DiscoveryBench) chooseAssetType() string {
	// 10%: generate new; 90%: reuse if possible
	genNew := d.st.rng.Intn(100) < 10
	if !genNew && len(d.st.assetTypes) > 0 {
		return d.st.assetTypes[d.st.rng.Intn(len(d.st.assetTypes))]
	}
	// generate new (or first ever)
	at := "type-" + d.st.randHex(4)
	d.st.assetTypes = append(d.st.assetTypes, at)
	return at
}

// chooseAssetKind picks from the allowed enum values and records unseen ones.
func (d *DiscoveryBench) chooseAssetKind() string {
	choices := []string{"Instance", "NotApplicable", "Type"}
	kind := choices[d.st.rng.Intn(len(choices))]
	// record if new
	found := false
	for _, k := range d.st.assetKinds {
		if k == kind {
			found = true
			break
		}
	}
	if !found {
		d.st.assetKinds = append(d.st.assetKinds, kind)
	}
	return kind
}

// pickExistingAssetType/kind for search filtering
func (d *DiscoveryBench) pickExistingAssetType() (string, bool) {
	if len(d.st.assetTypes) == 0 {
		return "", false
	}
	return d.st.assetTypes[d.st.rng.Intn(len(d.st.assetTypes))], true
}
func (d *DiscoveryBench) pickExistingAssetKind() (string, bool) {
	if len(d.st.assetKinds) == 0 {
		return "", false
	}
	return d.st.assetKinds[d.st.rng.Intn(len(d.st.assetKinds))], true
}

// makeUniqueDescriptor clones the template JSON, randomizes IDs, and overrides assetType/assetKind.
// Fail-fast on any error.
func (d *DiscoveryBench) makeUniqueDescriptor() (json.RawMessage, string, string, string) {
	if len(d.template) == 0 {
		panic("template not loaded")
	}
	var raw map[string]any
	must0(json.Unmarshal(d.template, &raw))

	// unique root id
	rootID := d.st.randHex(16)
	raw["id"] = rootID

	// override/assign assetType and assetKind
	assetType := d.chooseAssetType()
	assetKind := d.chooseAssetKind()
	raw["assetType"] = assetType
	raw["assetKind"] = assetKind

	// unique submodels (best-effort; keep structure loose)
	if smd, ok := raw["submodelDescriptors"].([]any); ok {
		for i := range smd {
			if m, ok := smd[i].(map[string]any); ok {
				m["id"] = d.st.randHex(16)
				if _, has := m["idShort"]; has {
					m["idShort"] = "smd-" + d.st.randHex(8)
				}
			}
		}
	}

	buf := must(json.Marshal(raw))
	return json.RawMessage(buf), rootID, assetType, assetKind
}

// tryCountResults inspects a JSON response and returns a best-effort count.
func tryCountResults(body []byte) int {
	// case 1: it's an array
	var arr []any
	if err := json.Unmarshal(body, &arr); err == nil {
		return len(arr)
	}
	// case 2: object with items/total-ish fields
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err == nil {
		candidateKeys := []string{"resultCount", "total", "totalItems", "count", "size"}
		for _, k := range candidateKeys {
			if v, ok := obj[k]; ok {
				switch vv := v.(type) {
				case float64:
					return int(vv)
				case int:
					return vv
				}
			}
		}
		// items/values arrays
		for _, k := range []string{"items", "value", "result", "results"} {
			if v, ok := obj[k]; ok {
				if s, ok := v.([]any); ok {
					return len(s)
				}
			}
		}
	}
	return 0
}

func (d *DiscoveryBench) DoOne(iter int) testenv.ComponentResult {
	op := d.st.pickWeightedOp()

	switch op {
	case "post":
		reqBody, rootID, assetType, assetKind := d.makeUniqueDescriptor()

		req := must(http.NewRequest("POST", d.baseURL, bytes.NewReader(reqBody)))
		req.Header.Set("Content-Type", "application/json")

		start := time.Now()
		resp := must(d.client.Do(req))
		dur := time.Since(start)

		defer resp.Body.Close()
		respBody := must(io.ReadAll(resp.Body))

		ok := resp.StatusCode >= 200 && resp.StatusCode < 300
		if ok {
			// record the posted id and the used type/kind for future ops
			d.st.aasList = append(d.st.aasList, rootID)
			// ensure type/kind are recorded (choose* already did, but keep future-proof)
			foundT := false
			for _, t := range d.st.assetTypes {
				if t == assetType {
					foundT = true
					break
				}
			}
			if !foundT {
				d.st.assetTypes = append(d.st.assetTypes, assetType)
			}
			foundK := false
			for _, k := range d.st.assetKinds {
				if k == assetKind {
					foundK = true
					break
				}
			}
			if !foundK {
				d.st.assetKinds = append(d.st.assetKinds, assetKind)
			}
		}

		return testenv.ComponentResult{
			DurationMs: dur.Nanoseconds(),
			Code:       resp.StatusCode,
			OK:         ok,
			Error:      nil,
			Op:         "post",
			Method:     "POST",
			URL:        d.baseURL,
			Request:    reqBody,
			Response:   json.RawMessage(respBody),
			Extra: map[string]any{
				"iter":        iter,
				"resultCount": 1, // posted one descriptor
				"id":          rootID,
				"assetType":   assetType,
				"assetKind":   assetKind,
			},
		}

	case "getById":
		// need an id; if none posted yet, fall back to search
		if len(d.st.aasList) == 0 {
			op = "search"
		} else {
			// pick a random posted ID
			id := d.st.aasList[d.st.rng.Intn(len(d.st.aasList))]

			// ✅ Encode to base64 for path usage
			encoded := base64.StdEncoding.EncodeToString([]byte(id))
			u := d.baseURL + "/" + url.PathEscape(encoded)

			req := must(http.NewRequest("GET", u, nil))

			start := time.Now()
			resp := must(d.client.Do(req))
			dur := time.Since(start)

			defer resp.Body.Close()
			body := must(io.ReadAll(resp.Body))

			ok := resp.StatusCode >= 200 && resp.StatusCode < 300
			return testenv.ComponentResult{
				DurationMs: dur.Nanoseconds(),
				Code:       resp.StatusCode,
				OK:         ok,
				Error:      nil, // will never reach here if a request/read error occurred
				Op:         "getById",
				Method:     "GET",
				URL:        u,
				Request:    nil,
				Response:   json.RawMessage(body),
				Extra: map[string]any{
					"iter":      iter,
					"id":        id,
					"encodedId": encoded,
				},
			}
		}

		// if there was no id, we drop into a search op below
		fallthrough

	default: // "search"
		// Build query params with 50% chance for each param using existing state values.
		q := url.Values{}
		if at, ok := d.pickExistingAssetType(); ok && d.st.rng.Intn(100) < 50 {
			q.Set("assetType", at)
		}
		if ak, ok := d.pickExistingAssetKind(); ok && d.st.rng.Intn(100) < 50 {
			q.Set("assetKind", ak)
		}

		u := d.baseURL
		if len(q) > 0 {
			u = u + "?" + q.Encode()
		}

		req := must(http.NewRequest("GET", u, nil))

		start := time.Now()
		resp := must(d.client.Do(req))
		dur := time.Since(start)

		defer resp.Body.Close()
		body := must(io.ReadAll(resp.Body))

		ok := resp.StatusCode >= 200 && resp.StatusCode < 300
		rc := 0
		if ok {
			rc = tryCountResults(body)
		}

		return testenv.ComponentResult{
			DurationMs: dur.Nanoseconds(),
			Code:       resp.StatusCode,
			OK:         ok,
			Error:      nil, // request/read failures would have panicked
			Op:         "search",
			Method:     "GET",
			URL:        u,
			Request:    nil, // no body for GET
			Response:   json.RawMessage(body),
			Extra: map[string]any{
				"iter":        iter,
				"resultCount": rc,
				"usedParams":  q.Encode(),
			},
		}
	}
}

// example execution: (log levels: full, name, basic)
// Use full for testing purposes and basic for benchmarking
// result is stored in benchmark_results in root directory
// $env:LOG_DETAIL = "full" go test -bench BenchmarkDiscovery -run ^$ -benchtime=100x -benchmem

func BenchmarkAASDR(b *testing.B) {
	mustHaveCompose(b)
	waitUntilHealthy(b)

	comp := NewDiscoveryBench(*seedFlag)
	testenv.BenchmarkComponent(b, comp)
}
