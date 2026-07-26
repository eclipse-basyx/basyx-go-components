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
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type handlerOperation struct {
	attributes []slog.Attr
	group      string
}

type contextHandler struct {
	base       slog.Handler
	operations []handlerOperation
}

func newContextHandler(base slog.Handler) slog.Handler {
	return &contextHandler{base: base}
}

func (handler *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.base.Enabled(ctx, level)
}

func (handler *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	target := handler.base
	if attributes := contextAttributes(ctx); len(attributes) > 0 {
		target = target.WithAttrs(attributes)
	}
	for _, operation := range handler.operations {
		if operation.group != "" {
			target = target.WithGroup(operation.group)
			continue
		}
		target = target.WithAttrs(operation.attributes)
	}
	return target.Handle(ctx, record)
}

func (handler *contextHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	if len(attributes) == 0 {
		return handler
	}
	operations := append([]handlerOperation(nil), handler.operations...)
	operations = append(operations, handlerOperation{attributes: append([]slog.Attr(nil), attributes...)})
	return &contextHandler{base: handler.base, operations: operations}
}

func (handler *contextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return handler
	}
	operations := append([]handlerOperation(nil), handler.operations...)
	operations = append(operations, handlerOperation{group: name})
	return &contextHandler{base: handler.base, operations: operations}
}

func contextAttributes(ctx context.Context) []slog.Attr {
	requestID := RequestIDFromContext(ctx)
	correlationID := CorrelationIDFromContext(ctx)
	spanContext := trace.SpanContext{}
	if ctx != nil {
		spanContext = trace.SpanContextFromContext(ctx)
	}
	if requestID == "" && correlationID == "" && !spanContext.IsValid() {
		return nil
	}
	attributes := make([]slog.Attr, 0, 5)
	if requestID != "" {
		attributes = append(attributes, slog.String("request.id", requestID))
	}
	if correlationID != "" {
		attributes = append(attributes, slog.String("correlation.id", correlationID))
	}
	if spanContext.IsValid() {
		attributes = append(
			attributes,
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
			slog.String("trace_flags", spanContext.TraceFlags().String()),
		)
	}
	return attributes
}
