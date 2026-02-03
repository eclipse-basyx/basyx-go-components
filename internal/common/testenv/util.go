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

// Package testenv provides testing utilities for integration tests and benchmarks.
// It includes helpers for HTTP requests, component benchmarking, Docker Compose management,
// and health checking of services.
// nolint:all
package testenv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
)

// LogDetail controls the verbosity level of benchmark logging.
type LogDetail int

const (
	// BaseURL is the default base URL for test services.
	BaseURL = "http://127.0.0.1:5004"

	// LogNameAndRuntime logs only component name and runtime duration.
	LogNameAndRuntime LogDetail = iota
	// LogBasic logs component, runtime, operation, status code, and success flag.
	LogBasic
	// LogFull logs all details including method, URL, errors, request/response bodies, and extra metadata.
	LogFull
)

// envLogDetail reads the LOG_DETAIL environment variable to determine the logging verbosity.
// Valid values are "name" (LogNameAndRuntime), "basic" (LogBasic), or any other value for LogFull (default).
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

// makeLogRecord creates a LogRecord from a ComponentResult based on the specified logging level.
// Lower log levels include less detail to reduce output size.
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
		lr.Extra = r.Extra
	}
	if level >= LogFull {
		lr.Method = r.Method
		lr.URL = r.URL
		if r.Error != nil {
			lr.Error = r.Error.Error()
		}
	}
	return lr
}

// ComponentBench is an interface for benchmarkable components.
// Implementations should define a Name and perform one iteration of work in DoOne.
type ComponentBench interface {
	// Name returns the name of the component being benchmarked.
	Name() string
	// DoOne performs one benchmark iteration and returns the result.
	DoOne(iter int) ComponentResult
}

// ComponentResult contains the results of a single benchmark iteration.
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

// LogRecord represents a single benchmark log entry to be written to JSON output.
type LogRecord struct {
	Iter       int    `json:"iter"`
	Component  string `json:"component"`
	DurationMs int64  `json:"duration_ns"`

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

// findProjectRoot locates the project root directory by searching upward for a go.mod file.
// Returns an error if no go.mod file is found.
func findProjectRoot() (string, error) {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

// BenchmarkComponent runs a benchmark for the given component and writes results to a JSON file.
// Results are written to benchmark_results/<component_name>_bench.json in the project root.
// The LOG_DETAIL environment variable controls the verbosity of logged data.
func BenchmarkComponent(b *testing.B, comp ComponentBench) {
	logDetail := envLogDetail()
	logs := make([]LogRecord, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := comp.DoOne(i)
		logs = append(logs, makeLogRecord(i, comp.Name(), res, logDetail))
	}
	b.StopTimer()

	root, err := findProjectRoot()
	if err != nil {
		b.Fatalf("could not locate project root: %v", err)
	}

	resultsDir := filepath.Join(root, "benchmark_results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		b.Fatalf("failed to create results directory: %v", err)
	}

	filename := filepath.Join(resultsDir, fmt.Sprintf("%s_bench.json", comp.Name()))
	f, err := os.Create(filename)
	if err != nil {
		b.Fatalf("could not create benchmark file: %v", err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(logs); err != nil {
		b.Fatalf("failed to encode logs: %v", err)
	}

	b.Logf("wrote %s with %d records (detail=%v)", filename, len(logs), logDetail)
}

// HTTPClient returns a configured HTTP client with a 20-second timeout.
func HTTPClient() *http.Client { return &http.Client{Timeout: 20 * time.Second} }

// PostJSONRaw sends a POST request with JSON body to the specified URL.
// Returns the response body, status code, and any error encountered.
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
	defer func() { _ = resp.Body.Close() }()
	data, e = io.ReadAll(resp.Body)
	return data, resp.StatusCode, e
}

// GetRaw sends a GET request to the specified URL.
// Returns the response body, status code, and any error encountered.
func GetRaw(url string) (data []byte, status int, err error) {
	resp, e := HTTPClient().Get(url)
	if e != nil {
		return nil, 0, e
	}
	defer func() { _ = resp.Body.Close() }()
	data, e = io.ReadAll(resp.Body)
	return data, resp.StatusCode, e
}

// DeleteRaw sends a DELETE request to the specified URL.
// Returns the response body, status code, and any error encountered.
func DeleteRaw(url string) (data []byte, status int, err error) {
	req, e := http.NewRequest("DELETE", url, nil)
	if e != nil {
		return nil, 0, e
	}
	resp, e := HTTPClient().Do(req)
	if e != nil {
		return nil, 0, e
	}
	defer func() { _ = resp.Body.Close() }()
	data, e = io.ReadAll(resp.Body)
	return data, resp.StatusCode, e
}

// PostJSONExpect sends a POST request with JSON body and expects a specific status code.
// Fails the test if the status code doesn't match or an error occurs.
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

// GetExpect sends a GET request and expects a specific status code.
// Fails the test if the status code doesn't match or an error occurs.
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

// DeleteExpect sends a DELETE request and expects a specific status code.
// Fails the test if the status code doesn't match or an error occurs.
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

// FindCompose searches for docker or podman on the PATH and returns the binary name and compose subcommand.
// Returns an error if neither docker nor podman is found.
func FindCompose() (bin string, args []string, err error) {
	if _, e := exec.LookPath("docker"); e == nil {
		return "docker", []string{"compose"}, nil
	}
	if _, e := exec.LookPath("podman"); e == nil {
		return "podman", []string{"compose"}, nil
	}
	return "", nil, errors.New("neither docker nor podman found on PATH")
}

// RunCompose executes a Docker Compose command with the given base command and arguments.
// Streams stdout and stderr to the current process's output streams.
func RunCompose(ctx context.Context, base string, args ...string) error {
	cmd := exec.CommandContext(ctx, base, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WaitHealthy polls the given URL until it returns HTTP 200 or the timeout is reached.
// Uses exponential backoff (starting at 1 second, max 5 seconds) between attempts.
// Fails the test if the service is not healthy within maxWait duration.
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

// BuildNameValuesMap converts a slice of SpecificAssetID into a map of name to sorted values.
// Values for each name are sorted alphabetically for consistent comparison.
func BuildNameValuesMap(in []types.ISpecificAssetID) map[string][]string {
	m := map[string][]string{}
	for _, s := range in {
		m[s.Name()] = append(m[s.Name()], s.Value())
	}
	for k := range m {
		sort.Strings(m[k])
	}
	return m
}
