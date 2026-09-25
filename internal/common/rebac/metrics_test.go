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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestClientMetricsAreLowCardinalityAndRecordFailures(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"allowed":true}`))
	}))
	defer server.Close()
	client := testClient(t, server.URL)
	if _, err := client.Check(t.Context(), "user:alice", "viewer", "document:secret", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Check(t.Context(), "", "viewer", "document:secret", nil); err == nil {
		t.Fatal("invalid check did not fail")
	}
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatal(err)
	}
	requests := metricPoints(t, collected, "basyx.rebac.client.requests")
	if len(requests) != 2 {
		t.Fatalf("request points = %d, want success and error", len(requests))
	}
	for _, point := range requests {
		assertMetricAttributes(t, point.Attributes)
	}
	if !hasMetricOutcome(requests, "check", "error") {
		t.Fatal("request metrics omitted error outcome")
	}
	errors := metricPoints(t, collected, "basyx.rebac.client.errors")
	if len(errors) != 1 || !hasMetricOutcome(errors, "check", "error") {
		t.Fatalf("error metrics = %#v", errors)
	}
	if batchPoints := histogramPoints(t, collected, "basyx.rebac.client.batch_size"); len(batchPoints) != 0 {
		t.Fatalf("non-batch operation emitted batch size: %#v", batchPoints)
	}
}

func TestBatchCheckMetricsRecordBatchSize(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(previous); _ = provider.Shutdown(t.Context()) })
	client, err := NewClient(Config{URL: "http://example.test", StoreID: "store", ModelID: "model", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.BatchCheck(t.Context(), nil); err == nil {
		t.Fatal("empty batch did not fail")
	}
	var collected metricdata.ResourceMetrics
	if err = reader.Collect(t.Context(), &collected); err != nil {
		t.Fatal(err)
	}
	points := histogramPoints(t, collected, "basyx.rebac.client.batch_size")
	if len(points) != 1 || points[0].Count != 1 || points[0].Sum != 0 {
		t.Fatalf("batch points = %#v", points)
	}
	assertMetricAttributes(t, points[0].Attributes)
}

func metricPoints(t *testing.T, collected metricdata.ResourceMetrics, name string) []metricdata.DataPoint[int64] {
	t.Helper()
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				if sum, ok := metric.Data.(metricdata.Sum[int64]); ok {
					return sum.DataPoints
				}
			}
		}
	}
	return nil
}

func histogramPoints(t *testing.T, collected metricdata.ResourceMetrics, name string) []metricdata.HistogramDataPoint[int64] {
	t.Helper()
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				if histogram, ok := metric.Data.(metricdata.Histogram[int64]); ok {
					return histogram.DataPoints
				}
			}
		}
	}
	return nil
}

func assertMetricAttributes(t *testing.T, set attribute.Set) {
	t.Helper()
	for _, item := range set.ToSlice() {
		if string(item.Key) != "operation" && string(item.Key) != "outcome" {
			t.Fatalf("unexpected identifying metric attribute %q", item.Key)
		}
	}
	if _, ok := set.Value("operation"); !ok {
		t.Fatal("metric omitted operation")
	}
	if _, ok := set.Value("outcome"); !ok {
		t.Fatal("metric omitted outcome")
	}
}

func hasMetricOutcome(points []metricdata.DataPoint[int64], operation, outcome string) bool {
	for _, point := range points {
		actualOperation, hasOperation := point.Attributes.Value("operation")
		actualOutcome, hasOutcome := point.Attributes.Value("outcome")
		if hasOperation && hasOutcome && actualOperation.AsString() == operation && actualOutcome.AsString() == outcome {
			return true
		}
	}
	return false
}
