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

package eventoutbox

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Worker runs a bounded pool of post-commit delivery workers for one sink.
type Worker struct {
	repository  *Repository
	sink, owner string
	publisher   Publisher
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	metrics     instruments
}

type instruments struct {
	successes, failures         metric.Int64Counter
	pending, retries, connected metric.Int64Gauge
	oldest                      metric.Float64Gauge
	options                     metric.MeasurementOption
}

func newInstruments(sink string) (instruments, error) {
	meter := otel.Meter("github.com/eclipse-basyx/basyx-go-components/eventing")
	var i instruments
	var err error
	i.options = metric.WithAttributes(attribute.String("sink", sink))
	counters := []struct {
		name   string
		target *metric.Int64Counter
	}{{"basyx.eventing.delivered", &i.successes}, {"basyx.eventing.delivery.failures", &i.failures}}
	for _, c := range counters {
		*c.target, err = meter.Int64Counter(c.name)
		if err != nil {
			return i, fmt.Errorf("OUTBOX-METRICS-COUNTER: %w", err)
		}
	}
	gauges := []struct {
		name   string
		target *metric.Int64Gauge
	}{{"basyx.eventing.pending", &i.pending}, {"basyx.eventing.retry.pending", &i.retries}, {"basyx.eventing.broker.connected", &i.connected}}
	for _, g := range gauges {
		*g.target, err = meter.Int64Gauge(g.name)
		if err != nil {
			return i, fmt.Errorf("OUTBOX-METRICS-GAUGE: %w", err)
		}
	}
	i.oldest, err = meter.Float64Gauge("basyx.eventing.pending.oldest.age", metric.WithUnit("s"))
	if err != nil {
		return i, fmt.Errorf("OUTBOX-METRICS-AGE: %w", err)
	}
	return i, nil
}

// Start creates four delivery workers and one queue observer.
func Start(ctx context.Context, repository *Repository, sink, owner string, publisher Publisher) (*Worker, error) {
	instruments, err := newInstruments(sink)
	if err != nil {
		return nil, err
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w := &Worker{repository: repository, sink: sink, owner: owner, publisher: publisher, cancel: cancel, metrics: instruments}
	for n := range 4 {
		w.wg.Go(func() { w.run(workerCtx, fmt.Sprintf("%s/%d", owner, n)) })
	}
	w.wg.Go(func() { w.observe(workerCtx) })
	return w, nil
}

func (w *Worker) run(ctx context.Context, owner string) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		found, err := w.repository.DeliverOne(ctx, w.sink, owner, w.publisher)
		w.recordResult(ctx, found, err)
		if found && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w *Worker) recordResult(ctx context.Context, found bool, err error) {
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		w.metrics.failures.Add(ctx, 1, w.metrics.options)
		slog.WarnContext(ctx, "event delivery deferred", "error.code", "OUTBOX-WORKER-RETRY", "sink", w.sink, "error", err)
	} else if found {
		w.metrics.successes.Add(ctx, 1, w.metrics.options)
	}
}

func (w *Worker) observe(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		w.recordStats(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w *Worker) recordStats(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	count, oldest, retries, err := w.repository.Stats(queryCtx, w.sink)
	if err != nil {
		slog.WarnContext(ctx, "event queue statistics unavailable", "error.code", "OUTBOX-WORKER-STATS", "sink", w.sink)
		return
	}
	age := float64(0)
	if !oldest.IsZero() {
		age = max(0, time.Since(oldest).Seconds())
	}
	w.metrics.pending.Record(ctx, count, w.metrics.options)
	w.metrics.retries.Record(ctx, retries, w.metrics.options)
	w.metrics.oldest.Record(ctx, age, w.metrics.options)
	if status, ok := w.publisher.(interface{ Connected() bool }); ok {
		connected := int64(0)
		if status.Connected() {
			connected = 1
		}
		w.metrics.connected.Record(ctx, connected, w.metrics.options)
	}
}

// Stop cancels in-flight delivery and waits before the database pool is closed.
func (w *Worker) Stop() { w.cancel(); w.wg.Wait() }
