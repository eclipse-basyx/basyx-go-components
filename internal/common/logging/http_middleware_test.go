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

package logging

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHTTPMiddlewarePreservesOrGeneratesRequestMetadata(t *testing.T) {
	tests := []struct {
		name                string
		headers             map[string]string
		expectedRequestID   string
		expectedCorrelation string
		generated           bool
	}{
		{
			name: "canonical headers",
			headers: map[string]string{
				RequestIDHeader:     "request-1",
				CorrelationIDHeader: "correlation-1",
			},
			expectedRequestID:   "request-1",
			expectedCorrelation: "correlation-1",
		},
		{
			name: "legacy headers",
			headers: map[string]string{
				LegacyRequestIDHeader:     "request-2",
				LegacyCorrelationIDHeader: "correlation-2",
			},
			expectedRequestID:   "request-2",
			expectedCorrelation: "correlation-2",
		},
		{
			name: "maximum length headers",
			headers: map[string]string{
				RequestIDHeader:     strings.Repeat("r", maximumRequestIDLength),
				CorrelationIDHeader: strings.Repeat("c", maximumRequestIDLength),
			},
			expectedRequestID:   strings.Repeat("r", maximumRequestIDLength),
			expectedCorrelation: strings.Repeat("c", maximumRequestIDLength),
		},
		{
			name:      "missing headers",
			generated: true,
		},
		{
			name: "invalid headers",
			headers: map[string]string{
				RequestIDHeader:     "invalid request",
				CorrelationIDHeader: strings.Repeat("c", maximumRequestIDLength+1),
			},
			generated: true,
		},
		{
			name: "surrounding whitespace",
			headers: map[string]string{
				RequestIDHeader:     " request-3 ",
				CorrelationIDHeader: "\tcorrelation-3",
			},
			generated: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var captured requestMetadata
			handler := HTTPMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				captured = requestMetadata{
					requestID:     RequestIDFromContext(request.Context()),
					correlationID: CorrelationIDFromContext(request.Context()),
				}
			}))
			request := httptest.NewRequest(http.MethodGet, "/resource", nil)
			for name, value := range test.headers {
				request.Header.Set(name, value)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if test.generated {
				if !regexp.MustCompile(`^req-[0-9a-f]{32}$`).MatchString(captured.requestID) {
					t.Fatalf("unexpected generated request ID %q", captured.requestID)
				}
				if captured.correlationID != captured.requestID {
					t.Fatalf("generated correlation ID %q does not match request ID %q", captured.correlationID, captured.requestID)
				}
			} else if captured.requestID != test.expectedRequestID || captured.correlationID != test.expectedCorrelation {
				t.Fatalf("unexpected request metadata: %#v", captured)
			}
			if response.Header().Get(RequestIDHeader) != captured.requestID {
				t.Fatalf("unexpected response request ID %q", response.Header().Get(RequestIDHeader))
			}
			if response.Header().Get(CorrelationIDHeader) != captured.correlationID {
				t.Fatalf("unexpected response correlation ID %q", response.Header().Get(CorrelationIDHeader))
			}
			if request.Header.Get(RequestIDHeader) != captured.requestID {
				t.Fatalf("request header was not canonicalized: %#v", request.Header)
			}
			if request.Header.Get(CorrelationIDHeader) != captured.correlationID {
				t.Fatalf("correlation header was not canonicalized: %#v", request.Header)
			}
		})
	}
}

func TestNewRequestIDFallsBackWhenRandomnessFails(t *testing.T) {
	requestID := newRequestID(errorReader{})
	if !regexp.MustCompile(`^req-[0-9]+-[0-9]+$`).MatchString(requestID) {
		t.Fatalf("unexpected fallback request ID %q", requestID)
	}
}

func TestHTTPMiddlewareWritesOneStructuredAccessEvent(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})
	router := chi.NewRouter()
	router.Post("/items/{itemID}", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte("ok"))
	})
	handler := HTTPMiddleware(router)
	request := httptest.NewRequest(http.MethodPost, "/items/42?access_token=secret", strings.NewReader("private body"))
	request.Header.Set(RequestIDHeader, "request-1")
	request.Header.Set("Authorization", "Bearer private-token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	record := decodeSingleRecord(t, output)
	expected := map[string]any{
		"msg":                       accessLogMessage,
		"request.id":                "request-1",
		"correlation.id":            "request-1",
		"http.request.method":       http.MethodPost,
		"url.path":                  "/items/42",
		"http.route":                "/items/{itemID}",
		"http.response.status_code": float64(http.StatusCreated),
		"http.response.body.size":   float64(2),
	}
	for key, value := range expected {
		if record[key] != value {
			t.Errorf("unexpected %s: got %#v want %#v in %#v", key, record[key], value, record)
		}
	}
	if duration, ok := record["duration_ms"].(float64); !ok || duration < 0 {
		t.Errorf("unexpected duration_ms: %#v", record["duration_ms"])
	}
	serialized := output.String()
	for _, secret := range []string{"access_token", "secret", "private body", "Authorization", "private-token"} {
		if strings.Contains(serialized, secret) {
			t.Errorf("access event contains %q: %s", secret, serialized)
		}
	}
}

func TestHTTPMiddlewareRecordsImplicitAndMissingRoutes(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})
	router := chi.NewRouter()
	router.Get("/implicit", func(http.ResponseWriter, *http.Request) {})
	handler := HTTPMiddleware(router)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/implicit", nil))
	record := decodeSingleRecord(t, output)
	if record["http.response.status_code"] != float64(http.StatusOK) {
		t.Fatalf("unexpected implicit status: %#v", record)
	}

	output.Reset()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))
	record = decodeSingleRecord(t, output)
	if record["http.response.status_code"] != float64(http.StatusNotFound) {
		t.Fatalf("unexpected missing-route status: %#v", record)
	}
	if _, ok := record["http.route"]; ok {
		t.Fatalf("missing route contains http.route: %#v", record)
	}
}

func TestHTTPMiddlewareLogsHealthChecksAtDebug(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	handler := HTTPMiddleware(router)

	infoOutput := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	if infoOutput.Len() != 0 {
		t.Fatalf("health request logged at info: %q", infoOutput.String())
	}

	debugOutput := configureForTest(t, Config{Format: FormatJSON, Level: LevelDebug})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	record := decodeSingleRecord(t, debugOutput)
	if record["level"] != "DEBUG" || record["http.route"] != "/health" {
		t.Fatalf("unexpected health record: %#v", record)
	}
}

func TestHTTPMiddlewareLogsPanicsAndRepanics(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})
	handler := HTTPMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusAccepted)
		panic("sentinel panic")
	}))

	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/panic", nil))
	}()

	if recovered != "sentinel panic" {
		t.Fatalf("unexpected recovered panic: %#v", recovered)
	}
	record := decodeSingleRecord(t, output)
	if record["http.response.status_code"] != float64(http.StatusInternalServerError) {
		t.Fatalf("unexpected panic status: %#v", record)
	}
}

func TestHTTPMiddlewarePreservesResponseWriterInterfaces(t *testing.T) {
	writer := newInterfaceResponseWriter()
	handler := HTTPMiddleware(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if _, ok := response.(http.Flusher); !ok {
			t.Error("http.Flusher is not preserved")
		}
		if _, ok := response.(http.Hijacker); !ok {
			t.Error("http.Hijacker is not preserved")
		}
		if _, ok := response.(io.ReaderFrom); !ok {
			t.Error("io.ReaderFrom is not preserved")
		}
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.ProtoMajor = 1

	handler.ServeHTTP(writer, request)
}

func TestHTTPMiddlewarePreservesHTTP2ResponseWriterInterfaces(t *testing.T) {
	writer := newInterfaceResponseWriter()
	handler := HTTPMiddleware(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if _, ok := response.(http.Flusher); !ok {
			t.Error("http.Flusher is not preserved")
		}
		if _, ok := response.(http.Pusher); !ok {
			t.Error("http.Pusher is not preserved")
		}
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.ProtoMajor = 2

	handler.ServeHTTP(writer, request)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("random source unavailable")
}

type interfaceResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newInterfaceResponseWriter() *interfaceResponseWriter {
	return &interfaceResponseWriter{header: make(http.Header)}
}

func (writer *interfaceResponseWriter) Header() http.Header {
	return writer.header
}

func (writer *interfaceResponseWriter) WriteHeader(status int) {
	writer.status = status
}

func (writer *interfaceResponseWriter) Write(value []byte) (int, error) {
	return writer.body.Write(value)
}

func (writer *interfaceResponseWriter) Flush() {}

func (writer *interfaceResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("not implemented")
}

func (writer *interfaceResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	return writer.body.ReadFrom(reader)
}

func (writer *interfaceResponseWriter) Push(string, *http.PushOptions) error {
	return errors.New("not implemented")
}
