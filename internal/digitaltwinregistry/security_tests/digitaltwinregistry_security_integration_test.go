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
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const (
	wenBaseURL              = "http://127.0.0.1:8082/api/v3"
	wenComposeFilePath      = "docker_compose/docker_compose.yml"
	deleteAllShellsActionID = "DELETE_ALL_SHELL_DESCRIPTORS"
)

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:        wenComposeFilePath,
		PreDownBeforeUp:    true,
		DownArgs:           []string{"down", "--remove-orphans"},
		HealthURL:          wenBaseURL + "/health",
		HealthTimeout:      3 * time.Minute,
		SkipDownAfterTests: false,
	}))
}

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		DefaultExpectedStatus: http.StatusOK,
		ActionHandlers: map[string]testenv.JSONStepAction{
			deleteAllShellsActionID: deleteAllDescriptors,
		},
		TokenProvider: testenv.NewPasswordGrantTokenProvider(
			"http://localhost:8080/realms/basyx/protocol/openid-connect/token",
			"basyx-ui",
			15*time.Second,
		),
	})
}

func deleteAllDescriptors(t *testing.T, runner *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, stepNumber int) {
	t.Helper()

	response, err := runner.RunStep(testenv.JSONSuiteStep{
		Method:         http.MethodGet,
		Endpoint:       wenBaseURL + "/shell-descriptors?limit=500",
		ExpectedStatus: http.StatusOK,
		Headers:        step.Headers,
		Token:          step.Token,
	}, stepNumber)
	require.NoError(t, err)

	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Body), &list))

	for _, item := range list.Result {
		enc := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		_, err = runner.RunStep(testenv.JSONSuiteStep{
			Method:         http.MethodDelete,
			Endpoint:       wenBaseURL + "/shell-descriptors/" + enc,
			ExpectedStatus: http.StatusNoContent,
			Headers:        step.Headers,
			Token:          step.Token,
		}, stepNumber)
		require.NoError(t, err)
	}
}
