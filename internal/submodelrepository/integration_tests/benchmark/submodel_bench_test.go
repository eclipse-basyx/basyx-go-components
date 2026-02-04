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

// Package bench provides a benchmark for the submodel repository service.
//
//nolint:all
package bench

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// Shared HTTP client with connection pooling for high-concurrency benchmarks.
// This prevents EOF errors caused by creating new connections for each request.
var benchHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
		MaxConnsPerHost:     200,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	},
}

// postJSON sends a POST request using the shared client with connection pooling.
func postJSON(url string, body any) ([]byte, int, error) {
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
	resp, e := benchHTTPClient.Do(req)
	if e != nil {
		return nil, 0, e
	}
	defer func() { _ = resp.Body.Close() }()
	data, e := io.ReadAll(resp.Body)
	return data, resp.StatusCode, e
}

// getRaw sends a GET request using the shared client with connection pooling.
func getRaw(url string) ([]byte, int, error) {
	resp, e := benchHTTPClient.Get(url)
	if e != nil {
		return nil, 0, e
	}
	defer func() { _ = resp.Body.Close() }()
	data, e := io.ReadAll(resp.Body)
	return data, resp.StatusCode, e
}

var (
	operationFlag   = flag.String("operation", "post", "Operation to benchmark: 'post' or 'get'")
	baseURLFlag     = flag.String("baseurl", "http://127.0.0.1:5004", "Base URL of the service")
	threadCountFlag = flag.Int("threads", 10, "Number of concurrent threads")
	submodelCount   = flag.Int("count", 1000, "Number of submodels to process")
	idPrefixFlag    = flag.String("idprefix", "submodelID2_%d", "ID prefix template (use %d for auto-increment)")
	logFailuresFlag = flag.Bool("logfailures", true, "Log failed requests with details")
	maxRetriesFlag  = flag.Int("retries", 3, "Maximum number of retries for failed requests")
	retryDelayFlag  = flag.Duration("retrydelay", 100*time.Millisecond, "Delay between retries")
	benchmarkRun    = false
)

func ensureSingleRun() bool {
	if benchmarkRun {
		return false
	}
	benchmarkRun = true
	return true
}

type benchmarkStats struct {
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	totalDuration      int64
	startTime          time.Time
	endTime            time.Time
}

func generateSubmodelID(index int) string {
	return fmt.Sprintf(*idPrefixFlag, index)
}

func generateSubmodel(smID string) map[string]any {
	var sm map[string]any
	// read json file bench_sm.json
	file, err := os.ReadFile("bench_sm.json")
	if err != nil {
		log.Fatalf("Failed to read benchmark submodel file: %v", err)
	}
	err = json.Unmarshal(file, &sm)
	if err != nil {
		log.Fatalf("Failed to unmarshal benchmark submodel JSON: %v", err)
	}
	sm["id"] = smID
	return sm
}

func executePostRequest(index int, stats *benchmarkStats) {
	submodelID := generateSubmodelID(index)
	submodel := generateSubmodel(submodelID)
	
	baseURL := *baseURLFlag
	if !strings.HasSuffix(baseURL, "/submodels") {
		baseURL = baseURL + "/submodels"
	}
	url := baseURL

	atomic.AddInt64(&stats.totalRequests, 1)

	var respBody []byte
	var code int
	var err error
	var duration time.Duration
	
	for attempt := 0; attempt <= *maxRetriesFlag; attempt++ {
		if attempt > 0 {
			time.Sleep(*retryDelayFlag)
			if *logFailuresFlag {
				log.Printf("[RETRY] POST #%d | Attempt %d/%d | ID: %s", index, attempt, *maxRetriesFlag, submodelID)
			}
		}
		
		start := time.Now()
		respBody, code, err = postJSON(url, submodel)
		duration = time.Since(start)

		if code == 201 && err == nil {
			break
		}
		
		if attempt < *maxRetriesFlag && (err != nil || code >= 500) {
			continue
		}
	}

	atomic.AddInt64(&stats.totalDuration, duration.Microseconds())

	if code == 201 && err == nil {
		atomic.AddInt64(&stats.successfulRequests, 1)
	} else {
		atomic.AddInt64(&stats.failedRequests, 1)
		if *logFailuresFlag {
			reqJSON, _ := json.MarshalIndent(submodel, "", "  ")
			if err != nil {
				log.Printf("[FAILED] POST #%d | URL: %s | ID: %s | Error: %v | Duration: %v\nRequest:\n%s", 
					index, url, submodelID, err, duration, string(reqJSON))
			} else {
				log.Printf("[FAILED] POST #%d | URL: %s | ID: %s | Status: %d | Duration: %v\nRequest:\n%s\nResponse:\n%s", 
					index, url, submodelID, code, duration, string(reqJSON), string(respBody))
			}
		}
	}
}

func executeGetRequest(index int, stats *benchmarkStats) {
	submodelID := generateSubmodelID(index)
	encodedID := common.EncodeString(submodelID)
	
	baseURL := *baseURLFlag
	if !strings.HasSuffix(baseURL, "/submodels") {
		baseURL = baseURL + "/submodels"
	}
	url := fmt.Sprintf("%s/%s", baseURL, encodedID)

	atomic.AddInt64(&stats.totalRequests, 1)

	var respBody []byte
	var code int
	var err error
	var duration time.Duration

	for attempt := 0; attempt <= *maxRetriesFlag; attempt++ {
		if attempt > 0 {
			time.Sleep(*retryDelayFlag)
			if *logFailuresFlag {
				log.Printf("[RETRY] GET #%d | Attempt %d/%d | ID: %s", index, attempt, *maxRetriesFlag, submodelID)
			}
		}

		start := time.Now()
		respBody, code, err = getRaw(url)
		duration = time.Since(start)

		if code == 200 && err == nil {
			break
		}

		if attempt < *maxRetriesFlag && (err != nil || code >= 500) {
			continue
		}
	}

	atomic.AddInt64(&stats.totalDuration, duration.Microseconds())

	if code == 200 && err == nil {
		atomic.AddInt64(&stats.successfulRequests, 1)
	} else {
		atomic.AddInt64(&stats.failedRequests, 1)
		if *logFailuresFlag {
			if err != nil {
				log.Printf("[FAILED] GET #%d | URL: %s | ID: %s | Error: %v | Duration: %v", index, url, submodelID, err, duration)
			} else {
				log.Printf("[FAILED] GET #%d | URL: %s | ID: %s | Status: %d | Duration: %v\nResponse:\n%s", 
					index, url, submodelID, code, duration, string(respBody))
			}
		}
	}
}

func runBenchmark() {
	stats := &benchmarkStats{
		startTime: time.Now(),
	}

	var counter atomic.Int64
	var wg sync.WaitGroup

	log.Printf("Starting %s benchmark with %d threads, %d submodels, baseURL=%s, idPrefix=%s",
		*operationFlag, *threadCountFlag, *submodelCount, *baseURLFlag, *idPrefixFlag)

	for t := 0; t < *threadCountFlag; t++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(counter.Add(1) - 1)
				if i >= *submodelCount {
					break
				}

				if *operationFlag == "post" {
					executePostRequest(i, stats)
				} else {
					executeGetRequest(i, stats)
				}
			}
		}()
	}

	wg.Wait()
	stats.endTime = time.Now()

	printStatistics(stats)
}

func printStatistics(stats *benchmarkStats) {
	totalTime := stats.endTime.Sub(stats.startTime)
	avgDuration := float64(stats.totalDuration) / float64(stats.totalRequests)
	throughput := float64(stats.totalRequests) / totalTime.Seconds()
	successRate := float64(stats.successfulRequests) / float64(stats.totalRequests) * 100

	log.Println("=" + strings.Repeat("=", 79))
	log.Println("BENCHMARK RESULTS")
	log.Println("=" + strings.Repeat("=", 79))
	log.Printf("Operation:           %s", strings.ToUpper(*operationFlag))
	log.Printf("Total Requests:      %d", stats.totalRequests)
	log.Printf("Successful:          %d", stats.successfulRequests)
	log.Printf("Failed:              %d", stats.failedRequests)
	log.Printf("Success Rate:        %.2f%%", successRate)
	log.Printf("Total Time:          %v", totalTime)
	log.Printf("Average Latency:     %.3f µs", avgDuration)
	log.Printf("Throughput:          %.2f ops/sec", throughput)
	log.Println("=" + strings.Repeat("=", 79))
}

func TestBenchmarkSubmodelRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark in short mode")
	}

	if !ensureSingleRun() {
		t.Skip("Benchmark already executed")
		return
	}

	runBenchmark()
	t.Log("Benchmark completed successfully")
}
