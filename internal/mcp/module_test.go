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

package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/go-chi/chi/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const testToken = "Bearer test-token"

type recordedRequest struct {
	method, path, rawQuery, authorization, ifMatch, body string
}

type fakeAPI struct {
	mu       sync.Mutex
	requests []recordedRequest
	respond  func(w http.ResponseWriter, r *http.Request)
}

func (a *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	a.mu.Lock()
	a.requests = append(a.requests, recordedRequest{
		method: r.Method, path: r.URL.EscapedPath(), rawQuery: r.URL.RawQuery,
		authorization: r.Header.Get("Authorization"), ifMatch: r.Header.Get("If-Match"), body: string(body),
	})
	a.mu.Unlock()
	if a.respond != nil {
		a.respond(w, r)
		return
	}
	w.Header().Set("ETag", `"1-abc"`)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"idShort":"Example"}`))
}

func (a *fakeAPI) recorded() []recordedRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]recordedRequest(nil), a.requests...)
}

func testConfig(writesEnabled bool) *common.Config {
	cfg := &common.Config{MCP: common.MCPConfig{Path: "/mcp", WritesEnabled: writesEnabled, MaxResultBytes: 4096}}
	cfg.Server.Port = 8081
	cfg.Server.ContextPath = "/api"
	return cfg
}

type headerTransport struct{ authorization string }

func (t headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", t.authorization)
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, module *Module) *sdk.ClientSession {
	t.Helper()
	router := chi.NewRouter()
	module.Register(router)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	transport := &sdk.StreamableClientTransport{
		Endpoint:   server.URL + "/api/mcp",
		HTTPClient: &http.Client{Transport: headerTransport{authorization: testToken}},
	}
	session, err := client.Connect(t.Context(), transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func newTestModule(t *testing.T, writesEnabled bool, api http.Handler) *Module {
	t.Helper()
	module, err := New(t.Context(), testConfig(writesEnabled), api)
	require.NoError(t, err)
	return module
}

func toolsByName(t *testing.T, session *sdk.ClientSession) map[string]*sdk.Tool {
	t.Helper()
	listed, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	tools := map[string]*sdk.Tool{}
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
	}
	return tools
}

func callTool(t *testing.T, session *sdk.ClientSession, name string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return result
}

func resultText(result *sdk.CallToolResult) string {
	var texts []string
	for _, content := range result.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func TestWriteToolsAreListedOnlyWhenEnabled(t *testing.T) {
	readTools := []string{"list_shells", "get_shell", "list_submodels", "get_submodel", "get_submodel_element", "get_concept_description", "query_shells", "query_submodels"}
	writeTools := []string{"create_shell", "create_submodel", "add_submodel_element", "set_submodel_element_value", "delete_shell", "delete_submodel", "delete_submodel_element"}

	readOnly := toolsByName(t, connect(t, newTestModule(t, false, &fakeAPI{})))
	require.Len(t, readOnly, len(readTools))
	for _, name := range readTools {
		require.Contains(t, readOnly, name)
		require.True(t, readOnly[name].Annotations.ReadOnlyHint, name)
	}
	schema, err := json.Marshal(readOnly["list_shells"].InputSchema)
	require.NoError(t, err)
	require.Contains(t, string(schema), `"limit"`)
	require.Contains(t, string(schema), `"cursor"`)

	writable := toolsByName(t, connect(t, newTestModule(t, true, &fakeAPI{})))
	require.Len(t, writable, len(readTools)+len(writeTools))
	for _, name := range writeTools {
		require.Contains(t, writable, name)
		require.False(t, writable[name].Annotations.ReadOnlyHint, name)
	}
	require.False(t, *writable["create_shell"].Annotations.DestructiveHint)
	require.True(t, *writable["delete_submodel"].Annotations.DestructiveHint)
	require.True(t, writable["set_submodel_element_value"].Annotations.IdempotentHint)
}

func TestReadToolForwardsCallerTokenAndReturnsETag(t *testing.T) {
	api := &fakeAPI{}
	session := connect(t, newTestModule(t, false, api))

	result := callTool(t, session, "get_submodel_element", map[string]any{
		"submodelId": "urn:example:sm", "idShortPath": "Measurements[0].Temperature", "valueOnly": true,
	})
	require.False(t, result.IsError, resultText(result))

	requests := api.recorded()
	require.Len(t, requests, 1)
	require.Equal(t, http.MethodGet, requests[0].method)
	require.Equal(t, "/api/submodels/"+common.EncodeString("urn:example:sm")+"/submodel-elements/Measurements%5B0%5D.Temperature/$value", requests[0].path)
	require.Equal(t, testToken, requests[0].authorization)

	output := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(resultText(result)), &output))
	require.Equal(t, `"1-abc"`, output["etag"])
	require.Equal(t, map[string]any{"idShort": "Example"}, output["data"])
}

func TestListAndQueryToolsPassFiltersAndPaging(t *testing.T) {
	api := &fakeAPI{}
	session := connect(t, newTestModule(t, false, api))

	callTool(t, session, "list_submodels", map[string]any{"semanticId": "urn:sem", "limit": 5, "cursor": "next"})
	callTool(t, session, "query_shells", map[string]any{"query": map[string]any{"$condition": map[string]any{"$eq": []any{}}}})

	requests := api.recorded()
	require.Len(t, requests, 2)
	require.Equal(t, "/api/submodels", requests[0].path)
	require.Equal(t, "cursor=next&limit=5&semanticId="+common.EncodeString(`{"keys":[{"type":"GlobalReference","value":"urn:sem"}],"type":"ExternalReference"}`), requests[0].rawQuery)
	require.Equal(t, http.MethodPost, requests[1].method)
	require.Equal(t, "/api/query/shells", requests[1].path)
	require.JSONEq(t, `{"$condition":{"$eq":[]}}`, requests[1].body)
}

func TestWriteToolsSendConditionalRequests(t *testing.T) {
	api := &fakeAPI{}
	session := connect(t, newTestModule(t, true, api))

	result := callTool(t, session, "set_submodel_element_value", map[string]any{
		"submodelId": "urn:example:sm", "idShortPath": "Speed", "value": "42", "etag": `"1-abc"`,
	})
	require.False(t, result.IsError, resultText(result))
	result = callTool(t, session, "delete_shell", map[string]any{"shellId": "urn:example:aas", "etag": `"2-def"`})
	require.False(t, result.IsError, resultText(result))

	requests := api.recorded()
	require.Equal(t, http.MethodPatch, requests[0].method)
	require.Equal(t, "/api/submodels/"+common.EncodeString("urn:example:sm")+"/submodel-elements/Speed/$value", requests[0].path)
	require.Equal(t, `"42"`, requests[0].body)
	require.Equal(t, `"1-abc"`, requests[0].ifMatch)
	require.Equal(t, http.MethodDelete, requests[1].method)
	require.Equal(t, "/api/shells/"+common.EncodeString("urn:example:aas"), requests[1].path)
	require.Equal(t, `"2-def"`, requests[1].ifMatch)
}

func TestCreateSubmodelReferencesItFromShell(t *testing.T) {
	api := &fakeAPI{}
	session := connect(t, newTestModule(t, true, api))

	result := callTool(t, session, "create_submodel", map[string]any{
		"submodel": map[string]any{"id": "urn:example:sm", "idShort": "Example", "modelType": "Submodel"},
		"shellId":  "urn:example:aas",
	})
	require.False(t, result.IsError, resultText(result))

	requests := api.recorded()
	require.Len(t, requests, 2)
	require.Equal(t, "/api/submodels", requests[0].path)
	require.Equal(t, "/api/shells/"+common.EncodeString("urn:example:aas")+"/submodel-refs", requests[1].path)
	require.JSONEq(t, `{"type":"ModelReference","keys":[{"type":"Submodel","value":"urn:example:sm"}]}`, requests[1].body)
}

func TestCreateSubmodelReportsMissingShellReference(t *testing.T) {
	api := &fakeAPI{respond: func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/submodel-refs") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"urn:example:sm"}`))
	}}
	session := connect(t, newTestModule(t, true, api))

	result := callTool(t, session, "create_submodel", map[string]any{
		"submodel": map[string]any{"id": "urn:example:sm"}, "shellId": "urn:missing",
	})
	require.True(t, result.IsError)
	require.Contains(t, resultText(result), "MCP-CREATESUBMODEL-ADDREFERENCE")
	require.Contains(t, resultText(result), "the submodel was created")
}

func TestAPIErrorsAreActionableToolErrors(t *testing.T) {
	api := &fakeAPI{respond: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"messages":[{"text":"ETag mismatch"}]}`))
	}}
	session := connect(t, newTestModule(t, true, api))

	result := callTool(t, session, "delete_submodel", map[string]any{"submodelId": "urn:example:sm", "etag": `"stale"`})
	require.True(t, result.IsError)
	text := resultText(result)
	require.Contains(t, text, "MCP-DISPATCH-HTTP412")
	require.Contains(t, text, "ETag mismatch")
	require.Contains(t, text, "read it again")
}

func TestOversizedAPIResultIsRejected(t *testing.T) {
	api := &fakeAPI{respond: func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`"` + strings.Repeat("x", 8192) + `"`))
	}}
	session := connect(t, newTestModule(t, false, api))

	result := callTool(t, session, "get_submodel", map[string]any{"submodelId": "urn:example:sm"})
	require.True(t, result.IsError)
	require.Contains(t, resultText(result), "MCP-DISPATCH-RESULTTOOLARGE")
}

func TestSecuredEndpointChallengesAndPublishesMetadata(t *testing.T) {
	module := newTestModule(t, false, &fakeAPI{})
	module.verifyToken = func(next http.Handler) http.Handler { return next }
	module.authorizationServers = []string{"https://issuer.example.org/realms/basyx"}
	router := chi.NewRouter()
	module.Register(router)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/mcp", strings.NewReader(`{}`)))
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	require.Equal(t, `Bearer resource_metadata="http://localhost:8081/.well-known/oauth-protected-resource/api/mcp"`, unauthenticated.Header().Get("WWW-Authenticate"))

	metadata := httptest.NewRecorder()
	router.ServeHTTP(metadata, httptest.NewRequest(http.MethodGet, ProtectedResourceMetadataPrefix+"/api/mcp", nil))
	require.Equal(t, http.StatusOK, metadata.Code)
	require.JSONEq(t, `{
		"resource": "http://localhost:8081/api/mcp",
		"authorization_servers": ["https://issuer.example.org/realms/basyx"],
		"bearer_methods_supported": ["header"],
		"resource_name": "Eclipse BaSyx AAS Environment"
	}`, metadata.Body.String())

	session := connect(t, module)
	require.Contains(t, toolsByName(t, session), "get_shell")
}

func TestUnsecuredEndpointPublishesNoMetadata(t *testing.T) {
	router := chi.NewRouter()
	newTestModule(t, false, &fakeAPI{}).Register(router)

	metadata := httptest.NewRecorder()
	router.ServeHTTP(metadata, httptest.NewRequest(http.MethodGet, ProtectedResourceMetadataPrefix+"/api/mcp", nil))
	require.Equal(t, http.StatusNotFound, metadata.Code)
}
