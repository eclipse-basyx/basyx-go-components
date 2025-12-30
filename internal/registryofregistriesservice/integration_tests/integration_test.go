//nolint:all
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

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
	Action         string `json:"action,omitempty"`
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

			// Log outgoing request for debugging for requests with bodies
			if stepNumber > 0 {
				reqLog := fmt.Sprintf("logs/REQUEST_STEP_%d.log", stepNumber)
				f, ferr := os.OpenFile(reqLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
				if ferr == nil {
					fmt.Fprintf(f, "%s %s\n", req.Method, req.URL.String()) //nolint:errcheck
					for k, v := range req.Header {
						fmt.Fprintf(f, "%s: %s\n", k, strings.Join(v, ",")) //nolint:errcheck
					}
					// If we have a path to a data file, log its contents for easier debugging
					if config.Data != "" {
						if data, rerr := os.ReadFile(config.Data); rerr == nil {
							fmt.Fprintf(f, "\n%s\n", string(data)) //nolint:errcheck
						}
					}
					_ = f.Close()
				}

				// Dump the raw outgoing HTTP request — this may consume req.Body, so prefer the data file for content
				if dump, derr := httputil.DumpRequestOut(req, false); derr == nil {
					rawFile := fmt.Sprintf("logs/RAW_REQUEST_STEP_%d.dump", stepNumber)
					_ = os.WriteFile(rawFile, dump, 0644) // ignore write error
				}
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
		if stepNumber > 0 {
			errLog := fmt.Sprintf("logs/REQUEST_STEP_%d.error.log", stepNumber)
			_ = os.WriteFile(errLog, []byte(err.Error()), 0644) // ignore write error
		}
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
			// Handle special actions from config
			if config.Action == "DELETE_ALL_REGISTRY_DESCRIPTORS" {
				// Fetch current descriptors
				body, err := makeRequest(TestConfig{Method: "GET", Endpoint: "http://127.0.0.1:5004/registry-descriptors", ExpectedStatus: 200}, i+1)
				require.NoError(t, err)

				var list struct {
					Result []struct {
						ID string `json:"id"`
					} `json:"Result"`
				}
				err = json.Unmarshal([]byte(body), &list)
				require.NoError(t, err)

				// Delete each descriptor by base64url-encoded id
				for _, item := range list.Result {
					enc := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
					_, err := makeRequest(TestConfig{Method: "DELETE", Endpoint: fmt.Sprintf("http://127.0.0.1:5004/registry-descriptors/%s", enc), ExpectedStatus: 204}, i+1)
					require.NoError(t, err)
				}
				return
			}

			response, err := makeRequest(config, i+1)
			require.NoError(t, err, "Request failed")

			if config.Method == "GET" && config.ShouldMatch != "" {
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
	// Setup: Start Docker Compose
	_, _ = fmt.Println("Starting Docker Compose...")
	cmd := exec.Command("podman", "compose", "-f", "docker_compose/docker-compose.yml", "up", "-d", "--build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Printf("Failed to start Docker Compose: %v\n", err)
		os.Exit(1)
	}

	_, _ = fmt.Println("Database initialized successfully.")

	time.Sleep(10 * time.Second)

	// Run tests
	code := m.Run()

	// Teardown: Stop Docker Compose
	_, _ = fmt.Println("Stopping Docker Compose...")
	cmd = exec.Command("podman", "compose", "-f", "docker_compose/docker-compose.yml", "down")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Printf("Failed to stop Docker Compose: %v\n", err)
	}

	os.Exit(code)
}
