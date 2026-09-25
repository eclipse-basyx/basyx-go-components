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

package auth

import (
	"context"
	"database/sql"
	"github.com/doug-martin/goqu/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"time"
)

func (s *rebacSecurity) observeOperationalMetrics(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		s.recordOperationalMetrics(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *rebacSecurity) recordOperationalMetrics(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From("rebac_scope").Select(goqu.L("desired_revision - applied_revision")).Where(goqu.C("scope").Eq(s.cfg.ReBAC.Scope)).Prepared(true).ToSQL()
	var gap int64
	if err == nil && s.runtime.State.DB.QueryRowContext(ctx, query, args...).Scan(&gap) == nil {
		recordOperationalGauge(ctx, "basyx.rebac.sync.revision_gap", float64(gap), "")
	}
	query, args, err = dialect.From("rebac_outbox").Select(goqu.MIN("created_at"), goqu.COALESCE(goqu.SUM("attempts"), 0)).Where(goqu.C("scope").Eq(s.cfg.ReBAC.Scope), goqu.C("revision").Gt(dialect.From("rebac_scope").Select("applied_revision").Where(goqu.C("scope").Eq(s.cfg.ReBAC.Scope)))).Prepared(true).ToSQL()
	var oldest sql.NullTime
	var attempts int64
	if err == nil && s.runtime.State.DB.QueryRowContext(ctx, query, args...).Scan(&oldest, &attempts) == nil {
		recordOperationalGauge(ctx, "basyx.rebac.barrier.age", pendingAge(oldest), "s")
		recordOperationalGauge(ctx, "basyx.rebac.sync.pending_attempts", float64(attempts), "")
	}
	query, args, err = dialect.From("audit_delivery").Join(goqu.T("audit_record"), goqu.On(goqu.I("audit_delivery.event_id").Eq(goqu.I("audit_record.event_id")))).Select(goqu.COUNT("*"), goqu.MIN("occurred_at")).Where(goqu.C("archived_at").IsNull()).Prepared(true).ToSQL()
	var backlog int64
	if err == nil && s.runtime.State.DB.QueryRowContext(ctx, query, args...).Scan(&backlog, &oldest) == nil {
		recordOperationalGauge(ctx, "basyx.audit.backlog", float64(backlog), "")
		recordOperationalGauge(ctx, "basyx.audit.worm.archival_lag", pendingAge(oldest), "s")
	}
}

func pendingAge(value sql.NullTime) float64 {
	if !value.Valid {
		return 0
	}
	return max(0, time.Since(value.Time).Seconds())
}

func recordOperationalGauge(ctx context.Context, name string, value float64, unit string) {
	gauge, err := otel.Meter("basyx/security").Float64Gauge(name, metric.WithUnit(unit))
	if err == nil {
		gauge.Record(ctx, value)
	}
}
