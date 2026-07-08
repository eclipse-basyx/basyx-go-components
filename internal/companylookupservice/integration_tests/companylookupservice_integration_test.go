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
// Author: Christian Koort ( Fraunhofer IESE )

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
	"github.com/stretchr/testify/require"
)

var companyLookupBaseURL = testenv.LocalURLFromEnv("BASYX_IT_API_PORT", 6004)
var companyLookupDSN = testenv.PostgresKeywordDSNFromEnv("BASYX_IT_DB_PORT", 6432, "basyxTestDB")

func deleteAllCompanyDescriptors(t *testing.T, runner *testenv.JSONSuiteRunner, stepNumber int) {
	response, err := runner.RunStep(testenv.JSONSuiteStep{
		Method:         http.MethodGet,
		Endpoint:       companyLookupBaseURL + "/companies",
		ExpectedStatus: http.StatusOK,
	}, stepNumber)
	require.NoError(t, err)

	var list struct {
		Result []struct {
			Domain string `json:"domain"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Body), &list))

	for _, item := range list.Result {
		enc := base64.RawURLEncoding.EncodeToString([]byte(item.Domain))
		_, err := runner.RunStep(testenv.JSONSuiteStep{
			Method:         http.MethodDelete,
			Endpoint:       fmt.Sprintf("%s/companies/%s", companyLookupBaseURL, enc),
			ExpectedStatus: http.StatusNoContent,
		}, stepNumber)
		require.NoError(t, err)
	}
}

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		EnableRequestLog: true,
		EnableRawDump:    true,
		StepName: func(step testenv.JSONSuiteStep, stepNumber int) string {
			context := "Not Provided"
			if step.Context != "" {
				context = step.Context
			}
			return fmt.Sprintf("Step_(%s)_%d_%s_%s", context, stepNumber, step.Method, step.Endpoint)
		},
		ActionHandlers: map[string]testenv.JSONStepAction{
			"DELETE_ALL_COMPANY_DESCRIPTORS": func(t *testing.T, runner *testenv.JSONSuiteRunner, _ testenv.JSONSuiteStep, stepNumber int) {
				deleteAllCompanyDescriptors(t, runner, stepNumber)
			},
			testenv.ActionCheckDBIsEmpty: testenv.NewCheckDBIsEmptyAction(testenv.CheckDBIsEmptyOptions{
				Driver: "pgx",
				DSN:    companyLookupDSN,
			}),
		},
	})
}

func TestMain(m *testing.M) {
	runtime := testenv.NewComposeRuntimeOrExit("company-lookup-it", []testenv.PortBinding{
		{Name: "api", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "db", EnvVar: "BASYX_IT_DB_PORT"},
	})
	companyLookupBaseURL = runtime.LocalURL("api")
	companyLookupDSN = runtime.PostgresKeywordDSN("db", "basyxTestDB")

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: "docker_compose/docker-compose.yml",
		ProjectName: runtime.ProjectName,
		Env:         runtime.Env(),
		HealthURL:   companyLookupBaseURL + "/health",
	}))
}
