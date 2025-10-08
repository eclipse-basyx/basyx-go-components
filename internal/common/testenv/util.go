package testenv

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"testing"
	"time"

	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi/go"
)

type LogDetail int

const (
	BaseURL                     = "http://127.0.0.1:5004"
	LogNameAndRuntime LogDetail = iota // only component + runtime
	LogBasic                           // + op, code, ok
	LogFull                            // + method, url, err, request/response, extra
)

func envLogDetail() LogDetail {
	switch os.Getenv("LOG_DETAIL") {
	case "name":
		return LogNameAndRuntime
	case "basic":
		return LogBasic
	default:
		return LogFull
	}
}

func makeLogRecord(iter int, componentName string, r ComponentResult, level LogDetail) LogRecord {
	lr := LogRecord{
		Iter:       iter,
		Component:  componentName,
		DurationMs: r.DurationMs,
	}
	if level >= LogBasic {
		lr.Op = r.Op
		lr.Code = r.Code
		lr.OK = r.OK
	}
	if level >= LogFull {
		lr.Method = r.Method
		lr.URL = r.URL
		if r.Error != nil {
			lr.Error = r.Error.Error()
		}
		lr.Request = r.Request
		lr.Response = r.Response
		lr.Extra = r.Extra
	}
	return lr
}

type ComponentBench interface {
	Name() string
	DoOne(iter int) ComponentResult
}

type ComponentResult struct {
	DurationMs int64
	Code       int
	OK         bool
	Error      error

	Op     string
	Method string
	URL    string

	Request  json.RawMessage
	Response json.RawMessage
	Extra    map[string]any
}

type LogRecord struct {
	Iter       int    `json:"iter"`
	Component  string `json:"component"`
	DurationMs int64  `json:"duration_ms"`

	Op   string `json:"op,omitempty"`
	Code int    `json:"code,omitempty"`
	OK   bool   `json:"ok,omitempty"`

	Method   string          `json:"method,omitempty"`
	URL      string          `json:"url,omitempty"`
	Error    string          `json:"error,omitempty"`
	Request  json.RawMessage `json:"request,omitempty"`
	Response json.RawMessage `json:"response,omitempty"`
	Extra    map[string]any  `json:"extra,omitempty"`
}

func BenchmarkComponent(b *testing.B, comp ComponentBench) {
	logDetail := envLogDetail()
	logs := make([]LogRecord, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := comp.DoOne(i)
		logs = append(logs, makeLogRecord(i, comp.Name(), res, logDetail))
	}
	b.StopTimer()

	filename := fmt.Sprintf("%s_bench.json", comp.Name())
	if f, err := os.Create(filename); err == nil {
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		_ = enc.Encode(logs)
		_ = f.Close()
		b.Logf("wrote %s with %d records (detail=%v)", filename, len(logs), logDetail)
	} else {
		b.Logf("could not write JSON log: %v", err)
	}
}

func RandomHex(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func HTTPClient() *http.Client { return &http.Client{Timeout: 20 * time.Second} }

func PostJSONRaw(url string, body any) (data []byte, status int, err error) {
	var r io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, 0, e
		}
		r = bytes.NewReader(b)
	}
	req, e := http.NewRequest("POST", url, r)
	if e != nil {
		return nil, 0, e
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := HTTPClient().Do(req)
	if e != nil {
		return nil, 0, e
	}
	defer resp.Body.Close()
	data, e = io.ReadAll(resp.Body)
	return data, resp.StatusCode, e
}

func GetRaw(url string) (data []byte, status int, err error) {
	resp, e := HTTPClient().Get(url)
	if e != nil {
		return nil, 0, e
	}
	defer resp.Body.Close()
	data, e = io.ReadAll(resp.Body)
	return data, resp.StatusCode, e
}

func DeleteRaw(url string) (data []byte, status int, err error) {
	req, e := http.NewRequest("DELETE", url, nil)
	if e != nil {
		return nil, 0, e
	}
	resp, e := HTTPClient().Do(req)
	if e != nil {
		return nil, 0, e
	}
	defer resp.Body.Close()
	data, e = io.ReadAll(resp.Body)
	return data, resp.StatusCode, e
}

func PostJSONExpect(t testing.TB, url string, body any, expect int) []byte {
	t.Helper()
	data, st, err := PostJSONRaw(url, body)
	if err != nil {
		t.Fatalf("POST %s error: %v", url, err)
	}
	if st != expect {
		t.Fatalf("POST %s expected %d got %d: %s", url, expect, st, string(data))
	}
	return data
}

func GetExpect(t testing.TB, url string, expect int) []byte {
	t.Helper()
	data, st, err := GetRaw(url)
	if err != nil {
		t.Fatalf("GET %s error: %v", url, err)
	}
	if st != expect {
		t.Fatalf("GET %s expected %d got %d: %s", url, expect, st, string(data))
	}
	return data
}

func DeleteExpect(t testing.TB, url string, expect int) []byte {
	t.Helper()
	data, st, err := DeleteRaw(url)
	if err != nil {
		t.Fatalf("DELETE %s error: %v", url, err)
	}
	if st != expect {
		t.Fatalf("DELETE %s expected %d got %d: %s", url, expect, st, string(data))
	}
	return data
}

func FindCompose() (bin string, args []string, err error) {
	if _, e := exec.LookPath("docker"); e == nil {
		return "docker", []string{"compose"}, nil
	}
	if _, e := exec.LookPath("podman"); e == nil {
		return "podman", []string{"compose"}, nil
	}
	return "", nil, errors.New("neither docker nor podman found on PATH")
}

func RunCompose(ctx context.Context, base string, args ...string) error {
	cmd := exec.CommandContext(ctx, base, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func WaitHealthy(t testing.TB, url string, maxWait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(maxWait)
	backoff := time.Second
	for {
		resp, err := HTTPClient().Get(url)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				_ = resp.Body.Close()
				return
			}
			_ = resp.Body.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("service not healthy at %s within %s", url, maxWait)
		}
		time.Sleep(backoff)
		if backoff < 5*time.Second {
			backoff += 500 * time.Millisecond
		}
	}
}

func BuildNameValuesMap(in []openapi.SpecificAssetId) map[string][]string {
	m := map[string][]string{}
	for _, s := range in {
		m[s.Name] = append(m[s.Name], s.Value)
	}
	for k := range m {
		sort.Strings(m[k])
	}
	return m
}
