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
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	// RequestIDHeader is the canonical HTTP request identifier header.
	RequestIDHeader = "X-Request-ID"
	// CorrelationIDHeader is the canonical HTTP correlation identifier header.
	CorrelationIDHeader = "X-Correlation-ID"
	// LegacyRequestIDHeader is accepted for compatibility with existing clients.
	LegacyRequestIDHeader = "Request-ID"
	// LegacyCorrelationIDHeader is accepted for compatibility with existing clients.
	LegacyCorrelationIDHeader = "Correlation-ID"

	maximumRequestIDLength = 128
	accessLogMessage       = "HTTP request completed"
)

var fallbackRequestIDCounter atomic.Uint64

type httpLoggingMiddleware struct {
	next          http.Handler
	routeContexts sync.Pool
}

// HTTPMiddleware assigns request metadata and emits one structured access event
// after the wrapped handler completes.
func HTTPMiddleware(next http.Handler) http.Handler {
	middleware := &httpLoggingMiddleware{next: next}
	middleware.routeContexts.New = func() any {
		return chi.NewRouteContext()
	}
	return middleware
}

func (loggingMiddleware *httpLoggingMiddleware) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	metadata := requestMetadataFromHeaders(request)
	request.Header.Set(RequestIDHeader, metadata.requestID)
	request.Header.Set(CorrelationIDHeader, metadata.correlationID)
	writer.Header().Set(RequestIDHeader, metadata.requestID)
	writer.Header().Set(CorrelationIDHeader, metadata.correlationID)

	ctx, routeContext := requestContext(request.Context(), loggingMiddleware.next, metadata, &loggingMiddleware.routeContexts)
	request = request.WithContext(ctx)
	response := middleware.NewWrapResponseWriter(writer, request.ProtoMajor)
	started := time.Now()

	if routeContext != nil {
		defer releaseRouteContext(&loggingMiddleware.routeContexts, routeContext)
	}
	defer func() {
		recovered := recover()
		status := response.Status()
		if recovered != nil {
			status = http.StatusInternalServerError
		} else if status == 0 {
			status = http.StatusOK
		}
		logHTTPRequest(request, response, status, time.Since(started))
		if recovered != nil {
			panic(recovered)
		}
	}()

	loggingMiddleware.next.ServeHTTP(response, request)
}

func (loggingMiddleware *httpLoggingMiddleware) Routes() []chi.Route {
	if routes, ok := loggingMiddleware.next.(chi.Routes); ok {
		return routes.Routes()
	}
	return nil
}

func (loggingMiddleware *httpLoggingMiddleware) Middlewares() chi.Middlewares {
	if routes, ok := loggingMiddleware.next.(chi.Routes); ok {
		return routes.Middlewares()
	}
	return nil
}

func (loggingMiddleware *httpLoggingMiddleware) Match(routeContext *chi.Context, method string, path string) bool {
	if routes, ok := loggingMiddleware.next.(chi.Routes); ok {
		return routes.Match(routeContext, method, path)
	}
	return false
}

func (loggingMiddleware *httpLoggingMiddleware) Find(routeContext *chi.Context, method string, path string) string {
	if routes, ok := loggingMiddleware.next.(chi.Routes); ok {
		return routes.Find(routeContext, method, path)
	}
	return ""
}

func requestMetadataFromHeaders(request *http.Request) requestMetadata {
	requestID := normalizedRequestID(firstRequestHeader(request, RequestIDHeader, LegacyRequestIDHeader))
	if requestID == "" {
		requestID = newRequestID(rand.Reader)
	}
	correlationID := normalizedRequestID(firstRequestHeader(request, CorrelationIDHeader, LegacyCorrelationIDHeader))
	if correlationID == "" {
		correlationID = requestID
	}
	return requestMetadata{requestID: requestID, correlationID: correlationID}
}

func requestContext(
	ctx context.Context,
	next http.Handler,
	metadata requestMetadata,
	routeContexts *sync.Pool,
) (context.Context, *chi.Context) {
	ctx = contextWithRequestMetadata(ctx, metadata)
	if chi.RouteContext(ctx) != nil {
		return ctx, nil
	}
	routes, ok := next.(chi.Routes)
	if !ok {
		return ctx, nil
	}
	routeContext := routeContexts.Get().(*chi.Context)
	routeContext.Reset()
	routeContext.Routes = routes
	return context.WithValue(ctx, chi.RouteCtxKey, routeContext), routeContext
}

func releaseRouteContext(routeContexts *sync.Pool, routeContext *chi.Context) {
	routeContext.Reset()
	routeContexts.Put(routeContext)
}

func firstRequestHeader(request *http.Request, names ...string) string {
	for _, name := range names {
		if value := request.Header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func normalizedRequestID(value string) string {
	if len(value) == 0 || len(value) > maximumRequestIDLength {
		return ""
	}
	for index := 0; index < len(value); index++ {
		if !validRequestIDCharacter(value[index]) {
			return ""
		}
	}
	return value
}

func validRequestIDCharacter(character byte) bool {
	isLetter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
	isDigit := character >= '0' && character <= '9'
	switch character {
	case '-', '.', '_', ':', '/':
		return true
	default:
		return isLetter || isDigit
	}
}

func newRequestID(random io.Reader) string {
	randomBytes := make([]byte, 16)
	if _, err := io.ReadFull(random, randomBytes); err == nil {
		return "req-" + hex.EncodeToString(randomBytes)
	}
	return fmt.Sprintf("req-%d-%d", time.Now().UTC().UnixNano(), fallbackRequestIDCounter.Add(1))
}

func logHTTPRequest(request *http.Request, response middleware.WrapResponseWriter, status int, duration time.Duration) {
	route := chi.RouteContext(request.Context()).RoutePattern()
	attributes := []any{
		"http.request.method", request.Method,
		"url.path", request.URL.Path,
		"http.response.status_code", status,
		"http.response.body.size", response.BytesWritten(),
		"duration_ms", float64(duration.Microseconds()) / 1000,
	}
	if route != "" {
		attributes = append(attributes, "http.route", route)
	}
	level := slog.LevelInfo
	if request.Method == http.MethodGet && route != "" && strings.HasSuffix(route, "/health") {
		level = slog.LevelDebug
	}
	slog.Log(request.Context(), level, accessLogMessage, attributes...)
}
