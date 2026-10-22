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

// Package aol implements deterministic agent operations over the secured BaSyx API.
package aol

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
)

// Identity is server-owned request identity; bearer tokens are never persisted.
type Identity struct{ Subject, Issuer, Token, SessionID, Profile string }
type identityKey struct{}
type budgetKey struct{}

// ContextWithIdentity associates authenticated caller identity with an operation.
func ContextWithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// IdentityFromContext returns the authenticated identity for this request.
func IdentityFromContext(ctx context.Context) Identity {
	i, _ := ctx.Value(identityKey{}).(Identity)
	return i
}

// WithCallBudget bounds fan-out independently for each tool call.
func WithCallBudget(ctx context.Context, limit int) context.Context {
	b := new(atomic.Int64)
	b.Store(int64(limit))
	return context.WithValue(ctx, budgetKey{}, b)
}

// Error provides a stable error code and a client correction hint.
type Error struct {
	Code     string `json:"code"`
	Cause    string `json:"cause"`
	Hint     string `json:"hint"`
	Status   int    `json:"status,omitempty"`
	Location string `json:"location,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Cause }

// Failure creates a safe, structured operation error.
func Failure(code, cause, hint string) *Error { return &Error{Code: code, Cause: cause, Hint: hint} }

// EncodeID encodes a plain AAS identifier for REST dispatch.
func EncodeID(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }

// DecodeID decodes identifiers in dispatch tests and response adapters.
func DecodeID(id string) string { b, _ := base64.RawURLEncoding.DecodeString(id); return string(b) }

// Dispatcher executes only server-selected paths against the existing secured router.
type Dispatcher struct {
	Router   http.Handler
	MaxBytes int
	BasePath string
}

// Response retains ETags for conditional operations.
type Response struct {
	Status int
	Header http.Header
	Body   json.RawMessage
}
type responseBuffer struct {
	header        http.Header
	body          bytes.Buffer
	status, limit int
	overflow      bool
}

func (w *responseBuffer) Header() http.Header { return w.header }
func (w *responseBuffer) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *responseBuffer) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	n := len(p)
	remaining := w.limit - w.body.Len()
	if len(p) > remaining {
		w.overflow = true
		p = p[:remaining]
	}
	_, _ = w.body.Write(p)
	return n, nil
}

// Do preserves cancellation and identity, but resets chi's outer route state.
func (d Dispatcher) Do(ctx context.Context, method, path string, body any, headers http.Header) (Response, error) {
	if b, ok := ctx.Value(budgetKey{}).(*atomic.Int64); ok && b.Add(-1) < 0 {
		return Response{}, Failure("MCP-DISPATCH-CALLLIMIT", "internal call budget exceeded", "Use a narrower request or continue with the returned cursor.")
	}
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "://") {
		return Response{}, Failure("MCP-DISPATCH-PATH", "invalid internal path", "Use a supported tool.")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, Failure("MCP-DISPATCH-ENCODE", err.Error(), "Supply JSON-compatible arguments.")
	}
	ctx = context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(d.BasePath, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", IdentityFromContext(ctx).Token)
	for key, values := range headers {
		req.Header[key] = append([]string(nil), values...)
	}
	w := &responseBuffer{header: make(http.Header), limit: d.MaxBytes}
	d.Router.ServeHTTP(w, req)
	if w.overflow {
		return Response{}, Failure("MCP-DISPATCH-BYTELIMIT", "REST response exceeds the byte budget", "Reduce the page size or request one element.")
	}
	result := Response{Status: w.status, Header: w.header, Body: w.body.Bytes()}
	if w.status < 200 || w.status >= 300 {
		return result, &Error{Code: fmt.Sprintf("MCP-DISPATCH-HTTP%d", w.status), Cause: http.StatusText(w.status), Hint: "Check the identifier, request shape and your REST permissions.", Status: w.status}
	}
	return result, nil
}

type toolKey struct{}

// ContextWithTool carries server-selected tool attribution through nested API calls.
func ContextWithTool(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, toolKey{}, name)
}
func toolFromContext(ctx context.Context) string {
	name, _ := ctx.Value(toolKey{}).(string)
	return name
}

// BoundedHTTP limits complete protocol responses, including catalogues and envelopes.
func BoundedHTTP(next http.Handler, limit int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buffer := &responseBuffer{header: make(http.Header), limit: limit}
		next.ServeHTTP(buffer, r)
		if buffer.overflow {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_ = json.NewEncoder(w).Encode(Failure("MCP-LIMIT-RESULTBYTES", "protocol response exceeds byte budget", "Increase the response budget or request less data."))
			return
		}
		for key, values := range buffer.header {
			w.Header()[key] = values
		}
		status := buffer.status
		if status == 0 {
			status = 200
		}
		w.WriteHeader(status)
		_, _ = w.Write(buffer.body.Bytes())
	})
}
