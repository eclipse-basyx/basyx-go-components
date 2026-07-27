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
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type tracingRoundTripper struct {
	base       http.RoundTripper
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// HTTPClientTransport adds client spans and W3C propagation to base when
// tracing is enabled. Requests without an active span pass through unchanged.
func HTTPClientTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	runtime := activeRuntime.Load()
	if runtime == nil || !runtime.enabled {
		return base
	}
	return newHTTPClientTransport(
		base,
		runtime.provider.Tracer(instrumentationName),
		otel.GetTextMapPropagator(),
	)
}

func newHTTPClientTransport(
	base http.RoundTripper,
	tracer trace.Tracer,
	propagator propagation.TextMapPropagator,
) http.RoundTripper {
	return &tracingRoundTripper{
		base:       base,
		tracer:     tracer,
		propagator: propagator,
	}
}

func (transport *tracingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if !trace.SpanContextFromContext(request.Context()).IsValid() {
		return transport.base.RoundTrip(request)
	}

	ctx, span := transport.tracer.Start(
		request.Context(),
		request.Method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(clientRequestAttributes(request)...),
	)
	defer span.End()

	tracedRequest := request.Clone(ctx)
	tracedRequest.Header = request.Header.Clone()
	if tracedRequest.Header == nil {
		tracedRequest.Header = make(http.Header)
	}
	transport.propagator.Inject(ctx, propagation.HeaderCarrier(tracedRequest.Header))
	response, err := transport.base.RoundTrip(tracedRequest)
	if err != nil {
		span.AddEvent("request error")
		span.SetStatus(codes.Error, "HTTP client request failed")
		return response, err
	}
	if response != nil {
		span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
		if response.StatusCode >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, "HTTP server error")
		}
	}
	return response, nil
}

func clientRequestAttributes(request *http.Request) []attribute.KeyValue {
	port := request.URL.Port()
	if port == "" {
		switch request.URL.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	attributes := []attribute.KeyValue{
		attribute.String("http.request.method", request.Method),
		attribute.String("url.scheme", request.URL.Scheme),
		attribute.String("server.address", request.URL.Hostname()),
		attribute.String("url.path", request.URL.Path),
	}
	if parsedPort, err := strconv.Atoi(port); err == nil {
		attributes = append(attributes, attribute.Int("server.port", parsedPort))
	}
	return attributes
}
