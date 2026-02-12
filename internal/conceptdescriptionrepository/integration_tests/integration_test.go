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

//nolint:all
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig represents the structure of your test configuration
type TestConfig struct {
	Method         string `json:"method"`
	Endpoint       string `json:"endpoint"`
	Data           string `json:"data,omitempty"`
	ShouldMatch    string `json:"shouldMatch,omitempty"`
	ExpectedStatus int    `json:"expectedStatus,omitempty"`
	Context        string `json:"context,omitempty"`
}

// loadTestConfig loads the test configuration from a JSON file
func loadTestConfig(filename string) ([]TestConfig, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	var configs []TestConfig
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&configs)
	return configs, err
}

// makeRequest performs an HTTP request based on the test config
func makeRequest(config TestConfig, stepNumber int) (string, error) {
	var req *http.Request
	var err error

	// Check if this is a file attachment upload endpoint
	if config.Method == "PUT" && strings.Contains(config.Endpoint, "/attachment") {
		return "", nil // Skip - will be handled by dedicated file tests
	}

	// Check if this is a file attachment download endpoint
	if config.Method == "GET" && strings.Contains(config.Endpoint, "/attachment") {
		return "", nil // Skip - will be handled by dedicated file tests
	}

	switch config.Method {
	case "GET":
		req, err = http.NewRequest("GET", config.Endpoint, nil)
	case "POST":
		if config.Data != "" {
			data, err := os.ReadFile(config.Data)
			if err != nil {
				return "", fmt.Errorf("failed to read data file: %v", err)
			}
			req, err = http.NewRequest("POST", config.Endpoint, bytes.NewBuffer(data))
			if err != nil {
				return "", err
			}

			req.Header.Set("Content-Type", "application/json")
		} else {
			req, err = http.NewRequest("POST", config.Endpoint, nil)
			if err != nil {
				return "", err
			}
		}
	case "DELETE":
		req, err = http.NewRequest("DELETE", config.Endpoint, nil)
		if err != nil {
			return "", err
		}
	case "PUT":
		if config.Data != "" {
			data, err := os.ReadFile(config.Data)
			if err != nil {
				return "", fmt.Errorf("failed to read data file: %v", err)
			}
			req, err = http.NewRequest("PUT", config.Endpoint, bytes.NewBuffer(data))
			if err != nil {
				return "", err
			}
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, err = http.NewRequest("PUT", config.Endpoint, nil)
			if err != nil {
				return "", err
			}
		}
	case "PATCH":
		if config.Data != "" {
			data, err := os.ReadFile(config.Data)
			if err != nil {
				return "", fmt.Errorf("failed to read data file: %v", err)
			}
			req, err = http.NewRequest("PATCH", config.Endpoint, bytes.NewBuffer(data))
			if err != nil {
				return "", err
			}
			// If payload looks like a JSON object/array, use merge-patch media type
			trimmed := strings.TrimSpace(string(data))
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				req.Header.Set("Content-Type", "application/merge-patch+json")
			} else {
				req.Header.Set("Content-Type", "application/json")
			}
		} else {
			req, err = http.NewRequest("PATCH", config.Endpoint, nil)
			if err != nil {
				return "", err
			}
		}
	default:
		return "", fmt.Errorf("unsupported method: %s", config.Method)
	}

	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Log the error to a step-specific file for easier diagnosis
		// if stepNumber > 0 {
		// 	errLog := fmt.Sprintf("logs/REQUEST_STEP_%d.error.log", stepNumber)
		// 	_ = os.WriteFile(errLog, []byte(err.Error()), 0644) // ignore write error
		// }
		return "", err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != config.ExpectedStatus {
		logFile := fmt.Sprintf("logs/STEP_%d.log", stepNumber)
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			fmt.Fprintf(f, "Expected status %d but got %d\n", config.ExpectedStatus, resp.StatusCode) //nolint:errcheck
			body, _ := io.ReadAll(resp.Body)
			fmt.Fprintf(f, "Response body: %s\n", body) //nolint:errcheck
			_ = f.Close()
		}
		_, _ = fmt.Printf("Response status code: %d\n", resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Printf("Response body: %s\n", body)
		return "", fmt.Errorf("expected status %d but got %d", config.ExpectedStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// uploadFileAttachment uploads a file to the attachment endpoint
func uploadFileAttachment(endpoint string, filePath string, fileName string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the file field
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return 0, fmt.Errorf("failed to create form file: %v", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return 0, fmt.Errorf("failed to copy file: %v", err)
	}

	// Add the fileName field if provided
	if fileName != "" {
		if err := writer.WriteField("fileName", fileName); err != nil {
			return 0, fmt.Errorf("failed to write fileName field: %v", err)
		}
	}

	err = writer.Close()
	if err != nil {
		return 0, fmt.Errorf("failed to close writer: %v", err)
	}

	req, err := http.NewRequest("PUT", endpoint, body)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

// downloadFileAttachment downloads a file from the attachment endpoint and returns content and content-type
func downloadFileAttachment(endpoint string) ([]byte, string, int, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", resp.StatusCode, fmt.Errorf("failed to read response: %v", err)
	}

	contentType := resp.Header.Get("Content-Type")
	return content, contentType, resp.StatusCode, nil
}

// IntegrationTest runs the integration tests based on the config file
func TestIntegration(t *testing.T) {
	// Load test configuration
	configs, err := loadTestConfig("it_config.json")
	require.NoError(t, err, "Failed to load test config")

	// Ensure logs directory exists
	if err := os.Mkdir("logs", 0755); err != nil && !os.IsExist(err) {
		t.Fatalf("Failed to create logs directory: %v", err)
	}

	for i, config := range configs {
		context := "Not Provided"
		if config.Context != "" {
			context = config.Context
		}
		t.Run(fmt.Sprintf("Step_(%s)_%d_%s_%s", context, i+1, config.Method, config.Endpoint), func(t *testing.T) {
			response, err := makeRequest(config, i+1)
			require.NoError(t, err, "Request failed")

			if (config.Method == "GET" || config.Method == "POST") && config.ShouldMatch != "" {
				expected, err := os.ReadFile(config.ShouldMatch)
				require.NoError(t, err, "Failed to read expected response file")

				// Parse and compare JSON
				var expectedJSON, responseJSON interface{}
				err = json.Unmarshal(expected, &expectedJSON)
				require.NoError(t, err, "Failed to parse expected JSON")

				err = json.Unmarshal([]byte(response), &responseJSON)
				require.NoError(t, err, "Failed to parse response JSON")

				// Re-marshal and compare as JSON strings for consistent comparison
				expectedBytes, _ := json.Marshal(expectedJSON)
				responseBytes, _ := json.Marshal(responseJSON)
				expectedStr := string(expectedBytes)
				responseStr := string(responseBytes)

				if expectedStr != responseStr {
					logFile := fmt.Sprintf("logs/STEP_%d.log", i+1)
					f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
					if err == nil {
						fmt.Fprintf(f, "JSON mismatch:\nExpected: %s\nActual: %s\n", expectedStr, responseStr) //nolint:errcheck
						_ = f.Close()
					}
				}

				assert.JSONEq(t, expectedStr, responseStr, "Response does not match expected")
				t.Logf("Expected: %s", expectedBytes)
			}

			t.Logf("Response: %s", response)
		})
	}
}

// TestMain handles setup and teardown
func TestMain(m *testing.M) {
	// Teardown: Stop Docker Compose
	_, _ = fmt.Println("Stopping old Docker Compose...")
	cmd := exec.Command("docker", "compose", "-f", "docker_compose/docker_compose.yml", "down")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Printf("Failed to stop Docker Compose: %v\n", err)
	}

	// Setup: Start Docker Compose
	_, _ = fmt.Println("Starting Docker Compose...")
	cmd = exec.Command("docker", "compose", "-f", "docker_compose/docker_compose.yml", "up", "-d", "--build", "--remove-orphans")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Printf("Failed to start Docker Compose: %v\n", err)
		os.Exit(1)
	}

	// Wait for service to be healthy
	_, _ = fmt.Println("Waiting for service to be healthy...")
	if !waitForHealthCheck() {
		_, _ = fmt.Println("Health check failed, exiting")
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	os.Exit(code)
}

// waitForHealthCheck waits for the service to become healthy
func waitForHealthCheck() bool {
	maxRetries := 30
	retryDelay := 5 * time.Second

	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get("http://localhost:6004/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			_, _ = fmt.Println("Service is healthy")
			return true
		}

		_, _ = fmt.Printf("Health check failed (attempt %d/%d): %v\n", i+1, maxRetries, err)
		time.Sleep(retryDelay)
	}

	_, _ = fmt.Println("Health check timed out")
	return false
}
