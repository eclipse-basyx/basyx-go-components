package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/stretchr/testify/require"
)

// TestConfig represents the structure of your test configuration
type TestConfig struct {
	Method         string `json:"method"`
	Endpoint       string `json:"endpoint"`
	Data           string `json:"data,omitempty"`
	ShouldMatch    string `json:"shouldMatch,omitempty"`
	ExpectedStatus int    `json:"expectedStatus,omitempty"`
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

// assertSameJSONContent compares two JSON strings semantically, ignoring order
func assertSameJSONContent(t *testing.T, expected, actual string) {
	var expectedData, actualData interface{}

	err := json.Unmarshal([]byte(expected), &expectedData)
	require.NoError(t, err, "Failed to parse expected JSON")

	err = json.Unmarshal([]byte(actual), &actualData)
	require.NoError(t, err, "Failed to parse actual JSON")

	// Use custom comparison that handles array ordering
	if !deepEqualIgnoreOrder(expectedData, actualData) {
		// Pretty print for better debugging
		expectedBytes, _ := json.MarshalIndent(expectedData, "", "  ")
		actualBytes, _ := json.MarshalIndent(actualData, "", "  ")

		t.Errorf("JSON content does not match semantically")
		t.Logf("Expected JSON:\n%s", string(expectedBytes))
		t.Logf("Actual JSON:\n%s", string(actualBytes))
	}
}

// deepEqualIgnoreOrder compares two interfaces recursively, ignoring array order
func deepEqualIgnoreOrder(expected, actual interface{}) bool {
	switch exp := expected.(type) {
	case map[string]interface{}:
		act, ok := actual.(map[string]interface{})
		if !ok || len(exp) != len(act) {
			return false
		}
		for key, expVal := range exp {
			actVal, exists := act[key]
			if !exists || !deepEqualIgnoreOrder(expVal, actVal) {
				return false
			}
		}
		return true
	case []interface{}:
		act, ok := actual.([]interface{})
		if !ok || len(exp) != len(act) {
			return false
		}

		// Create a boolean array to track which actual items have been matched
		matched := make([]bool, len(act))

		// For each expected item, find a matching actual item
		for _, expItem := range exp {
			found := false
			for j, actItem := range act {
				if !matched[j] && deepEqualIgnoreOrder(expItem, actItem) {
					matched[j] = true
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}

		// Check if all actual items were matched
		for _, wasMatched := range matched {
			if !wasMatched {
				return false
			}
		}

		return true
	default:
		// For primitive types, use direct comparison
		return expected == actual
	}
}

// makeRequest performs an HTTP request based on the test config
func makeRequest(config TestConfig) (string, error) {
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
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, err = http.NewRequest("POST", config.Endpoint, nil)
			if err != nil {
				return "", err
			}
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
	case "DELETE":
		req, err = http.NewRequest("DELETE", config.Endpoint, nil)
		if err != nil {
			return "", err
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
		return "", err
	}

    defer func() {
        _ = resp.Body.Close()
    }()

	if resp.StatusCode != config.ExpectedStatus {
		fmt.Printf("Response status code: %d\n", resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Response body: %s\n", body)
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

	// Wait for services to be ready (adjust as needed)
	time.Sleep(15 * time.Second) // Wait for Docker Compose services

	for i, config := range configs {
		t.Run(fmt.Sprintf("Step_%d_%s_%s", i+1, config.Method, config.Endpoint), func(t *testing.T) {
			response, err := makeRequest(config)
			require.NoError(t, err, "Request failed")

			if config.Method == "GET" && config.ShouldMatch != "" {
				expected, err := os.ReadFile(config.ShouldMatch)
				require.NoError(t, err, "Failed to read expected response file")

				assertSameJSONContent(t, string(expected), response)
			}

			t.Logf("Response: %s", response)
		})
	}
}

// TestMain handles setup and teardown
func TestMain(m *testing.M) {

	executable, _, err := testenv.FindCompose()
	if err != nil {
		fmt.Println("compose engine not found:", err)
		os.Exit(m.Run())
	}

	// Setup: Start Docker Compose
	fmt.Println("Starting Docker Compose...")
	cmd := exec.Command(executable, "compose", "-f", "docker_compose/docker_compose.yml", "up", "-d", "--build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to start Docker Compose: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Database initialized successfully.")

	// Run tests
	code := m.Run()

	// Teardown: Stop Docker Compose
	fmt.Println("Stopping Docker Compose...")
	cmd = exec.Command(executable, "compose", "-f", "docker_compose/docker_compose.yml", "down")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to stop Docker Compose: %v\n", err)
	}

	os.Exit(code)
}
