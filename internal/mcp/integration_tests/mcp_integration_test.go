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

package integrationtests

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const composeFilePath = "./docker_compose/docker_compose.yml"

var (
	apiURL        = testenv.LocalURLFromEnv("BASYX_IT_API_PORT", 6004)
	securedAPIURL = testenv.LocalURLFromEnv("BASYX_IT_SECURED_API_PORT", 6005)
	issuerURL     = testenv.LocalhostURLFromEnv("BASYX_IT_KEYCLOAK_PORT", 8080) + "/realms/basyx-mcp"
)

func TestMain(m *testing.M) {
	runtime := testenv.NewComposeRuntimeOrExit("mcp-it", []testenv.PortBinding{
		{Name: "api", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "secured-api", EnvVar: "BASYX_IT_SECURED_API_PORT"},
		{Name: "keycloak", EnvVar: "BASYX_IT_KEYCLOAK_PORT"},
	})
	apiURL = runtime.LocalURL("api")
	securedAPIURL = runtime.LocalURL("secured-api")
	issuerURL = runtime.LocalhostURL("keycloak") + "/realms/basyx-mcp"
	securityEnv := testenv.PrepareSecurityEnvOrExit("docker_compose/security_env", map[string]string{
		"http://localhost:8080": runtime.LocalhostURL("keycloak"),
	})

	code := testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:   composeFilePath,
		ProjectName:   runtime.ProjectName,
		Env:           runtime.EnvWith("BASYX_IT_SECURITY_ENV=" + securityEnv),
		HealthURL:     securedAPIURL + "/health",
		HealthTimeout: 4 * time.Minute,
		WaitForReady:  func() error { return waitForHealth(apiURL+"/health", 2*time.Minute) },
	})
	_ = os.RemoveAll(securityEnv)
	os.Exit(code)
}

func waitForHealth(endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		response, err := http.Get(endpoint) //nolint:gosec,noctx // Fixed local test endpoint.
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("MCPIT-WAITFORHEALTH-TIMEOUT: %s not healthy", endpoint)
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func connectMCP(t *testing.T, baseURL, token string) (*sdk.ClientSession, error) {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "basyx-mcp-it", Version: "1.0.0"}, nil)
	session, err := client.Connect(t.Context(), &sdk.StreamableClientTransport{
		Endpoint:   baseURL + "/mcp",
		HTTPClient: &http.Client{Transport: bearerTransport{token: token}, Timeout: 30 * time.Second},
		MaxRetries: -1,
	}, nil)
	if err == nil {
		t.Cleanup(func() { _ = session.Close() })
	}
	return session, err
}

func mustConnectMCP(t *testing.T, baseURL, token string) *sdk.ClientSession {
	t.Helper()
	session, err := connectMCP(t, baseURL, token)
	require.NoError(t, err)
	return session
}

type toolResult struct {
	isError bool
	text    string
	output  struct {
		Status      int             `json:"status"`
		ETag        string          `json:"etag"`
		Data        json.RawMessage `json:"data"`
		BodyOmitted bool            `json:"bodyOmitted"`
	}
}

func callTool(t *testing.T, session *sdk.ClientSession, name string, args map[string]any) toolResult {
	t.Helper()
	response, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	result := toolResult{isError: response.IsError}
	for _, content := range response.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			result.text += text.Text
		}
	}
	if !result.isError {
		require.NoError(t, json.Unmarshal([]byte(result.text), &result.output), result.text)
	}
	return result
}

func mustCallTool(t *testing.T, session *sdk.ClientSession, name string, args map[string]any) toolResult {
	t.Helper()
	result := callTool(t, session, name, args)
	require.False(t, result.isError, "%s failed: %s", name, result.text)
	return result
}

func toolNames(t *testing.T, session *sdk.ClientSession) []string {
	t.Helper()
	listed, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func shellFixture(id string) map[string]any {
	return map[string]any{
		"id":        id,
		"idShort":   "McpMotor",
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind":     "Instance",
			"globalAssetId": id + ":asset",
		},
	}
}

func submodelFixture(id, semanticID string) map[string]any {
	return map[string]any{
		"id":        id,
		"idShort":   "TechnicalData",
		"modelType": "Submodel",
		"semanticId": map[string]any{
			"type": "ExternalReference",
			"keys": []any{map[string]any{"type": "GlobalReference", "value": semanticID}},
		},
		"submodelElements": []any{
			map[string]any{"modelType": "Property", "idShort": "Temperature", "valueType": "xs:double", "value": "48"},
		},
	}
}

// TestMCPReadAndWriteRoundTrip runs against an AAS Environment that requires If-Match for every change.
func TestMCPReadAndWriteRoundTrip(t *testing.T) {
	session := mustConnectMCP(t, apiURL, "")
	require.Len(t, toolNames(t, session), 15)

	suffix := uuid.NewString()
	shellID := "urn:basyx:mcp-it:aas:" + suffix
	submodelID := "urn:basyx:mcp-it:sm:" + suffix
	semanticID := "urn:basyx:mcp-it:semantic:" + suffix
	shellArgs := map[string]any{"shellId": shellID}
	submodelArgs := map[string]any{"submodelId": submodelID}

	mustCallTool(t, session, "create_shell", map[string]any{"shell": shellFixture(shellID)})
	mustCallTool(t, session, "create_submodel", map[string]any{
		"submodel": submodelFixture(submodelID, semanticID), "shellId": shellID, "shellEtag": etagOf(t, session, "get_shell", shellArgs),
	})

	shell := mustCallTool(t, session, "get_shell", shellArgs)
	require.Contains(t, string(shell.output.Data), submodelID)

	listed := mustCallTool(t, session, "list_submodels", map[string]any{"semanticId": semanticID, "limit": 10})
	require.Contains(t, string(listed.output.Data), submodelID)

	queried := mustCallTool(t, session, "query_submodels", map[string]any{"query": map[string]any{
		"$condition": map[string]any{"$eq": []any{map[string]any{"$field": "$sm#id"}, map[string]any{"$strVal": submodelID}}},
	}})
	require.Contains(t, string(queried.output.Data), submodelID)

	speed := map[string]any{"modelType": "Property", "idShort": "Speed", "valueType": "xs:int", "value": "1500"}
	unconditional := callTool(t, session, "add_submodel_element", map[string]any{"submodelId": submodelID, "element": speed})
	require.True(t, unconditional.isError)
	require.Contains(t, unconditional.text, "MCP-DISPATCH-HTTP428")
	mustCallTool(t, session, "add_submodel_element", map[string]any{
		"submodelId": submodelID, "element": speed, "etag": etagOf(t, session, "get_submodel", submodelArgs),
	})

	temperature := map[string]any{"submodelId": submodelID, "idShortPath": "Temperature"}
	temperatureETag := etagOf(t, session, "get_submodel_element", temperature)
	mustCallTool(t, session, "set_submodel_element_value", map[string]any{
		"submodelId": submodelID, "idShortPath": "Temperature", "value": "50", "etag": temperatureETag,
	})
	value := mustCallTool(t, session, "get_submodel_element", map[string]any{"submodelId": submodelID, "idShortPath": "Temperature", "valueOnly": true})
	require.Contains(t, string(value.output.Data), "50")

	stale := callTool(t, session, "set_submodel_element_value", map[string]any{
		"submodelId": submodelID, "idShortPath": "Temperature", "value": "51", "etag": temperatureETag,
	})
	require.True(t, stale.isError)
	require.Contains(t, stale.text, "MCP-DISPATCH-HTTP412")

	speedPath := map[string]any{"submodelId": submodelID, "idShortPath": "Speed"}
	mustCallTool(t, session, "delete_submodel_element", withETag(speedPath, etagOf(t, session, "get_submodel_element", speedPath)))
	mustCallTool(t, session, "delete_submodel", withETag(submodelArgs, etagOf(t, session, "get_submodel", submodelArgs)))
	mustCallTool(t, session, "delete_shell", withETag(shellArgs, etagOf(t, session, "get_shell", shellArgs)))

	missing := callTool(t, session, "get_shell", shellArgs)
	require.True(t, missing.isError)
	require.Contains(t, missing.text, "MCP-DISPATCH-HTTP404")
}

// TestMCPLargeCreateStillReferencesShell creates a submodel whose response exceeds the MCP result limit.
func TestMCPLargeCreateStillReferencesShell(t *testing.T) {
	session := mustConnectMCP(t, apiURL, "")
	suffix := uuid.NewString()
	shellID := "urn:basyx:mcp-it:large:aas:" + suffix
	submodelID := "urn:basyx:mcp-it:large:sm:" + suffix
	shellArgs := map[string]any{"shellId": shellID}
	submodel := submodelFixture(submodelID, "urn:basyx:mcp-it:semantic:"+suffix)
	submodel["submodelElements"] = []any{
		map[string]any{"modelType": "Property", "idShort": "Notes", "valueType": "xs:string", "value": strings.Repeat("x", 96*1024)},
	}

	mustCallTool(t, session, "create_shell", map[string]any{"shell": shellFixture(shellID)})
	created := mustCallTool(t, session, "create_submodel", map[string]any{
		"submodel": submodel, "shellId": shellID, "shellEtag": etagOf(t, session, "get_shell", shellArgs),
	})
	require.True(t, created.output.BodyOmitted, created.text)
	require.Equal(t, http.StatusCreated, created.output.Status)
	require.NotEmpty(t, created.output.ETag, "an omitted body must still report the new resource's etag")

	shell := mustCallTool(t, session, "get_shell", shellArgs)
	require.Contains(t, string(shell.output.Data), submodelID)

	submodelArgs := map[string]any{"submodelId": submodelID}
	tooLarge := callTool(t, session, "get_submodel", submodelArgs)
	require.True(t, tooLarge.isError)
	require.Contains(t, tooLarge.text, "MCP-DISPATCH-RESULTTOOLARGE")

	mustCallTool(t, session, "delete_submodel", withETag(submodelArgs, created.output.ETag))
	mustCallTool(t, session, "delete_shell", withETag(shellArgs, etagOf(t, session, "get_shell", shellArgs)))
}

func etagOf(t *testing.T, session *sdk.ClientSession, tool string, args map[string]any) string {
	t.Helper()
	result := mustCallTool(t, session, tool, args)
	require.NotEmpty(t, result.output.ETag, "%s returned no etag", tool)
	return result.output.ETag
}

func withETag(args map[string]any, etag string) map[string]any {
	copied := map[string]any{"etag": etag}
	for key, value := range args {
		copied[key] = value
	}
	return copied
}

func TestMCPCreateWithInvalidPayloadReportsAPIError(t *testing.T) {
	session := mustConnectMCP(t, apiURL, "")

	result := callTool(t, session, "create_shell", map[string]any{"shell": map[string]any{"idShort": "NoIdentifier"}})
	require.True(t, result.isError)
	require.Contains(t, result.text, "MCP-DISPATCH-HTTP400")
}

func TestMCPSecuredEndpointRequiresBearerToken(t *testing.T) {
	response := testenv.DoHTTP(t, http.MethodPost, securedAPIURL+"/mcp", []byte(`{}`), nil)
	require.Equal(t, http.StatusUnauthorized, response.Status)
	metadataURL := securedAPIURL + "/.well-known/oauth-protected-resource/mcp"
	require.Equal(t, `Bearer resource_metadata="`+metadataURL+`"`, response.Header.Get("WWW-Authenticate"))

	invalid := testenv.DoHTTP(t, http.MethodPost, securedAPIURL+"/mcp", []byte(`{}`), map[string]string{"Authorization": "Bearer invalid"})
	require.Equal(t, http.StatusUnauthorized, invalid.Status)
	require.Equal(t, `Bearer resource_metadata="`+metadataURL+`"`, invalid.Header.Get("WWW-Authenticate"))

	_, err := connectMCP(t, securedAPIURL, "")
	require.Error(t, err)
}

func TestMCPSecuredEndpointSupportsOAuthClientDiscovery(t *testing.T) {
	metadata := getJSON(t, securedAPIURL+"/.well-known/oauth-protected-resource/mcp")
	require.Equal(t, securedAPIURL+"/mcp", metadata["resource"])
	require.Equal(t, []any{issuerURL}, metadata["authorization_servers"])

	server := getJSON(t, issuerURL+"/.well-known/openid-configuration")
	require.Contains(t, server["code_challenge_methods_supported"], "S256")
	registrationEndpoint, ok := server["registration_endpoint"].(string)
	require.True(t, ok)

	for name, scope := range map[string]string{
		"without scope":     "",
		"with openid scope": "openid profile offline_access basyx-aas-environment",
	} {
		t.Run(name, func(t *testing.T) {
			registered := registerClient(t, registrationEndpoint, "http://127.0.0.1:33418/callback", scope)
			require.Equal(t, http.StatusCreated, registered.Status, string(registered.Body))
		})
	}

	foreign := registerClient(t, registrationEndpoint, "https://client.example.org/callback", "")
	require.Equal(t, http.StatusForbidden, foreign.Status, string(foreign.Body))
}

func registerClient(t *testing.T, registrationEndpoint, redirectURI, scope string) testenv.HTTPResult {
	t.Helper()
	request := map[string]any{
		"client_name":                "BaSyx MCP integration test",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	if scope != "" {
		request["scope"] = scope
	}
	registration, err := json.Marshal(request)
	require.NoError(t, err)
	return testenv.DoHTTP(t, http.MethodPost, registrationEndpoint, registration, nil)
}

func TestMCPSecuredEndpointAppliesCallerPermissions(t *testing.T) {
	alice := mustConnectMCP(t, securedAPIURL, fetchToken(t, "alice"))
	bob := mustConnectMCP(t, securedAPIURL, fetchToken(t, "bob"))
	require.Len(t, toolNames(t, bob), 15)

	shellID := "urn:basyx:mcp-it:secured:aas:" + uuid.NewString()
	mustCallTool(t, alice, "create_shell", map[string]any{"shell": shellFixture(shellID)})

	read := mustCallTool(t, bob, "get_shell", map[string]any{"shellId": shellID})
	require.Contains(t, string(read.output.Data), shellID)

	denied := callTool(t, bob, "delete_shell", map[string]any{"shellId": shellID})
	require.True(t, denied.isError)
	require.Regexp(t, "MCP-DISPATCH-HTTP40[34]", denied.text)
	mustCallTool(t, bob, "get_shell", map[string]any{"shellId": shellID})
	mustCallTool(t, alice, "delete_shell", map[string]any{"shellId": shellID})
}

// fetchToken requests an offline refresh token like Codex does, with a client limited to its scopes.
func fetchToken(t *testing.T, user string) string {
	t.Helper()
	form := url.Values{
		"grant_type": {"password"},
		"client_id":  {"basyx-mcp-test"},
		"username":   {user},
		"password":   {user},
		"scope":      {"openid offline_access"},
	}
	response, err := http.PostForm(issuerURL+"/protocol/openid-connect/token", form) //nolint:gosec,noctx // Fixed local test endpoint.
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	require.NoError(t, json.Unmarshal(body, &token))
	require.NotEmpty(t, token.AccessToken)
	require.Equal(t, "Offline", jwtClaim(t, token.RefreshToken, "typ"))
	return token.AccessToken
}

func jwtClaim(t *testing.T, token, claim string) any {
	t.Helper()
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	claims := map[string]any{}
	require.NoError(t, json.Unmarshal(payload, &claims))
	return claims[claim]
}

func getJSON(t *testing.T, endpoint string) map[string]any {
	t.Helper()
	response := testenv.DoHTTP(t, http.MethodGet, endpoint, nil, nil)
	require.Equal(t, http.StatusOK, response.Status, string(response.Body))
	document := map[string]any{}
	require.NoError(t, json.Unmarshal(response.Body, &document))
	return document
}
