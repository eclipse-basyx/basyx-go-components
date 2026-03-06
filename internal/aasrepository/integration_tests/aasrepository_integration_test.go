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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq" // PostgreSQL Treiber
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
			encodedIdentifier := base64.RawStdEncoding.EncodeToString([]byte(item.ID))
			_, err = runner.RunStep(testenv.JSONSuiteStep{
				Method:         http.MethodDelete,
				Endpoint:       fmt.Sprintf("http://127.0.0.1:6004/shells/%s", encodedIdentifier),
				ExpectedStatus: http.StatusNoContent,
			}, stepNumber)
			require.NoError(t, err)
		}
	}
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

// TestMain handles setup and teardown
func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "docker_compose/docker_compose.yml",
		PreDownBeforeUp: true,
		HealthURL:       "http://localhost:6004/health",
		HealthTimeout:   150 * time.Second,
	}))
}
