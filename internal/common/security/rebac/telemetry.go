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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"context"
	"log/slog"
	"time"

	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationScope = "github.com/eclipse-basyx/basyx-go-components/rebac"

// Decision outcomes reported by spans and metrics.
const (
	outcomeGranted     = "granted"
	outcomeNone        = "none"
	outcomeUncovered   = "uncovered"
	outcomeUnavailable = "unavailable"
)

var tracer = otel.Tracer(instrumentationScope)

// instruments are the ReBAC metrics of one coordinator.
type instruments struct {
	decisions metric.Int64Counter
	duration  metric.Float64Histogram
	changes   metric.Int64Counter
	denials   metric.Int64Counter
}

// newInstruments creates the ReBAC metrics. Metrics never block
// authorization, so a failing meter falls back to no-op instruments.
func newInstruments() instruments {
	meter := otel.Meter(instrumentationScope)
	decisions, decisionsErr := meter.Int64Counter("basyx.rebac.decisions",
		metric.WithDescription("ReBAC decisions of requests ABAC does not allow unconditionally"))
	duration, durationErr := meter.Float64Histogram("basyx.rebac.decision.duration",
		metric.WithUnit("s"), metric.WithDescription("Duration of ReBAC decisions"))
	changes, changesErr := meter.Int64Counter("basyx.rebac.access.changes",
		metric.WithDescription("Audited access changes by event type"))
	denials, denialsErr := meter.Int64Counter("basyx.rebac.management.denials",
		metric.WithDescription("Management requests denied for missing can_manage or administrator status"))
	if err := firstError(decisionsErr, durationErr, changesErr, denialsErr); err != nil {
		slog.Warn("ReBAC metrics unavailable", "error.code", "REBAC-TELEMETRY-INSTRUMENTS", "error", err)
		fallback := noop.NewMeterProvider().Meter(instrumentationScope)
		decisions, _ = fallback.Int64Counter("basyx.rebac.decisions")
		duration, _ = fallback.Float64Histogram("basyx.rebac.decision.duration")
		changes, _ = fallback.Int64Counter("basyx.rebac.access.changes")
		denials, _ = fallback.Int64Counter("basyx.rebac.management.denials")
	}
	return instruments{decisions: decisions, duration: duration, changes: changes, denials: denials}
}

// traceDecision wraps one decision in a span and records its metrics.
func (c *Coordinator) traceDecision(ctx context.Context, route auth.ReBACRoute, decide func(context.Context) (*auth.ReBACGrantSet, string, error)) (*auth.ReBACGrantSet, error) {
	ctx, span := tracer.Start(ctx, "rebac.decision", trace.WithAttributes(
		attribute.String("http.request.method", route.Method), attribute.String("http.route", route.Pattern)))
	defer span.End()
	start := time.Now()
	grants, outcome, err := decide(ctx)
	if err != nil {
		outcome = outcomeUnavailable
		span.RecordError(err)
		span.SetStatus(codes.Error, "ReBAC decision unavailable")
	}
	outcomeAttribute := attribute.String("rebac.outcome", outcome)
	span.SetAttributes(outcomeAttribute)
	options := metric.WithAttributes(outcomeAttribute, attribute.String("http.route", route.Pattern))
	c.metrics.decisions.Add(ctx, 1, options)
	c.metrics.duration.Record(ctx, time.Since(start).Seconds(), options)
	return grants, err
}

func (c *Coordinator) countChange(ctx context.Context, eventType string) {
	c.metrics.changes.Add(ctx, 1, metric.WithAttributes(attribute.String("rebac.event", eventType)))
}

func (c *Coordinator) countManagementDenial(ctx context.Context, route string) {
	c.metrics.denials.Add(ctx, 1, metric.WithAttributes(attribute.String("http.route", route)))
}
