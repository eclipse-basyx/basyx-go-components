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
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const (
	preconfigurationComposeFilePath = "./docker_compose/docker_compose.yml"
	preconfigurationBaseURL         = "http://127.0.0.1:6014"
	observerStatusFilePath          = "./docker_compose/observer/health_observer_status.txt"
)

func TestMain(m *testing.M) {
	_ = os.MkdirAll("./docker_compose/observer", 0o750)
	_ = os.Remove(observerStatusFilePath)

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     preconfigurationComposeFilePath,
		PreDownBeforeUp: true,
	}))
}

func TestStartupPreconfiguration_HealthGateAndDataAvailability(t *testing.T) {
	healthURL := preconfigurationBaseURL + "/health"
	waitForHealthToBecomeUp(t, healthURL, 3*time.Minute)
	assertHealthObserverTransition(t, 2*time.Minute)

	shellCount := getCollectionCount(t, preconfigurationBaseURL+"/shells")
	submodelCount := getCollectionCount(t, preconfigurationBaseURL+"/submodels")
	conceptDescriptionCount := getCollectionCount(t, preconfigurationBaseURL+"/concept-descriptions")

	require.Greater(t, shellCount, 0, "expected at least one shell from startup preconfiguration")
	require.Greater(t, submodelCount, 0, "expected at least one submodel from startup preconfiguration")
	require.Greater(t, conceptDescriptionCount, 0, "expected at least one concept description from startup preconfiguration")
}

func waitForHealthToBecomeUp(t *testing.T, healthURL string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := testenv.HTTPClient().Get(healthURL)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, readErr)

		statusValue := parseHealthStatusValue(body)
		switch resp.StatusCode {
		case http.StatusServiceUnavailable:
			require.Equal(t, "DOWN", statusValue)
			detailValue := parseHealthDetailValue(body)
			require.Equal(t, "AAS preconfiguration in progress", detailValue)
		case http.StatusOK:
			require.Equal(t, "UP", statusValue)
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("service did not become healthy within %s", timeout)
}

func assertHealthObserverTransition(t *testing.T, timeout time.Duration) {
	t.Helper()

	const markerUnavailable = "HEALTH_OBSERVER_SAW_503"
	const markerUp = "HEALTH_OBSERVER_SAW_200"

	deadline := time.Now().Add(timeout)
	lastStatus := ""

	for time.Now().Before(deadline) {
		status, err := os.ReadFile(observerStatusFilePath)
		if err == nil {
			statusContent := string(status)
			lastStatus = statusContent
			if strings.Contains(statusContent, markerUnavailable) && strings.Contains(statusContent, markerUp) {
				return
			}
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf(
		"health observer did not capture 503->200 transition within %s. observer=%s statusFile=%s",
		timeout,
		lastStatus,
		observerStatusFilePath,
	)
}

func parseHealthStatusValue(responseBody []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return ""
	}
	statusValue, _ := payload["status"].(string)
	return statusValue
}

func parseHealthDetailValue(responseBody []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return ""
	}
	detailValue, _ := payload["details"].(string)
	return detailValue
}

func getCollectionCount(t *testing.T, endpoint string) int {
	t.Helper()

	resp, err := testenv.HTTPClient().Get(endpoint)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	require.Equalf(t, http.StatusOK, resp.StatusCode, "request failed for %s", endpoint)

	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var objectPayload map[string]any
	if err = json.Unmarshal(responseBody, &objectPayload); err == nil {
		if resultValues, ok := objectPayload["result"].([]any); ok {
			return len(resultValues)
		}
	}

	var listPayload []any
	if err = json.Unmarshal(responseBody, &listPayload); err == nil {
		return len(listPayload)
	}

	t.Fatalf("unable to parse collection response from %s: %s", endpoint, string(responseBody))
	return 0
}
