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

package telemetry

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestHTTPClientTransportCreatesParentedSpanAndInjectsContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	propagator := propagation.TraceContext{}
	var capturedRequest *http.Request
	base := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		capturedRequest = request
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    request,
		}, nil
	})
	transport := newHTTPClientTransport(base, provider.Tracer(instrumentationName), propagator)
	ctx, parent := provider.Tracer("test").Start(t.Context(), "parent")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://user:password@example.com:8443/items/42?token=private", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = response.Body.Close()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected one ended client span, got %d", len(spans))
	}
	span := spans[0]
	if span.SpanKind() != trace.SpanKindClient || span.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatalf("unexpected client span relationship: kind=%v parent=%v", span.SpanKind(), span.Parent())
	}
	if capturedRequest.Header.Get("traceparent") == "" {
		t.Fatal("trace context was not injected")
	}
	attributes := spanAttributes(span)
	expected := map[string]any{
		"http.request.method":       http.MethodPost,
		"url.scheme":                "https",
		"server.address":            "example.com",
		"server.port":               int64(8443),
		"url.path":                  "/items/42",
		"http.response.status_code": int64(http.StatusCreated),
	}
	for key, value := range expected {
		if attributes[key] != value {
			t.Errorf("unexpected %s: got %#v want %#v", key, attributes[key], value)
		}
	}
	serialized := attributesToString(attributes)
	for _, secret := range []string{"token", "private", "user", "password"} {
		if strings.Contains(serialized, secret) {
			t.Errorf("client span contains %q: %s", secret, serialized)
		}
	}
	if request.Header.Get("traceparent") != "" {
		t.Fatal("original request headers were mutated")
	}
	parent.End()
}

func TestHTTPClientTransportDoesNotFabricateTraceContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	var capturedRequest *http.Request
	base := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		capturedRequest = request
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	transport := newHTTPClientTransport(base, provider.Tracer(instrumentationName), propagation.TraceContext{})
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/items", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = response.Body.Close()

	if capturedRequest != request {
		t.Fatal("request without active span was unnecessarily cloned")
	}
	if capturedRequest.Header.Get("traceparent") != "" {
		t.Fatal("request without active span received trace context")
	}
	if len(recorder.Ended()) != 0 {
		t.Fatalf("request without active span produced %d spans", len(recorder.Ended()))
	}
}

func attributesToString(attributes map[string]any) string {
	var serialized strings.Builder
	for key, value := range attributes {
		serialized.WriteString(key)
		if text, ok := value.(string); ok {
			serialized.WriteString(text)
		}
	}
	return serialized.String()
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}
