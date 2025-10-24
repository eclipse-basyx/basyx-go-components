package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------
// Config models
// -----------------------------

type TokenCredentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// TestConfig represents the structure of your test configuration
type TestConfig struct {
	Context        string            `json:"context,omitempty"`
	Method         string            `json:"method"`
	Endpoint       string            `json:"endpoint"`
	Data           string            `json:"data,omitempty"`
	ShouldMatch    string            `json:"shouldMatch,omitempty"`
	ExpectedStatus int               `json:"expectedStatus,omitempty"`
	Token          *TokenCredentials `json:"token,omitempty"`
}

// -----------------------------
// Helpers: config loading
// -----------------------------

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

// -----------------------------
// Helpers: token retrieval & cache
// -----------------------------

var (
	tokenCache   = map[string]string{} // key: "user|password"
	tokenCacheMu sync.Mutex
)

// getAccessToken retrieves an access token from Keycloak using ROPC flow.
// It caches tokens per (user,password) for the duration of the test run.
func getAccessToken(creds *TokenCredentials) (string, error) {
	if creds == nil {
		return "", nil
	}

	key := creds.User + "|" + creds.Password
	tokenCacheMu.Lock()
	if cached, ok := tokenCache[key]; ok && cached != "" {
		tokenCacheMu.Unlock()
		return cached, nil
	}
	tokenCacheMu.Unlock()

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", "basyx-ui")
	form.Set("username", creds.User)
	form.Set("password", creds.Password)

	tokenURL := "http://localhost:8080/realms/basyx/protocol/openid-connect/token"

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request token: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get token, status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %v", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access_token in response")
	}

	tokenCacheMu.Lock()
	tokenCache[key] = tokenResp.AccessToken
	tokenCacheMu.Unlock()

	return tokenResp.AccessToken, nil
}

// -----------------------------
// HTTP request executor
// -----------------------------

// makeRequest performs an HTTP request based on the test config
func makeRequest(config TestConfig) (string, error) {
	var req *http.Request
	var err error

	// If ExpectedStatus not set, default to 200
	expectedStatus := config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}

	// Handle token if present
	var accessToken string
	if config.Token != nil {
		accessToken, err = getAccessToken(config.Token)
		if err != nil {
			return "", fmt.Errorf("failed to get access token: %v", err)
		}
	}

	switch strings.ToUpper(config.Method) {
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

	// Attach bearer token if available
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		fmt.Printf("Response status code: %d\n", resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Response body: %s\n", body)
		return "", fmt.Errorf("expected status %d but got %d", expectedStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// -----------------------------
// Tests
// -----------------------------

// IntegrationTest runs the integration tests based on the config file
func TestIntegration(t *testing.T) {
	// Load test configuration
	configs, err := loadTestConfig("it_config.json")
	require.NoError(t, err, "Failed to load test config")

	// Wait for services to be ready (adjust as needed)
	time.Sleep(15 * time.Second) // Wait for Docker Compose services

	for i, config := range configs {
		name := fmt.Sprintf("Step_%d_%s_%s", i+1, strings.ToUpper(config.Method), config.Endpoint)
		if config.Context != "" {
			name = fmt.Sprintf("Step_%d_%s", i+1, config.Context)
		}

		t.Run(name, func(t *testing.T) {
			response, err := makeRequest(config)
			require.NoError(t, err, "Request failed")

			if strings.ToUpper(config.Method) == "GET" && config.ShouldMatch != "" {
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
