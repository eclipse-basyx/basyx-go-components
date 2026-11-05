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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/go-chi/chi/v5"
)

const maxErrorDetailBytes = 512

// restCall describes one request against the AAS Environment API.
type restCall struct {
	method        string
	path          string
	query         url.Values
	body          any
	authorization string
	ifMatch       string
}

// restResult is a successful API response.
type restResult struct {
	status   int
	etag     string
	location string
	body     json.RawMessage
}

// restError reports a failed API call in a form the calling model can act on.
type restError struct {
	code    string
	status  int
	message string
	hint    string
}

func (e *restError) Error() string {
	text := e.code + ": " + e.message
	if e.hint != "" {
		text += ". " + e.hint
	}
	return text
}

// restDispatcher sends requests through the in-process router so the regular
// OIDC, ABAC, validation, history and eventing behavior applies unchanged.
type restDispatcher struct {
	router   http.Handler
	basePath string
	maxBytes int
}

func (d restDispatcher) do(ctx context.Context, call restCall) (restResult, error) {
	request, err := d.newRequest(ctx, call)
	if err != nil {
		return restResult{}, err
	}
	recorder := &boundedRecorder{header: make(http.Header), limit: d.maxBytes}
	d.router.ServeHTTP(recorder, request)
	if recorder.overflow {
		return restResult{}, &restError{
			code:    "MCP-DISPATCH-RESULTTOOLARGE",
			message: fmt.Sprintf("the response exceeds the configured limit of %d bytes", d.maxBytes),
			hint:    "Request a smaller page with limit and cursor, use valueOnly, or read a single submodel element",
		}
	}
	status := recorder.statusCode()
	if status < 200 || status >= 300 {
		return restResult{}, newRESTError(status, recorder.body.Bytes())
	}
	return restResult{
		status:   status,
		etag:     recorder.header.Get("ETag"),
		location: recorder.header.Get("Location"),
		body:     recorder.body.Bytes(),
	}, nil
}

func (d restDispatcher) newRequest(ctx context.Context, call restCall) (*http.Request, error) {
	var body io.Reader = http.NoBody
	if call.body != nil {
		raw, err := json.Marshal(call.body)
		if err != nil {
			return nil, fmt.Errorf("MCP-DISPATCH-ENCODEBODY: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	target := strings.TrimRight(d.basePath, "/") + call.path
	if len(call.query) > 0 {
		target += "?" + call.query.Encode()
	}
	// The MCP request already carries chi routing state for the MCP route.
	routedCtx := context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
	request, err := http.NewRequestWithContext(routedCtx, call.method, target, body)
	if err != nil {
		return nil, fmt.Errorf("MCP-DISPATCH-NEWREQUEST: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if call.body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if call.authorization != "" {
		request.Header.Set("Authorization", call.authorization)
	}
	if call.ifMatch != "" {
		request.Header.Set("If-Match", call.ifMatch)
	}
	return request, nil
}

func newRESTError(status int, body []byte) *restError {
	return &restError{
		code:    fmt.Sprintf("MCP-DISPATCH-HTTP%d", status),
		status:  status,
		message: restErrorMessage(status, body),
		hint:    restErrorHint(status),
	}
}

func restErrorMessage(status int, body []byte) string {
	message := http.StatusText(status)
	detail := strings.Join(restMessageTexts(body), "; ")
	if detail == "" {
		return message
	}
	if len(detail) > maxErrorDetailBytes {
		detail = detail[:maxErrorDetailBytes] + "..."
	}
	return message + ": " + detail
}

func restMessageTexts(body []byte) []string {
	var envelope struct {
		Messages []struct {
			Text string `json:"text"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil
	}
	texts := make([]string, 0, len(envelope.Messages))
	for _, message := range envelope.Messages {
		if text := strings.TrimSpace(message.Text); text != "" {
			texts = append(texts, text)
		}
	}
	return texts
}

func restErrorHint(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "Correct the request arguments or the AAS JSON payload"
	case http.StatusUnauthorized, http.StatusForbidden:
		return "The signed-in user is not permitted to perform this operation"
	case http.StatusNotFound:
		return "Check the identifier or idShortPath, or search with list_shells, list_submodels or a query tool"
	case http.StatusConflict:
		return "A resource with this identifier or idShort already exists"
	case http.StatusPreconditionFailed:
		return "The resource changed since it was read; read it again and retry with the new etag"
	case http.StatusPreconditionRequired:
		return "Read the resource first and pass its etag"
	default:
		return ""
	}
}

// boundedRecorder buffers a handler response up to a byte limit.
type boundedRecorder struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	limit    int
	overflow bool
}

func (r *boundedRecorder) Header() http.Header { return r.header }

func (r *boundedRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *boundedRecorder) Write(p []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	remaining := r.limit - r.body.Len()
	if len(p) > remaining {
		r.overflow = true
		_, _ = r.body.Write(p[:max(remaining, 0)])
		return len(p), nil
	}
	return r.body.Write(p)
}

func (r *boundedRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func shellPath(shellID string) string {
	return "/shells/" + common.EncodeString(shellID)
}

func submodelPath(submodelID string) string {
	return "/submodels/" + common.EncodeString(submodelID)
}

func elementPath(submodelID, idShortPath string) string {
	path := submodelPath(submodelID) + "/submodel-elements"
	if idShortPath != "" {
		path += "/" + url.PathEscape(idShortPath)
	}
	return path
}
