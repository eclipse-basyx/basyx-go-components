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

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventoutbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

type outboxClaimTrace struct {
	query string
	args  []any
}

func (trace *outboxClaimTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "event_outbox") && strings.Contains(data.SQL, "FOR UPDATE") {
		trace.query, trace.args = data.SQL, append([]any(nil), data.Args...)
	}
	return ctx
}

func (*outboxClaimTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestMQTTOutboxClaimSkipsBlockedBacklogWithoutScanningDescendants(t *testing.T) {
	assertOutboxClaimBounded(t, true, 1)
}

func TestMQTTOutboxClaimAvoidsScanningIndependentReadyEntities(t *testing.T) {
	assertOutboxClaimBounded(t, false, 10000)
}

func TestMQTTOutboxClaimAvoidsScanningReadyEntitiesBehindBlockedBacklog(t *testing.T) {
	assertOutboxClaimBounded(t, true, 10000)
}

func assertOutboxClaimBounded(t *testing.T, blocked bool, readyCount int) {
	t.Helper()
	trace := &outboxClaimTrace{}
	config, err := pgx.ParseConfig(submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	config.Tracer = trace
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() { require.NoError(t, db.Close()) }()
	_, err = db.ExecContext(t.Context(), "CREATE TEMP TABLE event_outbox (LIKE public.event_outbox INCLUDING ALL)")
	require.NoError(t, err)
	if blocked {
		populateBlockedOutbox(t, db)
	}
	populateReadyOutbox(t, db, readyCount)
	_, err = db.ExecContext(t.Context(), "ANALYZE event_outbox")
	require.NoError(t, err)
	repository := eventoutbox.NewRepository(db)
	var payload string
	found, err := repository.DeliverOne(t.Context(), "backlog-test", outboxTestPublisher(func(_ context.Context, _ json.RawMessage, raw []byte) error {
		payload = string(raw)
		return nil
	}))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "ready", payload)
	require.NotEmpty(t, trace.query)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var raw []byte
	// EXPLAIN wraps the actual GOQU statement captured from DeliverOne.
	err = tx.QueryRowContext(t.Context(), "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+trace.query, trace.args...).Scan(&raw)
	require.NoError(t, err)
	var plans []struct{ Plan outboxQueryPlan }
	require.NoError(t, json.Unmarshal(raw, &plans))
	require.Len(t, plans, 1)
	require.Less(t, outboxPlanVisitedRows(plans[0].Plan), float64(1000), "claim must stop after finding an eligible entity head: %s", raw)
}

func populateBlockedOutbox(t *testing.T, db *sql.DB) {
	t.Helper()
	dialect := goqu.Dialect("postgres")
	rows := dialect.From(goqu.L("generate_series(1, 10001)").As("n")).Select(
		goqu.I("n"), goqu.V("backlog-test"), goqu.I("n").Cast("text"), goqu.V("blocked"),
		goqu.V("blocked"), goqu.L("?::jsonb", `"test/topic"`), goqu.Func("now"),
		goqu.Case().When(goqu.I("n").Eq(1), goqu.L("? + INTERVAL '1 hour'", goqu.Func("now"))).Else(goqu.Func("now")),
	)
	query, args, err := dialect.Insert("event_outbox").Cols("seq", "sink_id", "event_id", "ordering_key", "envelope", "routing", "created_at", "next_attempt_at").FromQuery(rows).Prepared(true).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
}

func populateReadyOutbox(t *testing.T, db *sql.DB, count int) {
	t.Helper()
	dialect := goqu.Dialect("postgres")
	identifier := goqu.L("'ready-' || ?::text", goqu.I("n"))
	rows := dialect.From(goqu.L("generate_series(10002, ?)", 10001+count).As("n")).Select(
		goqu.I("n"), goqu.V("backlog-test"), identifier, identifier,
		goqu.V("ready"), goqu.L("?::jsonb", `"test/topic"`), goqu.Func("now"),
	)
	query, args, err := dialect.Insert("event_outbox").Cols("seq", "sink_id", "event_id", "ordering_key", "envelope", "routing", "created_at").FromQuery(rows).Prepared(true).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
}

type outboxQueryPlan struct {
	ActualRows      float64           `json:"Actual Rows"`
	ActualLoops     float64           `json:"Actual Loops"`
	RemovedByFilter float64           `json:"Rows Removed by Filter"`
	Plans           []outboxQueryPlan `json:"Plans"`
}

func outboxPlanVisitedRows(plan outboxQueryPlan) float64 {
	visited := (plan.ActualRows + plan.RemovedByFilter) * plan.ActualLoops
	for _, child := range plan.Plans {
		visited += outboxPlanVisitedRows(child)
	}
	return visited
}
