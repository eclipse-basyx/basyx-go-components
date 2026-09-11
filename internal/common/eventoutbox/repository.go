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

// Package eventoutbox provides durable, independently acknowledged event delivery.
package eventoutbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
)

const table = "event_outbox"

var dialect = goqu.Dialect("postgres")

// Publisher delivers one immutable envelope using adapter-specific routing metadata.
type Publisher interface {
	Publish(context.Context, json.RawMessage, []byte) error
}

// Repository persists pending deliveries separately from the HTTP feed.
type Repository struct{ db *sql.DB }

// NewRepository uses the model writer database for durable delivery.
func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// Enqueue writes one sink's delivery in the model transaction.
func (r *Repository) Enqueue(ctx context.Context, tx *sql.Tx, sink, key string, event events.FeedEvent, routing json.RawMessage) error {
	record, err := events.Record(event, false)
	if err != nil {
		return err
	}
	envelope, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("OUTBOX-ENQUEUE-JSON: %w", err)
	}
	query, args, err := dialect.Insert(table).Rows(goqu.Record{
		"sink_id": sink, "event_id": event.ID, "ordering_key": key, "envelope": string(envelope),
		"routing": goqu.L("?::jsonb", string(routing)), "created_at": event.Time,
	}).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("OUTBOX-ENQUEUE-SQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("OUTBOX-ENQUEUE-EXEC: %w", err)
	}
	return nil
}

type delivery struct {
	seq      int64
	envelope string
	routing  json.RawMessage
	attempts int
}

func claimQuery(sink string) (string, []any, error) {
	earlier := dialect.From(goqu.T(table).As("earlier")).Select(goqu.L("1")).Where(
		goqu.I("earlier.sink_id").Eq(goqu.I("pending.sink_id")),
		goqu.I("earlier.ordering_key").Eq(goqu.I("pending.ordering_key")),
		goqu.I("earlier.seq").Lt(goqu.I("pending.seq")),
	)
	return dialect.From(goqu.T(table).As("pending")).Select(goqu.I("pending.seq"), goqu.I("pending.envelope"), goqu.I("pending.routing"), goqu.I("pending.attempts")).Where(
		goqu.I("pending.sink_id").Eq(sink), goqu.I("pending.next_attempt_at").Lte(goqu.Func("clock_timestamp")),
		goqu.L("NOT EXISTS ?", earlier),
	).Order(goqu.I("pending.seq").Asc()).Limit(1).ForUpdate(goqu.SkipLocked).Prepared(true).ToSQL()
}

func claim(ctx context.Context, tx *sql.Tx, sink, owner string) (delivery, error) {
	query, args, err := claimQuery(sink)
	if err != nil {
		return delivery{}, fmt.Errorf("OUTBOX-CLAIM-SQL: %w", err)
	}
	var item delivery
	var routing []byte
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&item.seq, &item.envelope, &routing, &item.attempts); err != nil {
		return item, err
	}
	item.routing = routing
	err = updateDelivery(ctx, tx, item.seq, goqu.Record{"worker_id": owner})
	return item, err
}

// DeliverOne holds only a delivery-row lock while publishing, never a model transaction.
// The lock prevents concurrent replicas from passing an earlier pending event and
// is released by PostgreSQL on worker failure, without expiring leases or stale owners.
func (r *Repository) DeliverOne(ctx context.Context, sink, owner string, publisher Publisher) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("OUTBOX-DELIVER-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	item, err := claim(ctx, tx, sink, owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("OUTBOX-DELIVER-CLAIM: %w", err)
	}
	publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	publishErr := publisher.Publish(publishCtx, item.routing, []byte(item.envelope))
	cancel()
	if err = finishDelivery(ctx, tx, item, publishErr); err != nil {
		return true, err
	}
	if err = tx.Commit(); err != nil {
		return true, fmt.Errorf("OUTBOX-DELIVER-COMMIT: %w", err)
	}
	return true, publishErr
}

func finishDelivery(ctx context.Context, tx *sql.Tx, item delivery, publishErr error) error {
	if publishErr != nil {
		return updateDelivery(ctx, tx, item.seq, goqu.Record{
			"attempts":        goqu.L("LEAST(? + 1, 2147483647)", goqu.C("attempts").Cast("bigint")),
			"next_attempt_at": time.Now().UTC().Add(events.RetryDelay(item.attempts + 1)),
			"worker_id":       nil, "last_error_code": "OUTBOX-DELIVER-PUBLISH",
		})
	}
	query, args, err := dialect.Delete(table).Where(goqu.C("seq").Eq(item.seq)).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("OUTBOX-ACK-SQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("OUTBOX-ACK-EXEC: %w", err)
	}
	return nil
}

func updateDelivery(ctx context.Context, tx *sql.Tx, seq int64, fields goqu.Record) error {
	query, args, err := dialect.Update(table).Set(fields).Where(goqu.C("seq").Eq(seq)).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("OUTBOX-UPDATE-SQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("OUTBOX-UPDATE-EXEC: %w", err)
	}
	return nil
}

// Stats returns the pending count, oldest capture time, and deliveries awaiting retry.
func (r *Repository) Stats(ctx context.Context, sink string) (int64, time.Time, int64, error) {
	query, args, err := dialect.From(table).Select(goqu.COUNT("*"), goqu.MIN("created_at"), goqu.SUM(goqu.Case().When(goqu.C("attempts").Gt(0), goqu.L("1")).Else(goqu.L("0")))).Where(goqu.C("sink_id").Eq(sink)).Prepared(true).ToSQL()
	if err != nil {
		return 0, time.Time{}, 0, fmt.Errorf("OUTBOX-STATS-SQL: %w", err)
	}
	var count int64
	var oldest sql.NullTime
	var retries sql.NullInt64
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&count, &oldest, &retries)
	if err != nil {
		return 0, time.Time{}, 0, fmt.Errorf("OUTBOX-STATS-QUERY: %w", err)
	}
	return count, oldest.Time, retries.Int64, nil
}
