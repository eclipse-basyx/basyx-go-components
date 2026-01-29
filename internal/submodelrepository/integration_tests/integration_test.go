//nolint:all
package main

import (
	"bytes"
	"database/sql"
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

// TestFileAttachmentOperations tests file upload, download, and deletion for File SME
func TestFileAttachmentOperations(t *testing.T) {
	baseURL := "http://localhost:6004"
	submodelID := "aHR0cDovL2llc2UuZnJhdW5ob2Zlci5kZS9pZC9zbS9Pbmx5RmlsZVN1Ym1vZGVs" // base64 encoded: http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel
	testFilePath := "testFiles/marcus.gif"

	// Read the test file content for later comparison
	originalFileContent, err := os.ReadFile(testFilePath)
	require.NoError(t, err, "Failed to read test file")

	t.Run("1_Upload_File_Attachment", func(t *testing.T) {
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		statusCode, err := uploadFileAttachment(endpoint, testFilePath, "marcus.gif")
		require.NoError(t, err, "File upload failed")
		assert.Equal(t, http.StatusNoContent, statusCode, "Expected 204 No Content for file upload")
	})

	t.Run("2_Download_File_Attachment_And_Verify", func(t *testing.T) {
		// Wait a moment to ensure the file is available
		time.Sleep(2 * time.Second)
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		content, contentType, statusCode, err := downloadFileAttachment(endpoint)
		require.NoError(t, err, "File download failed")
		assert.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for file download")

		// Verify Content-Type is set (should be auto-detected as image/png)
		assert.NotEmpty(t, contentType, "Content-Type should be set")
		t.Logf("Downloaded file Content-Type: %s", contentType)

		// Verify file content matches uploaded file byte-by-byte
		assert.Equal(t, originalFileContent, content, "Downloaded file content should match uploaded file")
		t.Logf("File content verified: %d bytes", len(content))
	})

	t.Run("3_Update_File_Element_Value_Should_Delete_LargeObject", func(t *testing.T) {
		// Update the File SME value to an external URL (should trigger LO cleanup)
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile", baseURL, submodelID)
		updateData, err := os.ReadFile("bodies/updateFileElement.json")
		require.NoError(t, err, "Failed to read update data")

		req, err := http.NewRequest("PUT", endpoint, bytes.NewBuffer(updateData))
		require.NoError(t, err, "Failed to create PUT request")
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err, "PUT request failed")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Expected 204 No Content for File SME update")
	})

	t.Run("4_Verify_File_Attachment_Removed_After_Value_Update", func(t *testing.T) {
		// Try to download - should fail since value is now an external URL
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		_, _, statusCode, _ := downloadFileAttachment(endpoint)

		// Should return 404 or redirect to external URL (302)
		// Since value is now http://example.com/updated-file.png, it should redirect
		assert.Contains(t, []int{http.StatusFound, http.StatusNotFound}, statusCode,
			"Should redirect to external URL or return 404 after value update")
	})

	t.Run("5_Reupload_File_Attachment", func(t *testing.T) {
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		statusCode, err := uploadFileAttachment(endpoint, testFilePath, "test-image-reupload.png")
		require.NoError(t, err, "File reupload failed")
		assert.Equal(t, http.StatusNoContent, statusCode, "Expected 204 No Content for file reupload")
	})

	t.Run("6_Verify_Reuploaded_File", func(t *testing.T) {
		// Wait a moment to ensure the file is available
		time.Sleep(2 * time.Second)
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		content, contentType, statusCode, err := downloadFileAttachment(endpoint)
		require.NoError(t, err, "File download failed")
		assert.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for file download")
		assert.NotEmpty(t, contentType, "Content-Type should be set")
		assert.Equal(t, originalFileContent, content, "Reuploaded file content should match original")
	})

	t.Run("7_Delete_File_Attachment", func(t *testing.T) {
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		req, err := http.NewRequest("DELETE", endpoint, nil)
		require.NoError(t, err, "Failed to create DELETE request")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err, "DELETE request failed")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK for file deletion")
	})

	t.Run("8_Verify_File_Deleted", func(t *testing.T) {
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		_, _, statusCode, _ := downloadFileAttachment(endpoint)
		assert.Equal(t, http.StatusNotFound, statusCode, "Expected 404 Not Found after file deletion")
	})
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
	cmd = exec.Command("docker", "compose", "-f", "docker_compose/docker_compose.yml", "up", "-d", "--build")
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

	// Create DB Connection here
	sqlQuery, err := sql.Open("postgres", "postgres://admin:admin123@127.0.0.1:6432/basyxTestDB?sslmode=disable")

	if err != nil {
		_, _ = fmt.Printf("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	// wait for 5sec to ensure that the DB is ready
	time.Sleep(5 * time.Second)

	dir, osErr := os.Getwd()

	if osErr != nil {
		_, _ = fmt.Printf("Failed to get working directory: %v\n", osErr)
		os.Exit(1)
	}

	queryString, fileError := os.ReadFile(dir + "/sql/demoSubmodel.sql")

	if fileError != nil {
		_, _ = fmt.Printf("Failed to read SQL file: %v\n", fileError)
		os.Exit(1)
	}

	_, err = sqlQuery.Exec(string(queryString))

	if err != nil {
		_, _ = fmt.Printf("Failed to execute SQL script: %v\n", err)
		os.Exit(1)
	}

	_, _ = fmt.Println("Database initialized successfully.")

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
