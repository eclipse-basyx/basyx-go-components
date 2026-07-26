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
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	commonlogging "github.com/eclipse-basyx/basyx-go-components/internal/common/logging"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const testTraceParent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestHTTPMiddlewareReturnsHandlerDirectlyWhenDisabled(t *testing.T) {
	activeRuntime.Store(nil)
	next := &testHandler{}

	wrapped := HTTPMiddleware(next)

	if wrapped != next {
		t.Fatal("disabled tracing wrapped the handler")
	}
}

func TestHTTPMiddlewareRecordsServerSpanWithInboundContext(t *testing.T) {
	router := chi.NewRouter()
	router.Post("/items/{itemID}", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte("ok"))
	})
	handler, recorder := tracingTestHandler(commonlogging.HTTPMiddleware(router))
	request := httptest.NewRequest(http.MethodPost, "/items/42?access_token=private", strings.NewReader("private body"))
	request.Header.Set("traceparent", testTraceParent)
	request.Header.Set(commonlogging.RequestIDHeader, "request-1")
	request.Header.Set(commonlogging.CorrelationIDHeader, "correlation-1")
	request.Header.Set("Authorization", "Bearer private-token")

	handler.ServeHTTP(httptest.NewRecorder(), request)

	span := singleEndedSpan(t, recorder)
	if span.Name() != "POST /items/{itemID}" {
		t.Fatalf("unexpected span name %q", span.Name())
	}
	if span.SpanKind() != trace.SpanKindServer {
		t.Fatalf("unexpected span kind %v", span.SpanKind())
	}
	if span.SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("inbound trace context was not preserved: %s", span.SpanContext().TraceID())
	}
	attributes := spanAttributes(span)
	expected := map[string]any{
		"http.request.method":       http.MethodPost,
		"url.path":                  "/items/42",
		"http.route":                "/items/{itemID}",
		"http.response.status_code": int64(http.StatusCreated),
		"http.response.body.size":   int64(2),
		"request.id":                "request-1",
		"correlation.id":            "correlation-1",
	}
	for key, value := range expected {
		if attributes[key] != value {
			t.Errorf("unexpected %s: got %#v want %#v", key, attributes[key], value)
		}
	}
	serialized := span.Name()
	for key, value := range attributes {
		serialized += key + "=" + attributeValueString(value)
	}
	for _, secret := range []string{"access_token", "private", "Authorization", "private-token", "User-Agent"} {
		if strings.Contains(serialized, secret) {
			t.Errorf("span contains %q: %s", secret, serialized)
		}
	}
}

func TestHTTPMiddlewareCorrelatesAccessLogWithServerSpan(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})
	if _, err := commonlogging.Configure(
		commonlogging.Config{Format: commonlogging.FormatJSON, Level: commonlogging.LevelInfo},
		"testservice",
		&output,
	); err != nil {
		t.Fatalf("configure logging: %v", err)
	}

	router := chi.NewRouter()
	router.Get("/items/{itemID}", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	handler, recorder := tracingTestHandler(commonlogging.HTTPMiddleware(router))
	request := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	request.Header.Set(commonlogging.RequestIDHeader, "request-1")

	handler.ServeHTTP(httptest.NewRecorder(), request)

	span := singleEndedSpan(t, recorder)
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("decode access log: %v", err)
	}
	if record["trace_id"] != span.SpanContext().TraceID().String() {
		t.Fatalf("access log trace ID does not match span: %#v", record)
	}
	if record["span_id"] != span.SpanContext().SpanID().String() {
		t.Fatalf("access log span ID does not match span: %#v", record)
	}
	if record["trace_flags"] != "01" {
		t.Fatalf("unexpected access log trace flags: %#v", record)
	}
}

func TestHTTPMiddlewareCreatesTraceAndRecordsMissingRoute(t *testing.T) {
	router := chi.NewRouter()
	handler, recorder := tracingTestHandler(commonlogging.HTTPMiddleware(router))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))

	span := singleEndedSpan(t, recorder)
	if !span.SpanContext().IsValid() || span.Parent().IsValid() {
		t.Fatalf("unexpected generated span context: span=%v parent=%v", span.SpanContext(), span.Parent())
	}
	if span.Name() != http.MethodGet {
		t.Fatalf("unexpected unmatched span name %q", span.Name())
	}
	attributes := spanAttributes(span)
	if _, ok := attributes["http.route"]; ok {
		t.Fatalf("unmatched request contains route: %#v", attributes)
	}
	if attributes["http.response.status_code"] != int64(http.StatusNotFound) {
		t.Fatalf("unexpected status attributes: %#v", attributes)
	}
}

func TestHTTPMiddlewareRecordsImplicitResponseAndPreservesFlushing(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/stream", func(writer http.ResponseWriter, _ *http.Request) {
		if _, ok := writer.(http.Flusher); !ok {
			t.Fatal("wrapped response writer lost http.Flusher")
		}
		_, _ = writer.Write([]byte("stream"))
		writer.(http.Flusher).Flush()
	})
	handler, recorder := tracingTestHandler(commonlogging.HTTPMiddleware(router))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/stream", nil))

	attributes := spanAttributes(singleEndedSpan(t, recorder))
	if attributes["http.response.status_code"] != int64(http.StatusOK) ||
		attributes["http.response.body.size"] != int64(len("stream")) {
		t.Fatalf("unexpected response attributes: %#v", attributes)
	}
}

func TestHTTPMiddlewareMarksServerErrorsAndPanics(t *testing.T) {
	for _, test := range []struct {
		name         string
		handler      http.Handler
		expectPanic  bool
		expectedCode int
	}{
		{
			name: "server error",
			handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusServiceUnavailable)
			}),
			expectedCode: http.StatusServiceUnavailable,
		},
		{
			name: "panic",
			handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic("private panic value")
			}),
			expectPanic:  true,
			expectedCode: http.StatusInternalServerError,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, recorder := tracingTestHandler(commonlogging.HTTPMiddleware(test.handler))
			if test.expectPanic {
				defer func() {
					if recovered := recover(); recovered == nil {
						t.Fatal("expected panic")
					}
					assertErrorSpan(t, singleEndedSpan(t, recorder), test.expectedCode, true)
				}()
			}

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/failure", nil))
			if !test.expectPanic {
				assertErrorSpan(t, singleEndedSpan(t, recorder), test.expectedCode, false)
			}
		})
	}
}

func tracingTestHandler(next http.Handler) (http.Handler, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	return newHTTPMiddleware(next, provider, propagation.TraceContext{}), recorder
}

func singleEndedSpan(t *testing.T, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected one ended span, got %d", len(spans))
	}
	return spans[0]
}

func spanAttributes(span sdktrace.ReadOnlySpan) map[string]any {
	attributes := make(map[string]any, len(span.Attributes()))
	for _, item := range span.Attributes() {
		attributes[string(item.Key)] = item.Value.AsInterface()
	}
	return attributes
}

func attributeValueString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func assertErrorSpan(t *testing.T, span sdktrace.ReadOnlySpan, expectedStatus int, expectPanic bool) {
	t.Helper()
	if span.Status().Code != codes.Error {
		t.Fatalf("span is not marked as an error: %#v", span.Status())
	}
	if spanAttributes(span)["http.response.status_code"] != int64(expectedStatus) {
		t.Fatalf("unexpected span status attributes: %#v", spanAttributes(span))
	}
	hasPanicEvent := false
	for _, event := range span.Events() {
		if event.Name == "panic" {
			hasPanicEvent = true
		}
		for _, item := range event.Attributes {
			if strings.Contains(item.Value.String(), "private panic value") {
				t.Fatalf("panic event leaked the panic value: %#v", event)
			}
		}
	}
	if hasPanicEvent != expectPanic {
		t.Fatalf("unexpected panic event state %t", hasPanicEvent)
	}
}

type testHandler struct{}

func (*testHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}
