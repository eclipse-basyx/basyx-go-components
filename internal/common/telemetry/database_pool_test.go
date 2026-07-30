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
	"database/sql"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func TestDatabasePoolMetricsReflectDBStats(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(resource.NewSchemaless(semconv.ServiceName("testservice"))),
	)
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	var registry databasePoolRegistry
	if err := registry.initialize(provider.Meter(instrumentationName)); err != nil {
		t.Fatalf("initialize database pool metrics: %v", err)
	}
	db := new(sql.DB)
	stats := sql.DBStats{
		MaxOpenConnections: 40,
		OpenConnections:    17,
		InUse:              12,
		Idle:               5,
		WaitCount:          23,
		WaitDuration:       3500 * time.Millisecond,
		MaxIdleClosed:      7,
		MaxIdleTimeClosed:  11,
		MaxLifetimeClosed:  13,
	}
	if err := registry.register(db, DatabasePoolRoleWriter, func() sql.DBStats { return stats }); err != nil {
		t.Fatalf("register database pool: %v", err)
	}
	t.Cleanup(registry.unregister)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("collect database pool metrics: %v", err)
	}

	if serviceName, ok := collected.Resource.Set().Value(semconv.ServiceNameKey); !ok || serviceName.AsString() != "testservice" {
		t.Fatalf("unexpected service resource: %v", serviceName)
	}
	assertSingleInt64Point(t, collected, "db.client.connection.max", 40)
	countPoints := int64Points(t, collected, "db.client.connection.count")
	if got := pointValueByAttribute(t, countPoints, semconv.DBClientConnectionStateKey, "used"); got != 12 {
		t.Fatalf("unexpected in-use connection count: got %d want 12", got)
	}
	if got := pointValueByAttribute(t, countPoints, semconv.DBClientConnectionStateKey, "idle"); got != 5 {
		t.Fatalf("unexpected idle connection count: got %d want 5", got)
	}
	if got := sumPointValues(countPoints); got != int64(stats.OpenConnections) {
		t.Fatalf("unexpected open connection count: got %d want %d", got, stats.OpenConnections)
	}
	assertSingleInt64Point(t, collected, "basyx.db.client.connection.waits", 23)
	assertSingleFloat64Point(t, collected, "basyx.db.client.connection.wait_time", 3.5)

	closedPoints := int64Points(t, collected, "basyx.db.client.connection.closed")
	if got := pointValueByAttribute(t, closedPoints, databasePoolCloseReasonKey, "idle_limit"); got != 7 {
		t.Fatalf("unexpected idle-limit closure count: got %d want 7", got)
	}
	if got := pointValueByAttribute(t, closedPoints, databasePoolCloseReasonKey, "idle_time"); got != 11 {
		t.Fatalf("unexpected idle-time closure count: got %d want 11", got)
	}
	if got := pointValueByAttribute(t, closedPoints, databasePoolCloseReasonKey, "max_lifetime"); got != 13 {
		t.Fatalf("unexpected max-lifetime closure count: got %d want 13", got)
	}

	for _, point := range append(countPoints, closedPoints...) {
		assertPoolAttributes(t, point.Attributes)
	}
	for _, metricName := range []string{
		"db.client.connection.max",
		"basyx.db.client.connection.waits",
		"basyx.db.client.connection.wait_time",
	} {
		assertMetricPoolAttributes(t, collected, metricName)
	}
}

func TestDatabasePoolRegistrationIsIdempotentAndRolesAreUnique(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	var registry databasePoolRegistry
	if err := registry.initialize(provider.Meter(instrumentationName)); err != nil {
		t.Fatalf("initialize database pool metrics: %v", err)
	}
	firstDB := new(sql.DB)
	stats := func() sql.DBStats { return sql.DBStats{MaxOpenConnections: 10} }
	if err := registry.register(firstDB, DatabasePoolRoleWriter, stats); err != nil {
		t.Fatalf("register database pool: %v", err)
	}
	if err := registry.register(firstDB, DatabasePoolRoleWriter, stats); err != nil {
		t.Fatalf("repeat database pool registration: %v", err)
	}
	if got := len(registry.registrations); got != 1 {
		t.Fatalf("duplicate registration created %d callbacks", got)
	}

	if err := registry.register(firstDB, DatabasePoolRoleReader, stats); err == nil || !strings.Contains(err.Error(), "OTEL-DBPOOL-CONFLICT") {
		t.Fatalf("expected conflicting role error, got %v", err)
	}
	if err := registry.register(new(sql.DB), DatabasePoolRoleWriter, stats); err == nil || !strings.Contains(err.Error(), "OTEL-DBPOOL-CONFLICT") {
		t.Fatalf("expected duplicate role error, got %v", err)
	}
	registry.unregister()
}

func TestRegisterDatabasePoolDoesNothingWhenMetricsAreDisabled(t *testing.T) {
	previousRuntime := activeRuntime.Swap(&Runtime{
		enabled:        true,
		tracingEnabled: true,
	})
	t.Cleanup(func() { activeRuntime.Store(previousRuntime) })

	if err := RegisterDatabasePool(new(sql.DB), DatabasePoolRoleWriter); err != nil {
		t.Fatalf("disabled metric registration: %v", err)
	}
	if err := UnregisterDatabasePool(new(sql.DB)); err != nil {
		t.Fatalf("disabled metric unregistration: %v", err)
	}
}

func TestRegisterDatabasePoolUsesActiveRuntime(t *testing.T) {
	runtime, reader := installDatabasePoolTestRuntime(t)
	db := new(sql.DB)
	db.SetMaxOpenConns(24)

	if err := RegisterDatabasePool(db, DatabasePoolRoleWriter); err != nil {
		t.Fatalf("register database pool: %v", err)
	}
	if got := len(runtime.databasePools.registrations); got != 1 {
		t.Fatalf("active runtime contains %d registrations, want 1", got)
	}

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("collect database pool metrics: %v", err)
	}
	assertSingleInt64Point(t, collected, "db.client.connection.max", 24)
}

func TestUnregisterDatabasePoolAllowsRoleReplacement(t *testing.T) {
	runtime, reader := installDatabasePoolTestRuntime(t)
	firstDB := new(sql.DB)
	firstDB.SetMaxOpenConns(12)
	if err := RegisterDatabasePool(firstDB, DatabasePoolRoleWriter); err != nil {
		t.Fatalf("register first database pool: %v", err)
	}
	if err := UnregisterDatabasePool(firstDB); err != nil {
		t.Fatalf("unregister first database pool: %v", err)
	}
	if err := UnregisterDatabasePool(firstDB); err != nil {
		t.Fatalf("repeat database pool unregistration: %v", err)
	}

	replacementDB := new(sql.DB)
	replacementDB.SetMaxOpenConns(36)
	if err := RegisterDatabasePool(replacementDB, DatabasePoolRoleWriter); err != nil {
		t.Fatalf("register replacement database pool: %v", err)
	}
	if got := len(runtime.databasePools.registrations); got != 1 {
		t.Fatalf("active runtime contains %d registrations, want 1", got)
	}

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("collect replacement database pool metrics: %v", err)
	}
	assertSingleInt64Point(t, collected, "db.client.connection.max", 36)
}

func TestRegisterDatabasePoolRejectsInvalidInputs(t *testing.T) {
	if err := RegisterDatabasePool(nil, DatabasePoolRoleWriter); err == nil || !strings.Contains(err.Error(), "OTEL-DBPOOL-NILDB") {
		t.Fatalf("expected nil database error, got %v", err)
	}
	if err := RegisterDatabasePool(new(sql.DB), "tenant-123"); err == nil || !strings.Contains(err.Error(), "OTEL-DBPOOL-ROLE") {
		t.Fatalf("expected invalid role error, got %v", err)
	}
	if err := UnregisterDatabasePool(nil); err == nil || !strings.Contains(err.Error(), "OTEL-DBPOOL-NILDB") {
		t.Fatalf("expected nil database error, got %v", err)
	}
}

func installDatabasePoolTestRuntime(t *testing.T) (*Runtime, *metric.ManualReader) {
	t.Helper()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	runtime := &Runtime{
		enabled:        true,
		metricsEnabled: true,
		metricProvider: provider,
	}
	if err := runtime.databasePools.initialize(provider.Meter(instrumentationName)); err != nil {
		t.Fatalf("initialize database pool metrics: %v", err)
	}
	previousRuntime := activeRuntime.Swap(runtime)
	t.Cleanup(func() {
		activeRuntime.Store(previousRuntime)
		runtime.databasePools.unregister()
		_ = provider.Shutdown(t.Context())
	})
	return runtime, reader
}

func int64Points(t *testing.T, collected metricdata.ResourceMetrics, name string) []metricdata.DataPoint[int64] {
	t.Helper()
	for _, scope := range collected.ScopeMetrics {
		for _, collectedMetric := range scope.Metrics {
			if collectedMetric.Name != name {
				continue
			}
			sum, ok := collectedMetric.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric %s has data type %T", name, collectedMetric.Data)
			}
			return sum.DataPoints
		}
	}
	t.Fatalf("metric %s was not collected", name)
	return nil
}

func float64Points(t *testing.T, collected metricdata.ResourceMetrics, name string) []metricdata.DataPoint[float64] {
	t.Helper()
	for _, scope := range collected.ScopeMetrics {
		for _, collectedMetric := range scope.Metrics {
			if collectedMetric.Name != name {
				continue
			}
			sum, ok := collectedMetric.Data.(metricdata.Sum[float64])
			if !ok {
				t.Fatalf("metric %s has data type %T", name, collectedMetric.Data)
			}
			return sum.DataPoints
		}
	}
	t.Fatalf("metric %s was not collected", name)
	return nil
}

func assertSingleInt64Point(
	t *testing.T,
	collected metricdata.ResourceMetrics,
	name string,
	want int64,
) {
	t.Helper()
	points := int64Points(t, collected, name)
	if len(points) != 1 {
		t.Fatalf("metric %s has %d points, want 1", name, len(points))
	}
	if points[0].Value != want {
		t.Fatalf("metric %s has value %d, want %d", name, points[0].Value, want)
	}
}

func assertSingleFloat64Point(t *testing.T, collected metricdata.ResourceMetrics, name string, want float64) {
	t.Helper()
	points := float64Points(t, collected, name)
	if len(points) != 1 || points[0].Value != want {
		t.Fatalf("metric %s has points %v, want one point with value %v", name, points, want)
	}
}

func pointValueByAttribute(
	t *testing.T,
	points []metricdata.DataPoint[int64],
	key attribute.Key,
	wantValue string,
) int64 {
	t.Helper()
	for _, point := range points {
		value, ok := point.Attributes.Value(key)
		if ok && value.AsString() == wantValue {
			return point.Value
		}
	}
	t.Fatalf("point with %s=%q was not collected", key, wantValue)
	return 0
}

func sumPointValues(points []metricdata.DataPoint[int64]) int64 {
	var total int64
	for _, point := range points {
		total += point.Value
	}
	return total
}

func assertPoolAttributes(t *testing.T, attributes attribute.Set) {
	t.Helper()
	poolName, hasPoolName := attributes.Value(semconv.DBClientConnectionPoolNameKey)
	if !hasPoolName || poolName.AsString() != string(DatabasePoolRoleWriter) {
		t.Fatalf("unexpected pool name attribute: %v", poolName)
	}
	systemName, hasSystemName := attributes.Value(semconv.DBSystemNameKey)
	if !hasSystemName || systemName.AsString() != "postgresql" {
		t.Fatalf("unexpected database system attribute: %v", systemName)
	}
}

func assertMetricPoolAttributes(t *testing.T, collected metricdata.ResourceMetrics, name string) {
	t.Helper()
	switch name {
	case "basyx.db.client.connection.wait_time":
		for _, point := range float64Points(t, collected, name) {
			assertPoolAttributes(t, point.Attributes)
		}
	default:
		for _, point := range int64Points(t, collected, name) {
			assertPoolAttributes(t, point.Attributes)
		}
	}
}
