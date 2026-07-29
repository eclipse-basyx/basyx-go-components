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
	"database/sql"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

const databasePoolCloseReasonKey = attribute.Key("basyx.db.client.connection.close.reason")

// DatabasePoolRole identifies a bounded database pool role within a service.
type DatabasePoolRole string

const (
	// DatabasePoolRoleWriter identifies the shared read/write pool.
	DatabasePoolRoleWriter DatabasePoolRole = "writer"
	// DatabasePoolRoleReader identifies a future read-only pool.
	DatabasePoolRoleReader DatabasePoolRole = "reader"
)

type databasePoolMetrics struct {
	meter           metric.Meter
	connectionMax   metric.Int64ObservableUpDownCounter
	connectionCount metric.Int64ObservableUpDownCounter
	waits           metric.Int64ObservableCounter
	waitTime        metric.Float64ObservableCounter
	closed          metric.Int64ObservableCounter
}

type databasePoolRegistration struct {
	role         DatabasePoolRole
	registration metric.Registration
}

type databasePoolRegistry struct {
	mu            sync.Mutex
	metrics       *databasePoolMetrics
	registrations map[*sql.DB]databasePoolRegistration
	roles         map[DatabasePoolRole]*sql.DB
	closed        bool
}

// RegisterDatabasePool registers db with the active metric runtime. Calling it
// while metrics are disabled is a no-op.
func RegisterDatabasePool(db *sql.DB, role DatabasePoolRole) error {
	if db == nil {
		return fmt.Errorf("OTEL-DBPOOL-NILDB database handle is nil")
	}
	if !validDatabasePoolRole(role) {
		return fmt.Errorf("OTEL-DBPOOL-ROLE unsupported database pool role %q", role)
	}

	runtime := activeRuntime.Load()
	if runtime == nil || !runtime.metricsEnabled {
		return nil
	}
	return runtime.databasePools.register(db, role, db.Stats)
}

func (registry *databasePoolRegistry) initialize(meter metric.Meter) error {
	connectionMax, err := meter.Int64ObservableUpDownCounter(
		"db.client.connection.max",
		metric.WithDescription("The maximum number of open connections allowed."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return fmt.Errorf("OTEL-DBPOOL-INSTRUMENT create maximum connection instrument: %w", err)
	}
	connectionCount, err := meter.Int64ObservableUpDownCounter(
		"db.client.connection.count",
		metric.WithDescription("The number of connections that are currently in the described state."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return fmt.Errorf("OTEL-DBPOOL-INSTRUMENT create connection count instrument: %w", err)
	}
	waits, err := meter.Int64ObservableCounter(
		"basyx.db.client.connection.waits",
		metric.WithDescription("The cumulative number of waits for a database connection."),
		metric.WithUnit("{wait}"),
	)
	if err != nil {
		return fmt.Errorf("OTEL-DBPOOL-INSTRUMENT create connection wait instrument: %w", err)
	}
	waitTime, err := meter.Float64ObservableCounter(
		"basyx.db.client.connection.wait_time",
		metric.WithDescription("The cumulative time spent waiting for database connections."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("OTEL-DBPOOL-INSTRUMENT create connection wait time instrument: %w", err)
	}
	closed, err := meter.Int64ObservableCounter(
		"basyx.db.client.connection.closed",
		metric.WithDescription("The cumulative number of database connections closed by pool limits."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return fmt.Errorf("OTEL-DBPOOL-INSTRUMENT create closed connection instrument: %w", err)
	}

	registry.metrics = &databasePoolMetrics{
		meter:           meter,
		connectionMax:   connectionMax,
		connectionCount: connectionCount,
		waits:           waits,
		waitTime:        waitTime,
		closed:          closed,
	}
	registry.registrations = make(map[*sql.DB]databasePoolRegistration)
	registry.roles = make(map[DatabasePoolRole]*sql.DB)
	return nil
}

func (registry *databasePoolRegistry) register(
	db *sql.DB,
	role DatabasePoolRole,
	stats func() sql.DBStats,
) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if registry.closed {
		return fmt.Errorf("OTEL-DBPOOL-SHUTDOWN telemetry runtime is shutting down")
	}
	if existing, ok := registry.registrations[db]; ok {
		if existing.role == role {
			return nil
		}
		return fmt.Errorf("OTEL-DBPOOL-CONFLICT database pool is already registered as %q", existing.role)
	}
	if existingDB, ok := registry.roles[role]; ok && existingDB != db {
		return fmt.Errorf("OTEL-DBPOOL-CONFLICT database pool role %q is already registered", role)
	}

	registration, err := registry.registerCallback(role, stats)
	if err != nil {
		return err
	}
	registry.registrations[db] = databasePoolRegistration{
		role:         role,
		registration: registration,
	}
	registry.roles[role] = db
	return nil
}

func (registry *databasePoolRegistry) registerCallback(
	role DatabasePoolRole,
	stats func() sql.DBStats,
) (metric.Registration, error) {
	metrics := registry.metrics
	registration, err := metrics.meter.RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			observeDatabasePool(observer, metrics, role, stats())
			return nil
		},
		metrics.connectionMax,
		metrics.connectionCount,
		metrics.waits,
		metrics.waitTime,
		metrics.closed,
	)
	if err != nil {
		return nil, fmt.Errorf("OTEL-DBPOOL-REGISTER register database pool callback: %w", err)
	}
	return registration, nil
}

func observeDatabasePool(
	observer metric.Observer,
	metrics *databasePoolMetrics,
	role DatabasePoolRole,
	stats sql.DBStats,
) {
	poolAttributes := metric.WithAttributes(
		semconv.DBSystemNamePostgreSQL,
		semconv.DBClientConnectionPoolName(string(role)),
	)
	observer.ObserveInt64(metrics.connectionMax, int64(stats.MaxOpenConnections), poolAttributes)
	observer.ObserveInt64(
		metrics.connectionCount,
		int64(stats.InUse),
		metric.WithAttributes(
			semconv.DBSystemNamePostgreSQL,
			semconv.DBClientConnectionPoolName(string(role)),
			semconv.DBClientConnectionStateUsed,
		),
	)
	observer.ObserveInt64(
		metrics.connectionCount,
		int64(stats.Idle),
		metric.WithAttributes(
			semconv.DBSystemNamePostgreSQL,
			semconv.DBClientConnectionPoolName(string(role)),
			semconv.DBClientConnectionStateIdle,
		),
	)
	observer.ObserveInt64(metrics.waits, stats.WaitCount, poolAttributes)
	observer.ObserveFloat64(metrics.waitTime, stats.WaitDuration.Seconds(), poolAttributes)
	observeClosedConnections(observer, metrics.closed, role, "idle_limit", stats.MaxIdleClosed)
	observeClosedConnections(observer, metrics.closed, role, "idle_time", stats.MaxIdleTimeClosed)
	observeClosedConnections(observer, metrics.closed, role, "max_lifetime", stats.MaxLifetimeClosed)
}

func observeClosedConnections(
	observer metric.Observer,
	instrument metric.Int64ObservableCounter,
	role DatabasePoolRole,
	reason string,
	value int64,
) {
	observer.ObserveInt64(
		instrument,
		value,
		metric.WithAttributes(
			semconv.DBSystemNamePostgreSQL,
			semconv.DBClientConnectionPoolName(string(role)),
			databasePoolCloseReasonKey.String(reason),
		),
	)
}

func (registry *databasePoolRegistry) unregister() {
	registry.mu.Lock()
	registry.closed = true
	registrations := make([]metric.Registration, 0, len(registry.registrations))
	for _, registeredPool := range registry.registrations {
		registrations = append(registrations, registeredPool.registration)
	}
	clear(registry.registrations)
	clear(registry.roles)
	registry.mu.Unlock()

	for _, registration := range registrations {
		if err := registration.Unregister(); err != nil {
			logRuntimeWarning("OpenTelemetry database pool unregister failed", "OTEL-DBPOOL-UNREGISTER", err)
		}
	}
}

func validDatabasePoolRole(role DatabasePoolRole) bool {
	return role == DatabasePoolRoleWriter || role == DatabasePoolRoleReader
}
