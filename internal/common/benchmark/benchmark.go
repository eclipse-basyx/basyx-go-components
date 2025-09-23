package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

type BenchmarkStats struct {
	mu                 sync.Mutex
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	totalResponseTime  time.Duration
	startTime          time.Time
	endTime            time.Time
}

var stats BenchmarkStats

var threads = 28

// Total Number of elements = submodelElementCount*nestingLevel
var submodelElementCount = 1_248
var nestingLevel = 50
var shouldPost = true
var shouldGet = true
var shouldDelete = true
var baseURL = "http://localhost:5004"

// Shared HTTP client for reuse and performance
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	},
}

func sendRequest(method, url string, body []byte) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Panic in sendRequest: %v\n", r)
		}
	}()
	start := time.Now()
	var resp *http.Response
	var err error
	if method == "POST" {
		resp, err = httpClient.Post(url, "application/json", bytes.NewBuffer(body))
	} else if method == "GET" {
		resp, err = httpClient.Get(url)
	} else if method == "DELETE" {
		req, reqErr := http.NewRequest(http.MethodDelete, url, nil)
		if reqErr != nil {
			fmt.Println("Error creating DELETE request:", reqErr)
			stats.mu.Lock()
			stats.totalRequests++
			stats.failedRequests++
			stats.mu.Unlock()
			return
		}
		resp, err = httpClient.Do(req)
	}
	duration := time.Since(start)

	stats.mu.Lock()
	stats.totalRequests++
	stats.totalResponseTime += duration
	if err != nil {
		fmt.Printf("Error sending %s request: %v\n", method, err)
		stats.failedRequests++
	} else {
		defer resp.Body.Close()
		if (method == "DELETE" && resp.StatusCode == 204) || (method != "DELETE" && resp.StatusCode >= 200 && resp.StatusCode < 300) {
			stats.successfulRequests++
		} else {
			stats.failedRequests++
			fmt.Printf("Failed %s request: Status %s\n", method, resp.Status)
		}
	}
	stats.mu.Unlock()
}

func runBenchmarkPhase(phase string, action func(int, []byte)) {
	if (phase == "POST" && !shouldPost) || (phase == "GET" && !shouldGet) || (phase == "DELETE" && !shouldDelete) {
		return
	}
	stats.mu.Lock()
	stats.totalRequests = 0
	stats.successfulRequests = 0
	stats.failedRequests = 0
	stats.totalResponseTime = 0
	stats.startTime = time.Now()
	stats.mu.Unlock()
	var counter atomic.Int64
	var wg sync.WaitGroup
	numGoroutines := threads
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(counter.Add(1) - 1)
				if i >= submodelElementCount {
					break
				}
				var body []byte
				if phase == "POST" {
					sme := generateNestedSubmodelElements(i)
					var err error
					body, err = json.Marshal(sme)
					if err != nil {
						fmt.Println("Error marshaling JSON:", err)
						continue
					}
				}
				action(i, body)
				// Print progress every 1000 requests to reduce I/O
				if i%1000 == 0 {
					fmt.Printf("Progress (%s): %d\n", phase, i)
				}
			}
		}()
	}
	wg.Wait()
}

func main() {
	stats.startTime = time.Now()

	// POST phase
	runBenchmarkPhase("POST", func(i int, body []byte) {
		url := baseURL + "/submodels/aHR0cDovL2llc2UuZnJhdW5ob2Zlci5kZS9pZC9zbS9EZW1vU3VibW9kZWw/submodel-elements"
		sendRequest("POST", url, body)
	})

	printStats("POST")
	if shouldPost && shouldGet {
		// Wait 15 Seconds to allow DB to settle
		fmt.Println("Waiting 15 seconds to allow DB to settle...")
		time.Sleep(15 * time.Second)
		fmt.Println("Starting GET phase...")
	}
	// GET phase
	runBenchmarkPhase("GET", func(i int, body []byte) {
		url := baseURL + "/submodels/aHR0cDovL2llc2UuZnJhdW5ob2Zlci5kZS9pZC9zbS9EZW1vU3VibW9kZWw/submodel-elements/Level" + strconv.Itoa(i)
		sendRequest("GET", url, nil)
	})

	printStats("GET")

	// DELETE phase
	runBenchmarkPhase("DELETE", func(i int, body []byte) {
		url := baseURL + "/submodels/aHR0cDovL2llc2UuZnJhdW5ob2Zlci5kZS9pZC9zbS9EZW1vU3VibW9kZWw/submodel-elements/Level" + strconv.Itoa(i)
		sendRequest("DELETE", url, nil)
	})

	printStats("DELETE")
}

func printStats(phase string) {
	stats.mu.Lock()
	stats.endTime = time.Now()
	totalTime := stats.endTime.Sub(stats.startTime)
	fmt.Printf("\nBenchmark Statistics After %s:\n", phase)
	fmt.Printf("Total Time: %v\n", totalTime)
	fmt.Printf("Total Requests: %d\n", stats.totalRequests)
	fmt.Printf("Successful Requests: %d\n", stats.successfulRequests)
	fmt.Printf("Failed Requests: %d\n", stats.failedRequests)
	if stats.totalRequests > 0 {
		avgResponseTime := stats.totalResponseTime / time.Duration(stats.totalRequests)
		fmt.Printf("Average Response Time: %v\n", avgResponseTime)
		throughput := float64(stats.totalRequests) / totalTime.Seconds()
		fmt.Printf("Throughput: %.2f requests/second\n", throughput)
	}
	stats.mu.Unlock()
}

func generateNestedSubmodelElements(level int) gen.SubmodelElement {
	var SubmodelElement = gen.SubmodelElementCollection{
		IdShort:   "Level" + strconv.Itoa(level),
		ModelType: "SubmodelElementCollection",
	}
	LastElement := &SubmodelElement
	for i := 0; i < nestingLevel-1; i++ {
		// Add logic for nested elements here if needed
		nested := gen.SubmodelElementCollection{
			IdShort:   "NestedLevel" + strconv.Itoa(i),
			ModelType: "SubmodelElementCollection",
		}

		LastElement.Value = append(LastElement.Value, &nested)
		LastElement = &nested
	}
	return &SubmodelElement
}
