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
package bench

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const composeFilePath = "./docker_compose/docker_compose.yml"
const discoveryBaseURL = "http://127.0.0.1:5004"
const actionShellsByAssetLinkMissingBody = "CHECK_SHELLSBYASSETLINK_MISSING_BODY"
const actionLookupShellsNilBody = "CHECK_LOOKUPSHELLS_NIL_BODY"

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:   composeFilePath,
		HealthURL:     discoveryBaseURL + "/health",
		HealthTimeout: 2 * time.Minute,
	}))
}

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ConfigPath: "it_config.json",
		ShouldCompareResponse: testenv.CompareMethods(
			http.MethodGet,
			http.MethodPost,
		),
		ActionHandlers: map[string]testenv.JSONStepAction{
			actionShellsByAssetLinkMissingBody: checkPostNilBodyExpectedStatus,
			actionLookupShellsNilBody:          checkPostNilBodyExpectedStatus,
		},
	})
}

func checkPostNilBodyExpectedStatus(t *testing.T, _ *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, _ int) {
	t.Helper()

	expectedStatus := step.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusBadRequest
	}

	req, err := http.NewRequest(http.MethodPost, step.Endpoint, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testenv.HTTPClient().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatus, resp.StatusCode)
}
