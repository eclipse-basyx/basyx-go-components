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
// Author: Martin Stemmer ( Fraunhofer IESE )

//nolint:all
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const (
	actionPostRuleExpectContextLocation = "POST_RULE_EXPECT_CONTEXT_LOCATION"
	rulesSuiteBaseURL                   = "http://localhost:6014/api/v3"
	rulesSuiteTokenURL                  = "http://localhost:18080/realms/basyx/protocol/openid-connect/token"
	expectedCreatedRuleLocation         = "/api/v3/rules/7"
)

func TestIntegration(t *testing.T) {
	tokenProvider := testenv.NewPasswordGrantTokenProvider(rulesSuiteTokenURL, "basyx-ui", 10*time.Second)

	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		DefaultExpectedStatus: http.StatusOK,
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet, http.MethodPost, http.MethodPut),
		TokenProvider:         tokenProvider,
		ActionHandlers: map[string]testenv.JSONStepAction{
			actionPostRuleExpectContextLocation: func(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
				postRuleExpectContextLocation(t, step, tokenProvider)
			},
		},
	})
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: "docker_compose/docker_compose.yml",
		HealthURL:   rulesSuiteBaseURL + "/health",
	}))
}

func postRuleExpectContextLocation(t *testing.T, step testenv.JSONSuiteStep, tokenProvider testenv.JSONTokenProvider) {
	t.Helper()

	require.NotEmpty(t, step.Endpoint)
	require.NotEmpty(t, step.Data)
	require.NotNil(t, step.Token)

	bodyBytes, err := os.ReadFile(step.Data)
	require.NoError(t, err)

	method := step.Method
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequest(method, step.Endpoint, bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range step.Headers {
		req.Header.Set(key, value)
	}

	token, err := tokenProvider.GetAccessToken(step.Token)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := testenv.HTTPClient().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	expectedStatus := step.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusCreated
	}
	require.Equal(t, expectedStatus, resp.StatusCode)

	require.Equal(t, expectedCreatedRuleLocation, resp.Header.Get("Location"))

	var created struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(respBody, &created))
	require.EqualValues(t, 7, created.ID)

	if step.ShouldMatch != "" {
		expectedBody, readErr := os.ReadFile(step.ShouldMatch)
		require.NoError(t, readErr)
		require.JSONEq(t, string(expectedBody), string(respBody))
	}
}
