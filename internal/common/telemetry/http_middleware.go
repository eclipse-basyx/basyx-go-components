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
	"context"
	"net/http"
	"sync"

	commonlogging "github.com/eclipse-basyx/basyx-go-components/internal/common/logging"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"

type httpTracingMiddleware struct {
	next          http.Handler
	tracer        trace.Tracer
	propagator    propagation.TextMapPropagator
	routeContexts sync.Pool
}

// HTTPMiddleware traces requests when Configure installed an active runtime.
// It returns next directly when tracing is disabled.
func HTTPMiddleware(next http.Handler) http.Handler {
	runtime := activeRuntime.Load()
	if runtime == nil || !runtime.enabled {
		return next
	}
	return newHTTPMiddleware(next, runtime.provider, otel.GetTextMapPropagator())
}

func newHTTPMiddleware(
	next http.Handler,
	provider trace.TracerProvider,
	propagator propagation.TextMapPropagator,
) http.Handler {
	middleware := &httpTracingMiddleware{
		next:       next,
		tracer:     provider.Tracer(instrumentationName),
		propagator: propagator,
	}
	middleware.routeContexts.New = func() any {
		return chi.NewRouteContext()
	}
	return middleware
}

func (middleware *httpTracingMiddleware) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	ctx := middleware.propagator.Extract(request.Context(), propagation.HeaderCarrier(request.Header))
	ctx, span := middleware.tracer.Start(
		ctx,
		request.Method,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.request.method", request.Method),
			attribute.String("url.path", request.URL.Path),
		),
	)
	request = request.WithContext(ctx)
	routeContext := middleware.attachRouteContext(request)
	if routeContext != nil {
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
		defer releaseTracingRouteContext(&middleware.routeContexts, routeContext)
	}

	response := chimiddleware.NewWrapResponseWriter(writer, request.ProtoMajor)
	defer func() {
		middleware.finishRequest(span, request, response, recover())
	}()
	middleware.next.ServeHTTP(response, request)
}

func (middleware *httpTracingMiddleware) finishRequest(
	span trace.Span,
	request *http.Request,
	response chimiddleware.WrapResponseWriter,
	recovered any,
) {
	status := response.Status()
	if recovered != nil {
		status = http.StatusInternalServerError
	} else if status == 0 {
		status = http.StatusOK
	}

	route := chi.RouteContext(request.Context()).RoutePattern()
	if route != "" {
		span.SetName(request.Method + " " + route)
		span.SetAttributes(attribute.String("http.route", route))
	}
	span.SetAttributes(
		attribute.Int("http.response.status_code", status),
		attribute.Int64("http.response.body.size", int64(response.BytesWritten())),
	)
	if requestID := response.Header().Get(commonlogging.RequestIDHeader); requestID != "" {
		span.SetAttributes(attribute.String("request.id", requestID))
	}
	if correlationID := response.Header().Get(commonlogging.CorrelationIDHeader); correlationID != "" {
		span.SetAttributes(attribute.String("correlation.id", correlationID))
	}
	if status >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, "HTTP server error")
	}
	if recovered != nil {
		span.AddEvent("panic")
		span.SetStatus(codes.Error, "HTTP handler panic")
	}
	span.End()

	if recovered != nil {
		panic(recovered)
	}
}

func (middleware *httpTracingMiddleware) attachRouteContext(request *http.Request) *chi.Context {
	if chi.RouteContext(request.Context()) != nil {
		return nil
	}
	routes, ok := middleware.next.(chi.Routes)
	if !ok {
		return nil
	}
	routeContext := middleware.routeContexts.Get().(*chi.Context)
	routeContext.Reset()
	routeContext.Routes = routes
	return routeContext
}

func releaseTracingRouteContext(routeContexts *sync.Pool, routeContext *chi.Context) {
	routeContext.Reset()
	routeContexts.Put(routeContext)
}
