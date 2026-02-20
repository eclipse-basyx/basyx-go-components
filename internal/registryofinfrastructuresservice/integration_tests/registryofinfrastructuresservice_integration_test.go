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

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func deleteAllInfrastructureDescriptors(t *testing.T, runner *testenv.JSONSuiteRunner, stepNumber int) {
	body, err := runner.RunStep(testenv.JSONSuiteStep{
		Method:         http.MethodGet,
		Endpoint:       "http://127.0.0.1:5004/infrastructure-descriptors",
		ExpectedStatus: http.StatusOK,
	}, stepNumber)
	require.NoError(t, err)

	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &list))

	for _, item := range list.Result {
		enc := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		_, err := runner.RunStep(testenv.JSONSuiteStep{
			Method:         http.MethodDelete,
			Endpoint:       fmt.Sprintf("http://127.0.0.1:5004/infrastructure-descriptors/%s", enc),
			ExpectedStatus: http.StatusNoContent,
		}, stepNumber)
		require.NoError(t, err)
	}
}

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet),
		EnableRequestLog:      true,
		EnableRawDump:         true,
		StepName: func(step testenv.JSONSuiteStep, stepNumber int) string {
			context := "Not Provided"
			if step.Context != "" {
				context = step.Context
			}
			return fmt.Sprintf("Step_(%s)_%d_%s_%s", context, stepNumber, step.Method, step.Endpoint)
		},
		ActionHandlers: map[string]testenv.JSONStepAction{
			"DELETE_ALL_INFRASTRUCTURE_DESCRIPTORS": func(t *testing.T, runner *testenv.JSONSuiteRunner, _ testenv.JSONSuiteStep, stepNumber int) {
				deleteAllInfrastructureDescriptors(t, runner, stepNumber)
			},
			testenv.ActionCheckDBIsEmpty: testenv.NewCheckDBIsEmptyAction(testenv.CheckDBIsEmptyOptions{
				Driver: "postgres",
				DSN:    "host=127.0.0.1 port=6432 user=admin password=admin123 dbname=basyxTestDB sslmode=disable",
			}),
		},
	})
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: "docker_compose/docker-compose.yml",
		HealthURL:   "http://localhost:5004/health",
	}))
}
