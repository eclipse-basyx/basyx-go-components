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

package testenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultCheckDBIsEmptyExcludedTablesIncludesPersistentSchemaTables(t *testing.T) {
	excluded := defaultCheckDBIsEmptyExcludedTables([]string{"AAS_Identifier", " "})

	for _, table := range []string{
		"basyxsystem",
		"history_guard_config",
		"aas_history",
		"aas_history_payload",
		"submodel_history",
		"submodel_history_payload",
		"concept_description_history",
		"concept_description_history_payload",
		"descriptor_history",
		"descriptor_history_payload",
		"submodel_descriptor_history",
		"submodel_descriptor_history_payload",
		"aas_identifier",
	} {
		_, ok := excluded[table]
		require.Truef(t, ok, "expected %s to be excluded", table)
	}
}

func TestApplyJSONSuiteTemplateValuesPreservesDollarPathTokens(t *testing.T) {
	input := []byte(`[{"method":"GET","endpoint":"{{BASYX_IT_BASE_URL}}/submodels/$metadata/$path"}]`)

	output := applyJSONSuiteTemplateValues(input, map[string]string{
		"BASYX_IT_BASE_URL": "http://127.0.0.1:61234",
	})

	require.JSONEq(t,
		`[{"method":"GET","endpoint":"http://127.0.0.1:61234/submodels/$metadata/$path"}]`,
		string(output),
	)
}

func TestJSONSuiteRunnerTemplatesBodyAndExpectedFiles(t *testing.T) {
	tempDir := t.TempDir()
	bodyPath := filepath.Join(tempDir, "body.json")
	expectedPath := filepath.Join(tempDir, "expected.json")
	require.NoError(t, os.WriteFile(bodyPath, []byte(`{"href":"{{BASYX_IT_BASE_URL}}/submodels/$reference"}`), 0o600))
	require.NoError(t, os.WriteFile(expectedPath, []byte(`{"href":"{{BASYX_IT_BASE_URL}}/submodels/$metadata"}`), 0o600))

	runner := &JSONSuiteRunner{
		options: JSONSuiteOptions{
			TemplateValues: map[string]string{
				"BASYX_IT_BASE_URL": "http://127.0.0.1:61234",
			},
		},
	}

	body, err := runner.loadStepBody(JSONSuiteStep{Data: bodyPath})
	require.NoError(t, err)
	require.JSONEq(t, `{"href":"http://127.0.0.1:61234/submodels/$reference"}`, string(body))

	runner.compareJSONResponse(t, JSONSuiteStep{ShouldMatch: expectedPath}, 1, `{"href":"http://127.0.0.1:61234/submodels/$metadata"}`)
}

func TestPrepareSecurityEnvCopiesAndRewritesIssuer(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(sourceDir, "trustlist.json"),
		[]byte(`[{"issuer":"http://localhost:8080/realms/basyx"}]`),
		0o600,
	))

	targetDir, err := PrepareSecurityEnv(sourceDir, map[string]string{
		"http://localhost:8080": "http://localhost:18080",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(targetDir) })

	//nolint:gosec // The test reads a fixture path created under t.TempDir.
	rewritten, err := os.ReadFile(filepath.Join(targetDir, "trustlist.json"))
	require.NoError(t, err)
	require.JSONEq(t, `[{"issuer":"http://localhost:18080/realms/basyx"}]`, string(rewritten))
}
