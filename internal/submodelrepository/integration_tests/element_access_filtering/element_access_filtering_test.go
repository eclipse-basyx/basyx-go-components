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

package elementaccessfiltering

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const accessSubmodelID = "urn:uuid:design-example-001"

var (
	accessServices []accessService
	accessTokenURL string
)

type accessService struct {
	name    string
	baseURL string
}

func TestReferableCollectionFiltering(t *testing.T) {
	provider := accessTokenProvider(t)
	for _, service := range accessServices {
		t.Run(service.name, func(t *testing.T) {
			logsDir := filepath.Join("logs", service.name)
			require.NoError(t, os.MkdirAll(logsDir, 0o750))
			testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
				TokenProvider:    provider,
				LogsDir:          logsDir,
				EnableRequestLog: true,
				EnableRawDump:    true,
				TemplateValues: map[string]string{
					"BASYX_ELEMENT_ACCESS_BASE_URL": service.baseURL,
					"BASYX_ELEMENT_ACCESS_ID":       common.EncodeString(accessSubmodelID),
					"BASYX_ELEMENT_ACCESS_OTHER_ID": common.EncodeString(accessSubmodelID + ":other"),
				},
				ActionHandlers: map[string]testenv.JSONStepAction{
					"ASSERT_AUTHORIZED_COLLECTION":  assertAuthorizedCollection,
					"ASSERT_AUTHORIZED_PAGINATION":  assertAuthorizedPagination,
					"ASSERT_NESTED_SAFE":            assertNestedSafe,
					"ASSERT_NESTED_SAFE_PAGINATION": assertNestedSafePagination,
				},
			})
		})
	}
}

func accessTokenProvider(t *testing.T) *testenv.PasswordGrantTokenProvider {
	t.Helper()
	provider := testenv.NewPasswordGrantTokenProvider(accessTokenURL, "element-access-filtering", 10*time.Second)
	for _, identity := range []struct{ user, role, bpn string }{
		{"provider", "add_digital_twin", ""},
		{"reader", "view_digital_twin", "BPNL00000003AZQP"},
		{"wrong-bpn", "view_digital_twin", "BPNL00000003OTHER"},
		{"wrong-role", "unrelated_role", "BPNL00000003AZQP"},
		{"missing-bpn", "view_digital_twin", ""},
		{"nested-first", "view_digital_twin", "NESTED_FIRST"},
		{"nested-second", "view_digital_twin", "NESTED_SECOND"},
		{"nested-both", "view_digital_twin", "NESTED_BOTH"},
	} {
		token, err := provider.GetAccessToken(&testenv.TokenCredentials{User: identity.user, Password: "pwd"})
		require.NoError(t, err, "request token for %s", identity.user)
		parts := strings.Split(token, ".")
		require.Len(t, parts, 3)
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		require.NoError(t, err)
		var claims map[string]any
		require.NoError(t, json.Unmarshal(payload, &claims))
		require.Equal(t, "http://keycloak:8080/realms/element-access-filtering", claims["iss"])
		require.Equal(t, []any{identity.role}, claims["role"], "%s role must be a string array for CLAIMPATH $contains", identity.user)
		if identity.bpn == "" {
			require.NotContains(t, claims, "Edc-Bpn", "%s must have no BPN claim", identity.user)
		} else {
			require.Equal(t, identity.bpn, claims["Edc-Bpn"], "%s BPN", identity.user)
		}
	}
	return provider
}

func expectedAuthorizedResult(endpoint string) string {
	switch {
	case strings.Contains(endpoint, "/$value"):
		return `[{"GearRatio":"public-gear-ratio"}]`
	case strings.Contains(endpoint, "/$metadata"):
		return `[{"idShort":"GearRatio","modelType":"Property","valueType":"xs:string"}]`
	case strings.Contains(endpoint, "/$reference"):
		return `[{"keys":[{"type":"Submodel","value":"urn:uuid:design-example-001"},{"type":"Property","value":"GearRatio"}],"type":"ModelReference"}]`
	case strings.Contains(endpoint, "/$path"):
		return `["GearRatio"]`
	default:
		return `[{"idShort":"GearRatio","modelType":"Property","value":"public-gear-ratio","valueType":"xs:string"}]`
	}
}

func assertAuthorizedCollection(t *testing.T, runner *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, stepNumber int) {
	t.Helper()
	response, err := runner.RunStep(step, stepNumber)
	require.NoError(t, err)
	var page struct {
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Body), &page))
	require.JSONEq(t, expectedAuthorizedResult(step.Endpoint), string(page.Result))
}

func assertAuthorizedPagination(t *testing.T, runner *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, stepNumber int) {
	t.Helper()
	current := step
	seen := map[string]bool{}
	var results []json.RawMessage
	for pageNumber := 1; pageNumber <= 4; pageNumber++ {
		response, err := runner.RunStep(current, stepNumber)
		require.NoError(t, err)
		var page struct {
			PagingMetadata struct {
				Cursor string `json:"cursor"`
			} `json:"paging_metadata"`
			Result []json.RawMessage `json:"result"`
		}
		require.NoError(t, json.Unmarshal([]byte(response.Body), &page))
		require.LessOrEqual(t, len(page.Result), 1)
		results = append(results, page.Result...)
		cursor := page.PagingMetadata.Cursor
		if cursor == "" {
			actual, err := json.Marshal(results)
			require.NoError(t, err)
			require.JSONEq(t, expectedAuthorizedResult(step.Endpoint), string(actual))
			return
		}
		require.False(t, seen[cursor], "pagination cursor must advance")
		seen[cursor] = true
		current.Endpoint = step.Endpoint + "&cursor=" + url.QueryEscape(cursor)
	}
	t.Fatal("ELEMENTGRANT-PAGINATION-TERMINATE: pagination did not terminate")
}

func assertNestedSafe(t *testing.T, runner *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, stepNumber int) {
	t.Helper()
	response, err := runner.RunStep(step, stepNumber)
	require.NoError(t, err)
	assertNoPrivateElements(t, response.Body, step.Token.User)
}

func assertNoPrivateElements(t *testing.T, body string, user string) {
	t.Helper()
	var page map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(body), &page))
	require.NotNil(t, page)
	for _, secret := range []string{"Secret", "private-", "AllowedExtra", "GroupSimilar", "MagnetGrade", "GearRatio"} {
		require.NotContains(t, body, secret)
	}
	if user == "nested-first" {
		require.NotContains(t, body, "Second")
		require.NotContains(t, body, "public-second-value")
	}
	if user == "nested-second" {
		require.NotContains(t, body, "public-nested-value")
		require.NotContains(t, body, "Group.Allowed")
	}
}

func assertNestedSafePagination(t *testing.T, runner *testenv.JSONSuiteRunner, step testenv.JSONSuiteStep, stepNumber int) {
	t.Helper()
	current := step
	seen := map[string]bool{}
	for pageNumber := 0; pageNumber < 20; pageNumber++ {
		response, err := runner.RunStep(current, stepNumber)
		require.NoError(t, err)
		assertNoPrivateElements(t, response.Body, step.Token.User)
		var page struct {
			PagingMetadata struct {
				Cursor string `json:"cursor"`
			} `json:"paging_metadata"`
			Result []json.RawMessage `json:"result"`
		}
		require.NoError(t, json.Unmarshal([]byte(response.Body), &page))
		require.LessOrEqual(t, len(page.Result), 1)
		cursor := page.PagingMetadata.Cursor
		if cursor == "" {
			return
		}
		require.False(t, seen[cursor], "pagination cursor must advance")
		seen[cursor] = true
		current.Endpoint = step.Endpoint + "&cursor=" + url.QueryEscape(cursor)
	}
	t.Fatal("ELEMENTGRANT-PAGINATION-TERMINATE: pagination did not terminate")
}

func TestMain(m *testing.M) {
	runtime := testenv.NewComposeRuntimeOrExit("element-access-filtering", []testenv.PortBinding{
		{Name: "repository", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "environment", EnvVar: "BASYX_IT_ENVIRONMENT_API_PORT"},
		{Name: "keycloak", EnvVar: "BASYX_IT_KEYCLOAK_PORT"},
	})
	accessServices = []accessService{
		{name: "submodel_repository", baseURL: runtime.LocalURL("repository") + "/submodel-repo"},
		{name: "aas_environment", baseURL: runtime.LocalURL("environment") + "/aas-environment"},
	}
	accessTokenURL = runtime.LocalhostURL("keycloak") + "/realms/element-access-filtering/protocol/openid-connect/token"
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:  "docker_compose/docker_compose.yml",
		ProjectName:  runtime.ProjectName,
		Env:          runtime.Env(),
		WaitForReady: waitForAccessServices,
	}))
}

func waitForAccessServices() error {
	for _, service := range accessServices {
		_, _ = fmt.Fprintf(os.Stderr, "Waiting for %s at %s/health\n", service.name, service.baseURL)
		if err := testenv.WaitHealthyURL(service.baseURL+"/health", 150*time.Second); err != nil {
			return fmt.Errorf("ELEMENTGRANT-TESTMAIN-WAITREADY %s: %w", service.name, err)
		}
	}
	return nil
}
