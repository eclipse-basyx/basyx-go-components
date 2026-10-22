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

// Package mcp adapts the MCP protocol to deterministic AOL operations.
package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	docassets "github.com/eclipse-basyx/basyx-go-components/docu"
	"github.com/eclipse-basyx/basyx-go-components/internal/aol"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

// Module owns process-lifetime budgets and stateless protocol handlers.
type Module struct {
	security   auth.ABACSettings
	cfg        common.MCPConfig
	reader     aol.Reader
	operations aol.Operations
	docs       []aol.Document
	aggregates Aggregates
	auth       func(http.Handler) http.Handler
	transport  http.Handler
	browser    *browserAuth
	semaphore  chan struct{}
	mu         sync.Mutex
	rates      map[string]window
	calls      metric.Int64Counter
	latency    metric.Float64Histogram
	bytes      metric.Int64Histogram
}
type window struct {
	start time.Time
	count int
}

// New constructs an optional module without opening another HTTP listener.
func New(ctx context.Context, cfg *common.Config, router http.Handler, db, reader *sql.DB, openapi []byte, security ...auth.ABACSettings) (*Module, error) {
	if err := cfg.MCP.Validate(); err != nil {
		return nil, err
	}
	m := &Module{cfg: cfg.MCP, semaphore: make(chan struct{}, cfg.MCP.Limits.MaxConcurrentCalls), rates: map[string]window{}}
	if len(security) > 0 {
		m.security = security[0]
	}
	if !cfg.MCP.Enabled {
		return m, nil
	}
	if err := m.initAuthentication(ctx, cfg); err != nil {
		return nil, err
	}
	if err := m.validateProfiles(); err != nil {
		return nil, err
	}
	templates, err := aol.LoadTemplates(cfg.MCP.TemplateDirectory, cfg.MCP.AllowedTemplates)
	if err != nil {
		return nil, err
	}
	dispatch := aol.Dispatcher{Router: router, MaxBytes: cfg.MCP.Limits.MaxResultBytes, BasePath: common.NormalizeBasePath(cfg.Server.ContextPath)}
	m.reader = aol.Reader{Dispatch: dispatch, PageSize: cfg.MCP.Limits.MaxItemsPerPage, MaxScan: cfg.MCP.Limits.MaxScanItems}
	m.operations = aol.Operations{Store: aol.SQLStore{DB: db}, Validator: aol.Validator{Templates: templates}, Dispatch: dispatch, TTL: time.Duration(cfg.MCP.Limits.PlanTTLSeconds) * time.Second, PublicURL: cfg.MCP.PublicURL, Preflight: m.authorizeDraft, Lease: time.Duration(cfg.MCP.Limits.TimeoutSeconds+90) * time.Second}
	m.docs, err = aol.Documents(docassets.Files)
	if err != nil {
		return nil, err
	}
	m.docs = append(m.docs, aol.Document{URI: "basyx-docs://openapi.yaml", Text: string(openapi)})
	m.aggregates = Aggregates{Reader: m.reader, DB: reader, Timeout: time.Duration(cfg.MCP.Limits.TimeoutSeconds) * time.Second}
	if err = m.initMetrics(); err != nil {
		return nil, err
	}
	if err = m.initBrowserAuth(ctx); err != nil {
		return nil, err
	}
	m.transport = sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server { return m.server(aol.IdentityFromContext(r.Context()).Profile) }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return m, nil
}
func (m *Module) initMetrics() error {
	meter := otel.Meter("basyx/mcp")
	var err error
	if m.calls, err = meter.Int64Counter("mcp.tool.calls"); err != nil {
		return err
	}
	if m.latency, err = meter.Float64Histogram("mcp.tool.duration", metric.WithUnit("s")); err != nil {
		return err
	}
	m.bytes, err = meter.Int64Histogram("mcp.tool.response_bytes", metric.WithUnit("By"))
	return err
}

// Handler returns the protected protocol and approval endpoint handler.
func (m *Module) Handler() http.Handler {
	if !m.cfg.Enabled {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasSuffix(r.URL.Path, "/metadata") {
			m.metadata(w)
			return
		}
		request, handled := m.browserRequest(w, r)
		if handled {
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+strings.TrimRight(m.cfg.PublicURL, "/")+`/metadata"`)
		m.auth(http.HandlerFunc(m.serveAuthenticated)).ServeHTTP(w, request)
	})
}
func (m *Module) serveAuthenticated(w http.ResponseWriter, r *http.Request) {
	identity := requestIdentity(r)
	if identity.Subject == "" && !m.cfg.AllowAnonymous {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+strings.TrimRight(m.cfg.PublicURL, "/")+`/metadata"`)
		writeHTTPError(w, 401, aol.Failure("MCP-AUTH-REQUIRED", "authentication required", "Authenticate with the configured OIDC provider."))
		return
	}
	profile := r.URL.Query().Get("profile")
	if profile == "" {
		profile = r.Header.Get("Mcp-Profile")
	}
	if profile == "" {
		profile = "default"
	}
	if _, ok := m.cfg.Profiles[profile]; !ok {
		writeHTTPError(w, 400, aol.Failure("MCP-PROFILE-UNKNOWN", "unknown profile", "Select a configured profile."))
		return
	}
	identity.Profile = profile
	if !m.allow(identity) {
		writeHTTPError(w, 429, aol.Failure("MCP-LIMIT-RATE", "request rate limit exceeded", "Wait for the next minute before retrying."))
		return
	}
	ctx := aol.ContextWithIdentity(r.Context(), identity)
	r = r.WithContext(ctx)
	if strings.Contains(r.URL.Path, "/confirm/") {
		m.confirm(w, r)
		return
	}
	w.Header().Del("WWW-Authenticate")
	r.Body = http.MaxBytesReader(w, r.Body, m.cfg.Limits.MaxRequestBytes)
	raw, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		writeHTTPError(w, 413, aol.Failure("MCP-LIMIT-REQUESTBYTES", "request exceeds byte budget", "Reduce the request size."))
		return
	}
	if depthErr := requestDepth(raw); depthErr != nil {
		writeHTTPError(w, 400, depthErr)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	aol.BoundedHTTP(m.transport, m.cfg.Limits.MaxResultBytes).ServeHTTP(w, r)
}
func requestIdentity(r *http.Request) aol.Identity {
	claims := auth.ClaimsFromContext(r.Context())
	subject, _ := claims.GetString("sub")
	issuer, _ := claims.GetString("iss")
	token := r.Header.Get("Authorization")
	sum := sha256.Sum256([]byte(token))
	return aol.Identity{Subject: subject, Issuer: issuer, Token: token, SessionID: hex.EncodeToString(sum[:12])}
}
func (m *Module) allow(identity aol.Identity) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	key := identity.Issuer + "\x00" + identity.Subject
	if identity.Subject == "" {
		key = "anonymous"
	}
	entry := m.rates[key]
	if now.Sub(entry.start) >= time.Minute {
		entry = window{start: now}
	}
	if entry.count >= m.cfg.Limits.CallsPerMinute {
		return false
	}
	if len(m.rates) >= 10000 {
		for k, v := range m.rates {
			if now.Sub(v.start) >= time.Minute {
				delete(m.rates, k)
			}
		}
		if len(m.rates) >= 10000 {
			return false
		}
	}
	entry.count++
	m.rates[key] = entry
	return true
}
func (m *Module) metadata(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"resource": m.cfg.PublicURL, "authorization_servers": m.cfg.AuthorizationServers, "bearer_methods_supported": []string{"header"}})
}
func writeHTTPError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(err)
}
func (m *Module) execute(ctx context.Context, tool tool, args json.RawMessage) (*sdk.CallToolResult, error) {
	ctx, span := otel.Tracer("basyx/mcp").Start(ctx, "mcp.tool/"+tool.name)
	defer span.End()
	span.SetAttributes(attribute.String("mcp.tool", tool.name), attribute.String("mcp.toolset", tool.set))
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(m.cfg.Limits.TimeoutSeconds)*time.Second)
	defer cancel()
	ctx = aol.ContextWithTool(aol.WithCallBudget(ctx, m.cfg.Limits.MaxInternalCalls), tool.name)
	select {
	case m.semaphore <- struct{}{}:
		defer func() { <-m.semaphore }()
	default:
		return toolResult(nil, aol.Failure("MCP-LIMIT-CONCURRENCY", "concurrent tool limit exceeded", "Retry when another tool call finishes."), m.cfg.Limits.MaxResultBytes)
	}
	var result any
	var err error
	input := arguments{}
	if err = decodeArguments(args, tool, input); err == nil {
		result, err = m.call(ctx, tool.name, input)
	}
	if id, ok := input["plan_id"].(string); ok {
		span.SetAttributes(attribute.String("aol.plan_id", id))
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
		span.SetStatus(codes.Error, "tool failed")
	}
	span.SetAttributes(attribute.String("aol.outcome", outcome))
	attrs := metric.WithAttributes(attribute.String("mcp.tool", tool.name), attribute.String("aol.outcome", outcome))
	m.calls.Add(ctx, 1, attrs)
	m.latency.Record(ctx, time.Since(start).Seconds(), attrs)
	response, responseErr := toolResult(result, err, m.cfg.Limits.MaxResultBytes)
	if response != nil {
		raw, _ := json.Marshal(response)
		m.bytes.Record(ctx, int64(len(raw)), attrs)
	}
	return response, responseErr
}
func toolResult(value any, err error, limit int) (*sdk.CallToolResult, error) {
	isError := err != nil
	if isError {
		var failure *aol.Error
		if !errors.As(err, &failure) {
			failure = aol.Failure("MCP-TOOL-FAILED", "operation failed", "Inspect server logs and the changeset before retrying.")
		}
		value = map[string]any{"error": failure, "result": value}
	}
	response, encodeErr := encodeResult(value, isError)
	if encodeErr != nil {
		return nil, encodeErr
	}
	raw, _ := json.Marshal(response)
	if len(raw) > limit {
		return encodeResult(map[string]any{"error": aol.Failure("MCP-LIMIT-RESULTBYTES", "result exceeds response budget", "Request a smaller page or one element.")}, true)
	}
	return response, nil
}
func encodeResult(value any, isError bool) (*sdk.CallToolResult, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("MCP-RESULT-ENCODE: %w", err)
	}
	return &sdk.CallToolResult{IsError: isError, StructuredContent: value, Content: []sdk.Content{&sdk.TextContent{Text: string(raw)}}}, nil
}
func requestDepth(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return aol.Failure("MCP-REQUEST-JSON", "invalid JSON request", "Correct the JSON request.")
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
			if depth > 64 {
				return aol.Failure("MCP-LIMIT-DEPTH", "request exceeds nesting limit", "Reduce nesting to at most 64 levels.")
			}
		}
	}
}

func (m *Module) initAuthentication(ctx context.Context, cfg *common.Config) error {
	if cfg.MCP.WriteMode == "confirm" && (!cfg.ABAC.Enabled || cfg.MCP.AllowAnonymous || !m.security.Enabled) {
		return aol.Failure("MCP-INIT-WRITESECURITY", "controlled writes require ABAC and authenticated callers", "Enable ABAC and disable anonymous MCP access.")
	}
	m.auth = func(next http.Handler) http.Handler { return next }
	if !cfg.MCP.AllowAnonymous && (cfg.MCP.PublicURL == "" || len(cfg.MCP.AuthorizationServers) == 0) {
		return aol.Failure("MCP-INIT-OAUTH", "secured MCP requires publicUrl and authorizationServers", "Configure OAuth protected-resource metadata.")
	}
	if cfg.ABAC.Enabled || !cfg.MCP.AllowAnonymous {
		oidc, err := auth.NewOIDCForConfig(ctx, cfg)
		if err != nil {
			return err
		}
		m.auth = oidc.Middleware
	}
	return nil
}
