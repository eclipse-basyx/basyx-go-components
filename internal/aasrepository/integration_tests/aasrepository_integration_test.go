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
	"encoding/base64"
	"encoding/json"
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

const actionDeleteAllAAS = "DELETE_ALL_AAS"

func deleteAllAAS(t *testing.T, runner *testenv.JSONSuiteRunner, stepNumber int) {
	for {
		response, err := runner.RunStep(testenv.JSONSuiteStep{
			Method:         http.MethodGet,
			Endpoint:       "http://127.0.0.1:6004/shells",
			ExpectedStatus: http.StatusOK,
		}, stepNumber)
		require.NoError(t, err)

		var list struct {
			Result []struct {
				ID string `json:"id"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal([]byte(response.Body), &list))

		if len(list.Result) == 0 {
			return
		}

		for _, item := range list.Result {
			encodedIdentifier := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
			_, err = runner.RunStep(testenv.JSONSuiteStep{
				Method:         http.MethodDelete,
				Endpoint:       fmt.Sprintf("http://127.0.0.1:6004/shells/%s", encodedIdentifier),
				ExpectedStatus: http.StatusNoContent,
			}, stepNumber)
			require.NoError(t, err)
		}
	}
}

func createAASForThumbnailTest(baseURL string, aasID string) (int, error) {
	body := fmt.Sprintf(`{"id":"%s","modelType":"AssetAdministrationShell","assetInformation":{"assetKind":"Instance"}}`, aasID)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/shells", strings.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

func uploadThumbnail(endpoint string, filePath string, fileName string) (int, error) {
	file, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return 0, fmt.Errorf("failed to create form file: %v", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return 0, fmt.Errorf("failed to copy file: %v", err)
	}

	if fileName != "" {
		if err = writer.WriteField("fileName", fileName); err != nil {
			return 0, fmt.Errorf("failed to write fileName field: %v", err)
		}
	}

	err = writer.Close()
	if err != nil {
		return 0, fmt.Errorf("failed to close writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPut, endpoint, body)
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

func downloadThumbnail(endpoint string) ([]byte, string, int, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", resp.StatusCode, fmt.Errorf("failed to read response: %v", err)
	}

	return body, resp.Header.Get("Content-Type"), resp.StatusCode, nil
}

// IntegrationTest runs the integration tests based on the config file
func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet, http.MethodPost),
		ActionHandlers: map[string]testenv.JSONStepAction{
			actionDeleteAllAAS: func(t *testing.T, runner *testenv.JSONSuiteRunner, _ testenv.JSONSuiteStep, stepNumber int) {
				deleteAllAAS(t, runner, stepNumber)
			},
			testenv.ActionAssertSubmodelAbsent: testenv.NewCheckSubmodelAbsentAction(testenv.CheckSubmodelAbsentOptions{
				Driver: "postgres",
				DSN:    "postgres://admin:admin123@127.0.0.1:6432/basyxTestDB?sslmode=disable",
			}),
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

func TestThumbnailAttachmentOperations(t *testing.T) {
	baseURL := "http://localhost:6004"
	aasID := fmt.Sprintf("https://example.com/ids/aas/thumbnail_test_%d", time.Now().UnixNano())
	aasIdentifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	thumbnailEndpoint := fmt.Sprintf("%s/shells/%s/asset-information/thumbnail", baseURL, aasIdentifier)
	testFilePath := "testFiles/marcus.gif"

	statusCode, err := createAASForThumbnailTest(baseURL, aasID)
	require.NoError(t, err, "AAS creation failed")
	assert.Equal(t, http.StatusCreated, statusCode, "Expected 201 Created for AAS creation")

	originalContent, err := os.ReadFile(testFilePath)
	require.NoError(t, err, "Failed to read thumbnail test file")

	t.Run("1_Upload_Thumbnail", func(t *testing.T) {
		uploadStatus, uploadErr := uploadThumbnail(thumbnailEndpoint, testFilePath, "marcus.gif")
		require.NoError(t, uploadErr, "Thumbnail upload failed")
		assert.Equal(t, http.StatusNoContent, uploadStatus, "Expected 204 No Content for thumbnail upload")
	})

	t.Run("2_Download_Thumbnail_And_Verify", func(t *testing.T) {
		time.Sleep(2 * time.Second)
		content, contentType, getStatus, getErr := downloadThumbnail(thumbnailEndpoint)
		require.NoError(t, getErr, "Thumbnail download failed")
		assert.Equal(t, http.StatusOK, getStatus, "Expected 200 OK for thumbnail download")
		assert.NotEmpty(t, contentType, "Content-Type should be set")
		t.Logf("Downloaded thumbnail Content-Type: %s", contentType)
		assert.Equal(t, originalContent, content, "Downloaded thumbnail content should match uploaded content")
		t.Logf("Thumbnail content verified: %d bytes", len(content))
	})

	t.Run("3_Delete_Thumbnail", func(t *testing.T) {
		req, reqErr := http.NewRequest(http.MethodDelete, thumbnailEndpoint, nil)
		require.NoError(t, reqErr, "Failed to create DELETE request")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, doErr := client.Do(req)
		require.NoError(t, doErr, "DELETE request failed")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Expected 204 No Content for thumbnail deletion")
	})

	t.Run("4_Verify_Thumbnail_Deleted", func(t *testing.T) {
		_, _, getStatus, getErr := downloadThumbnail(thumbnailEndpoint)
		require.NoError(t, getErr, "Thumbnail download after delete should not fail at HTTP level")
		assert.Equal(t, http.StatusNotFound, getStatus, "Expected 404 Not Found after thumbnail deletion")
	})

	t.Run("5_Upload_Thumbnail_For_NonExisting_AAS", func(t *testing.T) {
		nonExistingID := base64.RawStdEncoding.EncodeToString([]byte("https://example.com/ids/aas/non_existing_thumbnail_test"))
		nonExistingEndpoint := fmt.Sprintf("%s/shells/%s/asset-information/thumbnail", baseURL, nonExistingID)

		uploadStatus, uploadErr := uploadThumbnail(nonExistingEndpoint, testFilePath, "marcus.gif")
		require.NoError(t, uploadErr, "Upload request for non-existing AAS should complete")
		assert.Equal(t, http.StatusNotFound, uploadStatus, "Expected 404 Not Found for non-existing AAS")
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
