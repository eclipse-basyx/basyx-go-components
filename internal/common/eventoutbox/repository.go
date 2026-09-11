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
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
)

const table = "event_outbox"

var dialect = goqu.Dialect("postgres")

// Publisher delivers an immutable CloudEvents envelope using stored routing metadata.
// Publish must honor context cancellation and return nil only after the transport
// accepts delivery. An error leaves the entry pending for another attempt.
type Publisher interface {
	Publish(context.Context, json.RawMessage, []byte) error
}

// Repository persists pending deliveries separately from the HTTP feed.
type Repository struct{ db *sql.DB }

// NewRepository connects an outbox to the model writer database.
//
// Parameters:
//   - db: Database pool owned by the caller; keep open until workers have stopped.
//
// Returns:
//   - *Repository: Outbox repository using db for all reads and writes.
func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// Enqueue stores one REGULAR CloudEvent in the caller's model transaction.
//
// The caller retains commit/rollback ownership and must roll back tx on error.
// A successful enqueue becomes eligible for delivery only after commit.
//
// Parameters:
//   - ctx: Mutation request context, including security metadata.
//   - tx: Active model transaction.
//   - sink: Stable destination ID used by the delivery workers.
//   - key: Mutated entity's ordering key, shared by its derived events.
//   - event: Captured event with its final ID and timestamp.
//   - routing: Valid JSON metadata produced by the transport adapter.
//
// Returns:
//   - error: Coded serialization or insertion error; otherwise nil.
//
// Example:
//
//	routing, err := mqtt.Routing(topicPrefix, event)
//	if err != nil {
//		return err
//	}
//	return repository.Enqueue(ctx, tx, sinkID, mutation.Table+":"+mutation.Identifier, event, routing)
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
	seq         int64
	envelope    string
	routing     json.RawMessage
	attempts    int
	orderingKey string
}

func claimQuery(sink, afterKey string) (string, []any, error) {
	head := dialect.From(table).Select("ordering_key", "seq").Where(goqu.C("sink_id").Eq(sink)).
		Order(goqu.C("ordering_key").Asc(), goqu.C("seq").Asc()).Limit(1)
	first := head
	if afterKey != "" {
		first = first.Where(goqu.C("ordering_key").Gt(afterKey))
	}
	next := head.Where(goqu.C("ordering_key").Gt(goqu.I("heads.ordering_key")))
	step := dialect.From("heads").CrossJoin(goqu.Lateral(next).As("next_head")).
		Select(goqu.I("next_head.ordering_key"), goqu.I("next_head.seq"))
	ready := dialect.From(goqu.T(table).As("pending")).
		Select(goqu.I("pending.seq"), goqu.I("pending.envelope"), goqu.I("pending.routing"), goqu.I("pending.attempts"), goqu.I("pending.ordering_key")).
		Where(goqu.I("pending.seq").Eq(goqu.I("heads.seq")), goqu.I("pending.next_attempt_at").Lte(goqu.Func("statement_timestamp"))).
		Limit(1).ForUpdate(goqu.SkipLocked, goqu.T("pending"))
	return dialect.From("heads").WithRecursive("heads", first.UnionAll(step)).
		CrossJoin(goqu.Lateral(ready).As("ready")).Select(goqu.I("ready.*")).Limit(1).Prepared(true).ToSQL()
}

func claim(ctx context.Context, tx *sql.Tx, sink, afterKey string) (delivery, error) {
	item, err := claimAfter(ctx, tx, sink, afterKey)
	if errors.Is(err, sql.ErrNoRows) && afterKey != "" {
		return claimAfter(ctx, tx, sink, "")
	}
	return item, err
}

func claimAfter(ctx context.Context, tx *sql.Tx, sink, afterKey string) (delivery, error) {
	query, args, err := claimQuery(sink, afterKey)
	if err != nil {
		return delivery{}, fmt.Errorf("OUTBOX-CLAIM-SQL: %w", err)
	}
	var item delivery
	var routing []byte
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&item.seq, &item.envelope, &routing, &item.attempts, &item.orderingKey); err != nil {
		return item, err
	}
	item.routing = routing
	return item, nil
}

// DeliverOne claims and attempts one eligible delivery for a destination.
//
// It opens its own transaction and holds a row lock during the publish attempt,
// bounded by ctx and a ten-second timeout. Workers cannot pass an earlier pending
// event for the same entity. PostgreSQL releases the lock on connection loss.
// Successful acknowledgments delete the row; publish failures store a retry time.
// Database failures roll back the delivery transaction.
//
// Parameters:
//   - ctx: Worker context controlling the database transaction and publish deadline.
//   - sink: Destination ID whose pending queue is processed.
//   - publisher: Transport adapter receiving the immutable envelope and routing metadata.
//
// Returns:
//   - bool: True once a row is claimed, including failed attempts; false when no row is ready or claiming fails.
//   - error: Publish or database error; nil if no row is ready or an acknowledged deletion commits.
func (r *Repository) DeliverOne(ctx context.Context, sink string, publisher Publisher) (bool, error) {
	found, _, err := r.deliverNext(ctx, sink, publisher, "")
	return found, err
}

func (r *Repository) deliverNext(ctx context.Context, sink string, publisher Publisher, afterKey string) (bool, string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, afterKey, fmt.Errorf("OUTBOX-DELIVER-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	item, err := claim(ctx, tx, sink, afterKey)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, afterKey, fmt.Errorf("OUTBOX-DELIVER-CLAIM: %w", err)
	}
	publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	publishErr := publisher.Publish(publishCtx, item.routing, []byte(item.envelope))
	cancel()
	if err = finishDelivery(ctx, tx, item, publishErr); err != nil {
		return true, item.orderingKey, err
	}
	if err = tx.Commit(); err != nil {
		return true, item.orderingKey, fmt.Errorf("OUTBOX-DELIVER-COMMIT: %w", err)
	}
	return true, item.orderingKey, publishErr
}

func finishDelivery(ctx context.Context, tx *sql.Tx, item delivery, publishErr error) error {
	if publishErr != nil {
		return updateDelivery(ctx, tx, item.seq, goqu.Record{
			"attempts":        goqu.L("LEAST(? + 1, 2147483647)", goqu.C("attempts").Cast("bigint")),
			"next_attempt_at": time.Now().UTC().Add(events.RetryDelay(item.attempts + 1)),
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

// Stats reads delivery queue counts and age information for a destination.
//
// Parameters:
//   - ctx: Context controlling the database query.
//   - sink: Destination ID to inspect.
//
// Returns:
//   - int64: Total pending deliveries.
//   - time.Time: Oldest capture time, or zero for an empty queue.
//   - int64: Pending deliveries with at least one failed attempt.
//   - error: Coded database error; otherwise nil.
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
