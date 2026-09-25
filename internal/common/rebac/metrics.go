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

package rebac

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type clientMetrics struct {
	requests  metric.Int64Counter
	errors    metric.Int64Counter
	latency   metric.Float64Histogram
	batchSize metric.Int64Histogram
}

func newClientMetrics() (clientMetrics, error) {
	meter := otel.Meter("github.com/eclipse-basyx/basyx-go-components/rebac")
	requests, err := meter.Int64Counter("basyx.rebac.client.requests")
	if err != nil {
		return clientMetrics{}, fmt.Errorf("REBAC-METRICS-REQUESTS: %w", err)
	}
	errors, err := meter.Int64Counter("basyx.rebac.client.errors")
	if err != nil {
		return clientMetrics{}, fmt.Errorf("REBAC-METRICS-ERRORS: %w", err)
	}
	latency, err := meter.Float64Histogram("basyx.rebac.client.latency", metric.WithUnit("s"))
	if err != nil {
		return clientMetrics{}, fmt.Errorf("REBAC-METRICS-LATENCY: %w", err)
	}
	batchSize, err := meter.Int64Histogram("basyx.rebac.client.batch_size")
	if err != nil {
		return clientMetrics{}, fmt.Errorf("REBAC-METRICS-BATCHSIZE: %w", err)
	}
	return clientMetrics{requests: requests, errors: errors, latency: latency, batchSize: batchSize}, nil
}

func (metrics clientMetrics) measure(ctx context.Context, operation string, batchSize int) func(error) {
	started := time.Now()
	return func(err error) {
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		options := metric.WithAttributes(attribute.String("operation", operation), attribute.String("outcome", outcome))
		metrics.requests.Add(ctx, 1, options)
		metrics.latency.Record(ctx, time.Since(started).Seconds(), options)
		if err != nil {
			metrics.errors.Add(ctx, 1, options)
		}
		if batchSize >= 0 {
			metrics.batchSize.Record(ctx, int64(batchSize), options)
		}
	}
}
