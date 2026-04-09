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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq" // PostgreSQL Treiber
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

func requestJSON(method string, endpoint string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to marshal payload: %v", err)
		}
		body = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %v", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return resp.StatusCode, respBody, nil
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

func TestContractGetAllConceptDescriptionsAllowsNullableIDShort(t *testing.T) {
	baseURL := "http://localhost:6004"
	conceptDescriptionID := fmt.Sprintf("https://example.com/ids/cd/contract-null-idshort-%d", time.Now().UnixNano())

	statusCode, responseBody, err := requestJSON(http.MethodPost, baseURL+"/concept-descriptions", map[string]any{
		"id":        conceptDescriptionID,
		"modelType": "ConceptDescription",
	})
	if err != nil {
		t.Fatalf("failed to create concept description: %v", err)
	}
	if statusCode != http.StatusCreated {
		t.Fatalf("expected 201 on create, got %d with body: %s", statusCode, string(responseBody))
	}

	statusCode, responseBody, err = requestJSON(http.MethodGet, baseURL+"/concept-descriptions", nil)
	if err != nil {
		t.Fatalf("failed to list concept descriptions: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d with body: %s", statusCode, string(responseBody))
	}

	var listResponse map[string]any
	if err = json.Unmarshal(responseBody, &listResponse); err != nil {
		t.Fatalf("failed to parse list response: %v", err)
	}

	resultRaw, ok := listResponse["result"].([]any)
	if !ok {
		t.Fatalf("expected result array in list response, got: %T", listResponse["result"])
	}

	foundCreatedConceptDescription := false
	for _, entry := range resultRaw {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		identifier, ok := item["id"].(string)
		if ok && identifier == conceptDescriptionID {
			foundCreatedConceptDescription = true
			break
		}
	}

	if !foundCreatedConceptDescription {
		t.Fatalf("expected concept description %s in list response, got body: %s", conceptDescriptionID, string(responseBody))
	}
}

// TestMain handles setup and teardown
func TestMain(m *testing.M) {
	if os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1" {
		os.Exit(m.Run())
	}

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "docker_compose/docker_compose.yml",
		UpArgs:          []string{"up", "-d", "--build", "--remove-orphans"},
		PreDownBeforeUp: true,
		HealthURL:       "http://localhost:6004/health",
		HealthTimeout:   150 * time.Second,
	}))
}
