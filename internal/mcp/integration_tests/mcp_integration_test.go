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

package integration_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/aol"
	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func endpoint(t *testing.T) string {
	t.Helper()
	value := os.Getenv("BASYX_MCP_IT_URL")
	if value == "" {
		t.Skip("set BASYX_MCP_IT_URL=http://localhost:18584/mcp after starting BaSyxMCPExample")
	}
	return value
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	request := r.Clone(r.Context())
	request.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(request)
}
func userToken(t *testing.T, user string) string {
	t.Helper()
	response, err := http.PostForm("http://localhost:18580/realms/basyx-mcp/protocol/openid-connect/token", url.Values{"grant_type": {"password"}, "client_id": {"basyx-mcp"}, "username": {user}, "password": {"sandbox-" + user}, "scope": {"openid"}})
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, 200, response.StatusCode)
	var token struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&token))
	require.NotEmpty(t, token.AccessToken)
	return token.AccessToken
}
func connect(t *testing.T, user, profile string) *sdk.ClientSession {
	t.Helper()
	base := endpoint(t)
	token := userToken(t, user)
	client := &http.Client{Transport: bearerTransport{token: token, base: http.DefaultTransport}, Timeout: 45 * time.Second}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "BaSyx MVP integration", Version: "1"}, nil).Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: base + "?profile=" + profile, HTTPClient: client}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}
func call(t *testing.T, c *sdk.ClientSession, name string, args any) map[string]any {
	t.Helper()
	result, err := c.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, result.IsError, "tool %s failed: %v", name, result.StructuredContent)
	raw, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(raw, &value))
	return value
}
func TestLiveDiscoveryReadDescriptionQueryAndDocs(t *testing.T) {
	c := connect(t, "alice", "analyst")
	found := call(t, c, "find_asset", map[string]any{"query": "M-4711"})
	require.Len(t, found["result"], 1)
	value := call(t, c, "get_element", map[string]any{"submodel": "urn:example:sm:technical-public", "path": "Temperature"})
	require.Equal(t, float64(48), value["data"])
	require.Equal(t, "degC", value["unit"])
	require.Contains(t, value["source"], "Temperature")
	description := call(t, c, "describe_asset", map[string]any{"shell": "urn:example:aas:motor-public"})
	require.NotEmpty(t, description["submodels"])
	condition := map[string]any{"$condition": map[string]any{"$eq": []any{map[string]any{"$field": "$sm#idShort"}, map[string]any{"$strVal": "TechnicalData"}}}}
	dry := call(t, c, "query_aasql", map[string]any{"target": "submodels", "aasql": condition, "dry_run": true})
	require.Equal(t, false, dry["executed"])
	query := call(t, c, "query_aasql", map[string]any{"target": "submodels", "aasql": condition})
	require.Len(t, query["result"], 2)
	docs := call(t, c, "search_docs", map[string]any{"query": "OTEL tracing"})
	require.NotEmpty(t, docs["result"])
	for _, uri := range []string{"aasql://grammar", "aasql://fields", "basyx-docs://admin/telemetry.md"} {
		resource, err := c.ReadResource(t.Context(), &sdk.ReadResourceParams{URI: uri})
		require.NoError(t, err)
		require.NotEmpty(t, resource.Contents)
	}
	prompt, err := c.GetPrompt(t.Context(), &sdk.GetPromptParams{Name: "ask_asset", Arguments: map[string]string{"shell": "urn:example:aas:motor-public", "question": "What is its temperature?"}})
	require.NoError(t, err)
	require.NotEmpty(t, prompt.Messages)
}
func TestLiveAggregationsEqualCallerVisibleData(t *testing.T) {
	for _, test := range []struct {
		user, average string
		count         int
	}{{"alice", "18.0000000000000000", 2}, {"bob", "12.0000000000000000", 1}, {"carol", "18.0000000000000000", 2}} {
		t.Run(test.user, func(t *testing.T) {
			c := connect(t, test.user, "analyst")
			result := call(t, c, "run_read_model", map[string]any{"name": "value_statistics_by_semantic_id", "params": map[string]any{"semanticId": "urn:example:rated-power"}})
			rows := result["result"].([]any)
			require.Len(t, rows, 1)
			row := rows[0].(map[string]any)
			require.Equal(t, float64(test.count), row["count"])
			require.Equal(t, test.average, row["avg"])
			require.Equal(t, "kW", row["unit"])
			counts := call(t, c, "run_read_model", map[string]any{"name": "count_submodels_by_semantic_id", "params": map[string]any{}})
			require.NotEmpty(t, counts["result"])
		})
	}
	denied := connect(t, "eve", "analyst")
	result, err := denied.CallTool(t.Context(), &sdk.CallToolParams{Name: "get_shell", Arguments: map[string]any{"shell": "urn:example:aas:motor-public"}})
	require.NoError(t, err)
	require.True(t, result.IsError)
}
func TestLiveProfileAndAuthenticationGates(t *testing.T) {
	base := endpoint(t)
	request, err := http.NewRequestWithContext(t.Context(), "POST", base, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, 401, response.StatusCode)
	require.Contains(t, response.Header.Get("WWW-Authenticate"), "resource_metadata")
	c := connect(t, "alice", "default")
	tools, err := c.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.LessOrEqual(t, len(tools.Tools), 15)
	_, err = c.CallTool(t.Context(), &sdk.CallToolParams{Name: "apply_plan", Arguments: map[string]any{"plan_id": "absent", "confirm_token": "forged"}})
	require.Error(t, err)
}
func TestLiveHumanApprovalCreationVerificationAndRollback(t *testing.T) {
	c := connect(t, "alice", "operator")
	draft := newDraft(t)
	report := call(t, c, "validate_aas_draft", map[string]any{"draft": draft})
	require.Equal(t, true, report["valid"])
	plan := call(t, c, "plan_create_aas", map[string]any{"draft": draft})
	id := plan["plan_id"].(string)
	denied, err := c.CallTool(t.Context(), &sdk.CallToolParams{Name: "apply_plan", Arguments: map[string]any{"plan_id": id, "confirm_token": "forged"}})
	require.NoError(t, err)
	require.True(t, denied.IsError)
	token := approveInTestBrowser(t, plan["confirmation_url"].(string))
	applied := call(t, c, "apply_plan", map[string]any{"plan_id": id, "confirm_token": token})
	require.Equal(t, "applied", applied["status"])
	replay, err := c.CallTool(t.Context(), &sdk.CallToolParams{Name: "apply_plan", Arguments: map[string]any{"plan_id": id, "confirm_token": token}})
	require.NoError(t, err)
	require.True(t, replay.IsError)
	changeset := call(t, c, "get_changeset", map[string]any{"id": id})
	rollbackToken := approveInTestBrowser(t, changeset["confirmation_url"].(string))
	rolledBack := call(t, c, "rollback_changeset", map[string]any{"id": id, "confirm_token": rollbackToken})
	require.Equal(t, "rolled_back", rolledBack["status"])
	missing, err := c.CallTool(t.Context(), &sdk.CallToolParams{Name: "get_shell", Arguments: map[string]any{"shell": draft.Shell["id"]}})
	require.NoError(t, err)
	require.True(t, missing.IsError)
}
func newDraft(t *testing.T) aol.Draft {
	t.Helper()
	raw, err := os.ReadFile("../../../examples/BaSyxMCPExample/draft.json")
	require.NoError(t, err)
	var draft aol.Draft
	require.NoError(t, json.Unmarshal(raw, &draft))
	suffix := uuid.NewString()
	draft.Shell["id"] = fmt.Sprint(draft.Shell["id"]) + ":" + suffix
	id := fmt.Sprint(draft.Submodels[0]["id"])
	slug := draft.Templates[id]
	id += ":" + suffix
	draft.Submodels[0]["id"] = id
	draft.Templates = map[string]string{id: slug}
	return draft
}

// browserTransport models the localhost secure-cookie exception used by browsers.
// It changes only test responses from the fixed local Keycloak sandbox.
type browserTransport struct{ base http.RoundTripper }

func (b browserTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := b.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	if r.URL.Host == "keycloak.localhost:18580" {
		cookies := response.Cookies()
		response.Header.Del("Set-Cookie")
		for _, cookie := range cookies {
			cookie.Secure = false
			response.Header.Add("Set-Cookie", cookie.String())
		}
	}
	return response, nil
}
func approveInTestBrowser(t *testing.T, uri string) string {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == "keycloak.localhost:18580" {
			address = "127.0.0.1:18580"
		}
		return dialer.DialContext(ctx, network, address)
	}
	client := &http.Client{Jar: jar, Transport: browserTransport{base: transport}, Timeout: 30 * time.Second}
	response, err := client.Get(uri)
	require.NoError(t, err)
	page := parsePage(t, response)
	response, err = client.PostForm(page.action, url.Values{"username": {"alice"}, "password": {"sandbox-alice"}, "credentialId": {""}})
	require.NoError(t, err)
	page = parsePage(t, response)
	require.NotEmpty(t, page.inputs["csrf"])
	form := url.Values{"csrf": {page.inputs["csrf"]}, "action": {page.inputs["action"]}, "approve": {"yes"}}
	request, err := http.NewRequestWithContext(t.Context(), "POST", uri, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parsed, err := url.Parse(uri)
	require.NoError(t, err)
	request.Header.Set("Origin", parsed.Scheme+"://"+parsed.Host)
	response, err = client.Do(request)
	require.NoError(t, err)
	page = parsePage(t, response)
	require.NotEmpty(t, page.pre)
	token := page.pre[len(page.pre)-1]
	require.Len(t, token, 64)
	return token
}

type htmlPage struct {
	action string
	inputs map[string]string
	pre    []string
}

func parsePage(t *testing.T, response *http.Response) htmlPage {
	t.Helper()
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1048576))
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode, "approval response status")
	root, err := html.Parse(strings.NewReader(string(raw)))
	require.NoError(t, err)
	page := htmlPage{inputs: map[string]string{}}
	walkPage(root, &page)
	return page
}
func walkPage(node *html.Node, page *htmlPage) {
	if node.Type == html.ElementNode {
		attributes := map[string]string{}
		for _, a := range node.Attr {
			attributes[a.Key] = a.Val
		}
		if node.Data == "form" {
			page.action = attributes["action"]
		}
		if node.Data == "input" {
			page.inputs[attributes["name"]] = attributes["value"]
		}
		if node.Data == "pre" {
			page.pre = append(page.pre, nodeText(node))
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkPage(child, page)
	}
}
func nodeText(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var result strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		_, _ = result.WriteString(nodeText(child))
	}
	return result.String()
}

func TestLiveSagaCompensatesEarlierCreatesOnDuplicateID(t *testing.T) {
	c := connect(t, "alice", "operator")
	draft := newDraft(t)
	raw, err := json.Marshal(draft.Submodels[0])
	require.NoError(t, err)
	var duplicate map[string]any
	require.NoError(t, json.Unmarshal(raw, &duplicate))
	duplicate["id"] = "urn:example:sm:technical-public"
	draft.Submodels = append(draft.Submodels, duplicate)
	draft.Templates["urn:example:sm:technical-public"] = "demo-technical-data"
	plan := call(t, c, "plan_create_aas", map[string]any{"draft": draft})
	token := approveInTestBrowser(t, plan["confirmation_url"].(string))
	result, err := c.CallTool(t.Context(), &sdk.CallToolParams{Name: "apply_plan", Arguments: map[string]any{"plan_id": plan["plan_id"], "confirm_token": token}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	encoded, _ := json.Marshal(result.StructuredContent)
	var response map[string]any
	require.NoError(t, json.Unmarshal(encoded, &response))
	state := response["result"].(map[string]any)
	require.Equal(t, "compensated", state["status"])
	missing, err := c.CallTool(t.Context(), &sdk.CallToolParams{Name: "get_submodel_value", Arguments: map[string]any{"submodel": draft.Submodels[0]["id"]}})
	require.NoError(t, err)
	require.True(t, missing.IsError)
}
func TestLiveRollbackAbortsWhenAResourceChanged(t *testing.T) {
	c := connect(t, "alice", "operator")
	draft := newDraft(t)
	plan := call(t, c, "plan_create_aas", map[string]any{"draft": draft})
	id := plan["plan_id"].(string)
	token := approveInTestBrowser(t, plan["confirmation_url"].(string))
	_ = call(t, c, "apply_plan", map[string]any{"plan_id": id, "confirm_token": token})
	t.Cleanup(func() {
		restJSON(t, "DELETE", "/shells/"+aol.EncodeID(draft.Shell["id"].(string)), nil)
		restJSON(t, "DELETE", "/submodels/"+aol.EncodeID(draft.Submodels[0]["id"].(string)), nil)
	})
	elements := draft.Submodels[0]["submodelElements"].([]any)
	elements[1].(map[string]any)["value"] = "49"
	status := restJSON(t, "PUT", "/submodels/"+aol.EncodeID(draft.Submodels[0]["id"].(string)), draft.Submodels[0])
	require.True(t, status >= 200 && status < 300)
	changeset := call(t, c, "get_changeset", map[string]any{"id": id})
	token = approveInTestBrowser(t, changeset["confirmation_url"].(string))
	result, err := c.CallTool(t.Context(), &sdk.CallToolParams{Name: "rollback_changeset", Arguments: map[string]any{"id": id, "confirm_token": token}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, 200, restJSON(t, "GET", "/shells/"+aol.EncodeID(draft.Shell["id"].(string)), nil))
}
func restJSON(t *testing.T, method, path string, body any) int {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(endpoint(t), "/mcp")+path, strings.NewReader(string(raw)))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+userToken(t, "alice"))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	return response.StatusCode
}

func TestLiveReadToolsPreserveCallerPermissions(t *testing.T) {
	for _, user := range []string{"alice", "bob", "eve"} {
		t.Run(user, func(t *testing.T) {
			client := connect(t, user, "analyst")
			assertReadVisibility(t, client, user)
		})
	}
	denied := connect(t, "eve", "analyst")
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"find_asset", map[string]any{"query": "M-4711"}},
		{"query_aasql", map[string]any{"target": "submodels", "aasql": map[string]any{"$condition": map[string]any{"$boolean": true}}}},
		{"run_read_model", map[string]any{"name": "count_submodels_by_semantic_id", "params": map[string]any{}}},
	} {
		result, err := denied.CallTool(t.Context(), &sdk.CallToolParams{Name: test.name, Arguments: test.args})
		require.NoError(t, err)
		require.True(t, result.IsError, test.name)
	}
	for _, user := range []string{"bob", "eve"} {
		operator := connect(t, user, "operator")
		result, err := operator.CallTool(t.Context(), &sdk.CallToolParams{Name: "plan_create_aas", Arguments: map[string]any{"draft": newDraft(t)}})
		require.NoError(t, err)
		require.True(t, result.IsError, "read-only users cannot plan creates")
	}
}

func assertReadVisibility(t *testing.T, client *sdk.ClientSession, user string) {
	t.Helper()
	for _, visibility := range []string{"public", "private"} {
		shell := "urn:example:aas:motor-" + visibility
		submodel := "urn:example:sm:technical-" + visibility
		cases := []struct {
			name string
			args map[string]any
		}{
			{"get_shell", map[string]any{"shell": shell}},
			{"list_submodels_of_shell", map[string]any{"shell": shell}},
			{"describe_asset", map[string]any{"shell": shell}},
			{"get_submodel_value", map[string]any{"submodel": submodel}},
			{"get_element", map[string]any{"submodel": submodel, "path": "Temperature"}},
		}
		for _, test := range cases {
			t.Run(visibility+"/"+test.name, func(t *testing.T) {
				result, err := client.CallTool(t.Context(), &sdk.CallToolParams{Name: test.name, Arguments: test.args})
				require.NoError(t, err)
				allowed := user == "alice" || (user == "bob" && visibility == "public")
				require.Equal(t, !allowed, result.IsError, "tool response: %v", result.StructuredContent)
			})
		}
	}
}
