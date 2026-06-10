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
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const actionAssertRecentChangeIDs = "ASSERT_RECENT_CHANGE_IDS"

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		DefaultExpectedStatus: http.StatusOK,
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet, http.MethodPost),
		TokenProvider: testenv.NewPasswordGrantTokenProvider(
			"http://localhost:8080/realms/basyx/protocol/openid-connect/token",
			"basyx-ui",
			10*time.Second,
		),
		ActionHandlers: map[string]testenv.JSONStepAction{
			actionAssertRecentChangeIDs: assertRecentChangeIDsAction,
		},
	})
}

func assertRecentChangeIDsAction(t *testing.T, runner *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, stepNumber int) {
	t.Helper()

	response, err := runner.RunStep(step, stepNumber)
	require.NoError(t, err)

	expectedIDs := loadExpectedRecentChangeIDs(t, step.ShouldMatch)
	actualIDs := extractRecentChangeIDs(t, response.Body)

	require.ElementsMatch(t, expectedIDs, actualIDs)
}

func loadExpectedRecentChangeIDs(t *testing.T, path string) []string {
	t.Helper()

	require.NotEmpty(t, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var expectedIDs []string
	require.NoError(t, json.Unmarshal(data, &expectedIDs))
	return expectedIDs
}

func extractRecentChangeIDs(t *testing.T, responseBody string) []string {
	t.Helper()

	var payload struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(responseBody), &payload))

	ids := make([]string, 0, len(payload.Result))
	for _, change := range payload.Result {
		ids = append(ids, change.ID)
	}
	return ids
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "docker_compose/docker_compose.yml",
		PreDownBeforeUp: true,
		HealthURL:       "http://localhost:6004/health",
		HealthTimeout:   180 * time.Second,
	}))
}
