package main

import (
	"bytes"
	"encoding/base64"
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

	// errcheck: handle close error
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
		name := fmt.Sprintf("Step_%d_%s_%s", i+1, config.Method, config.Endpoint)
		if config.Action != "" {
			name = fmt.Sprintf("Step_%d_ACTION_%s", i+1, config.Action)
		}
		t.Run(name, func(t *testing.T) {
			// Handle special actions from config
			if config.Action == "DELETE_ALL_AAS_DESCRIPTORS" {
				// Fetch current descriptors
				body, err := makeRequest(TestConfig{Method: "GET", Endpoint: "http://127.0.0.1:6004/shell-descriptors", ExpectedStatus: 200})
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
					_, err := makeRequest(TestConfig{Method: "DELETE", Endpoint: fmt.Sprintf("http://127.0.0.1:6004/shell-descriptors/%s", enc), ExpectedStatus: 204})
					require.NoError(t, err)
				}
				return
			}

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

	// Only after config-driven cleanup and empty check, verify DB is empty
	// wait for code to be merged
	/*
	   t.Run("Check_DB_Empty", func(t *testing.T) {
	       db, err := sql.Open("postgres", "host=127.0.0.1 port=5432 user=admin password=admin123 dbname=basyxTestDB sslmode=disable")
	       require.NoError(t, err)
	       defer func() { _ = db.Close() }()
	       require.NoError(t, db.Ping())

	       rows, err := db.Query("SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	       require.NoError(t, err)
	       defer func() { _ = rows.Close() }()

	       nonEmpty := []string{}
	       for rows.Next() {
	           var table string
	           require.NoError(t, rows.Scan(&table))

	           var cnt int
	           q := fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", table)
	           err = db.QueryRow(q).Scan(&cnt)
	           require.NoError(t, err)
	           if cnt != 0 {
	               nonEmpty = append(nonEmpty, fmt.Sprintf("%s:%d", table, cnt))
	           }
	       }
	       require.NoError(t, rows.Err())

	       assert.Empty(t, nonEmpty, "Expected all tables empty, but found rows in: %v", nonEmpty)
	   })
	*/
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
