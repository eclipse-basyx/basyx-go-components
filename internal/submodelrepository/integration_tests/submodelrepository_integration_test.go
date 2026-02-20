//nolint:all
package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet, http.MethodPost),
		ShouldSkipStep: func(step testenv.JSONSuiteStep) bool {
			if strings.EqualFold(step.Method, http.MethodPut) && strings.Contains(step.Endpoint, "/attachment") {
				return true
			}
			if strings.EqualFold(step.Method, http.MethodGet) && strings.Contains(step.Endpoint, "/attachment") {
				return true
			}
			return false
		},
		StepName: func(step testenv.JSONSuiteStep, stepNumber int) string {
			context := "Not Provided"
			if step.Context != "" {
				context = step.Context
			}
			return fmt.Sprintf("Step_(%s)_%d_%s_%s", context, stepNumber, step.Method, step.Endpoint)
		},
	})
}

// TestFileAttachmentOperations tests file upload, download, and deletion for File SME
func TestFileAttachmentOperations(t *testing.T) {
	baseURL := "http://localhost:6004"
	submodelID := "aHR0cDovL2llc2UuZnJhdW5ob2Zlci5kZS9pZC9zbS9Pbmx5RmlsZVN1Ym1vZGVsX1Rlc3Q" // base64 encoded: http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel_Test
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
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "docker_compose/docker_compose.yml",
		PreDownBeforeUp: true,
		HealthURL:       "http://localhost:6004/health",
		HealthTimeout:   150 * time.Second,
	}))
}
