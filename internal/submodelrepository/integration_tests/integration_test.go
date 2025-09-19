package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
}

// loadTestConfig loads the test configuration from a JSON file
func loadTestConfig(filename string) ([]TestConfig, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var configs []TestConfig
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&configs)
	return configs, err
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

	defer resp.Body.Close()

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

				// Parse and compare JSON
				var expectedJSON, responseJSON interface{}
				err = json.Unmarshal(expected, &expectedJSON)
				require.NoError(t, err, "Failed to parse expected JSON")

				err = json.Unmarshal([]byte(response), &responseJSON)
				require.NoError(t, err, "Failed to parse response JSON")

				assert.Equal(t, expectedJSON, responseJSON, "Response does not match expected")
			}

			t.Logf("Response: %s", response)
		})
	}
}

// TestMain handles setup and teardown
func TestMain(m *testing.M) {
	// Setup: Start Docker Compose
	// fmt.Println("Starting Docker Compose...")
	// cmd := exec.Command("docker-compose", "-f", "docker_compose/docker_compose.yml", "up", "-d", "--build")
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	// if err := cmd.Run(); err != nil {
	// 	fmt.Printf("Failed to start Docker Compose: %v\n", err)
	// 	os.Exit(1)
	// }

	// Create DB Connection here
	sql, err := sql.Open("postgres", "postgres://admin:admin123@127.0.0.1:5432/basyxTestDB?sslmode=disable")

	if err != nil {
		fmt.Printf("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	//wait for 5sec to ensure that the DB is ready
	// time.Sleep(5 * time.Second)

	dir, osErr := os.Getwd()

	if osErr != nil {
		fmt.Printf("Failed to get working directory: %v\n", osErr)
		os.Exit(1)
	}

	queryString, fileError := os.ReadFile(dir + "/sql/demoSubmodel.sql")

	if fileError != nil {
		fmt.Printf("Failed to read SQL file: %v\n", fileError)
		os.Exit(1)
	}

	_, err = sql.Exec(string(queryString))

	if err != nil {
		fmt.Printf("Failed to execute SQL script: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Database initialized successfully.")

	// Run tests
	code := m.Run()

	// Teardown: Stop Docker Compose
	// fmt.Println("Stopping Docker Compose...")
	// cmd = exec.Command("docker-compose", "-f", "docker_compose/docker_compose.yml", "down")
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	// if err := cmd.Run(); err != nil {
	// 	fmt.Printf("Failed to stop Docker Compose: %v\n", err)
	// }

	os.Exit(code)
}
