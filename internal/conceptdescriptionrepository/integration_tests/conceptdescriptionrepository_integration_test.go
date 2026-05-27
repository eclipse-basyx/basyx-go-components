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

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

func requestJSON(method string, endpoint string, payload any) (int, []byte, error) {
	status, body, _, err := requestJSONWithHeaders(method, endpoint, payload)
	return status, body, err
}

func requestJSONWithHeaders(method string, endpoint string, payload any) (int, []byte, http.Header, error) {
	var body io.Reader
	if payload != nil {
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("failed to marshal payload: %v", err)
		}
		body = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to create request: %v", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	// #nosec G107 -- integration tests intentionally issue HTTP requests against local test service endpoints.
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header.Clone(), fmt.Errorf("failed to read response body: %v", err)
	}

	return resp.StatusCode, respBody, resp.Header.Clone(), nil
}

func cleanupConceptDescription(t *testing.T, endpoint string, conceptDescriptionID string) {
	t.Helper()

	encodedIdentifier := base64.RawURLEncoding.EncodeToString([]byte(conceptDescriptionID))
	statusCode, responseBody, err := requestJSON(http.MethodDelete, endpoint+"/"+encodedIdentifier, nil)
	if err != nil {
		t.Fatalf("failed to cleanup concept description: %v", err)
	}
	if statusCode != http.StatusNoContent && statusCode != http.StatusNotFound {
		t.Fatalf("unexpected cleanup status %d with body: %s", statusCode, string(responseBody))
	}
}

func assertLocationHeaderMatches(t *testing.T, expectedLocation string, actualLocation string) {
	t.Helper()

	if actualLocation == "" {
		t.Fatalf("expected non-empty Location header")
	}

	expectedURL, err := url.Parse(expectedLocation)
	if err != nil {
		t.Fatalf("failed to parse expected Location %q: %v", expectedLocation, err)
	}

	actualURL, err := url.Parse(actualLocation)
	if err != nil {
		t.Fatalf("failed to parse actual Location %q: %v", actualLocation, err)
	}

	if expectedURL.Scheme != actualURL.Scheme || expectedURL.Path != actualURL.Path || expectedURL.RawQuery != actualURL.RawQuery || expectedURL.Port() != actualURL.Port() {
		t.Fatalf("expected Location %s, got %s", expectedLocation, actualLocation)
	}

	expectedHost := strings.ToLower(expectedURL.Hostname())
	actualHost := strings.ToLower(actualURL.Hostname())
	if expectedHost == actualHost {
		return
	}

	allowedLoopbackHosts := map[string]struct{}{"localhost": {}, "127.0.0.1": {}}
	if _, ok := allowedLoopbackHosts[expectedHost]; !ok {
		t.Fatalf("expected host %s is not loopback-compatible", expectedHost)
	}
	if _, ok := allowedLoopbackHosts[actualHost]; !ok {
		t.Fatalf("actual host %s is not loopback-compatible", actualHost)
	}
}

func TestLocationHeadersForCreateEndpointsConceptDescriptionRepository(t *testing.T) {
	baseURL := "http://127.0.0.1:6004"
	endpoint := baseURL + "/concept-descriptions"

	t.Run("PostConceptDescriptionSetsLocation", func(t *testing.T) {
		conceptDescriptionID := fmt.Sprintf("https://example.com/ids/cd/location-post-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupConceptDescription(t, endpoint, conceptDescriptionID) })

		statusCode, responseBody, headers, err := requestJSONWithHeaders(http.MethodPost, endpoint, map[string]any{
			"id":        conceptDescriptionID,
			"modelType": "ConceptDescription",
		})
		if err != nil {
			t.Fatalf("failed to create concept description: %v", err)
		}
		if statusCode != http.StatusCreated {
			t.Fatalf("expected 201 on create, got %d with body: %s", statusCode, string(responseBody))
		}

		expectedIdentifier := base64.RawURLEncoding.EncodeToString([]byte(conceptDescriptionID))
		expectedLocation := endpoint + "/" + expectedIdentifier
		assertLocationHeaderMatches(t, expectedLocation, headers.Get("Location"))
	})

	t.Run("PutConceptDescriptionByIdSetsLocationOnlyOnCreate", func(t *testing.T) {
		conceptDescriptionID := fmt.Sprintf("https://example.com/ids/cd/location-put-%d", time.Now().UnixNano())
		t.Cleanup(func() { cleanupConceptDescription(t, endpoint, conceptDescriptionID) })

		encodedIdentifier := base64.RawURLEncoding.EncodeToString([]byte(conceptDescriptionID))
		putEndpoint := endpoint + "/" + encodedIdentifier

		createStatusCode, createResponseBody, createHeaders, createErr := requestJSONWithHeaders(http.MethodPut, putEndpoint, map[string]any{
			"id":        conceptDescriptionID,
			"modelType": "ConceptDescription",
		})
		if createErr != nil {
			t.Fatalf("failed to create concept description through put: %v", createErr)
		}
		if createStatusCode != http.StatusCreated {
			t.Fatalf("expected 201 on put-create, got %d with body: %s", createStatusCode, string(createResponseBody))
		}
		assertLocationHeaderMatches(t, putEndpoint, createHeaders.Get("Location"))

		updateStatusCode, updateResponseBody, updateHeaders, updateErr := requestJSONWithHeaders(http.MethodPut, putEndpoint, map[string]any{
			"id":        conceptDescriptionID,
			"modelType": "ConceptDescription",
		})
		if updateErr != nil {
			t.Fatalf("failed to update concept description through put: %v", updateErr)
		}
		if updateStatusCode != http.StatusNoContent {
			t.Fatalf("expected 204 on put-update, got %d with body: %s", updateStatusCode, string(updateResponseBody))
		}
		if updateHeaders.Get("Location") != "" {
			t.Fatalf("expected empty Location on update, got %s", updateHeaders.Get("Location"))
		}
	})
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
