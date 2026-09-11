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

package eventfeed

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // registers the postgres dialect with goqu
)

const (
	retentionBatchSize = 1000
	retentionLockKey   = int64(6471200)

	publishBatchSize = 500
	publishLockKey   = int64(6471201)
)

type queryExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Repository persists and queries event feed records.
type Repository struct {
	db      *sql.DB
	dialect goqu.DialectWrapper
	maxAge  time.Duration
	now     func() time.Time
}

// NewRepository creates a Repository backed by db, treating events older
// than maxAge as outside the retention window for page queries.
func NewRepository(db *sql.DB, maxAge time.Duration) *Repository {
	return &Repository{
		db:      db,
		dialect: goqu.Dialect("postgres"),
		maxAge:  maxAge,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Save persists event using a dedicated connection (not a model transaction).
func (r *Repository) Save(ctx context.Context, event FeedEvent) error {
	_, err := r.save(ctx, r.db, event)
	return err
}

// SaveTx persists event in the same writer transaction as the model mutation.
func (r *Repository) SaveTx(ctx context.Context, tx *sql.Tx, event FeedEvent) (FeedEvent, error) {
	return r.save(ctx, tx, event)
}

func (r *Repository) save(ctx context.Context, exec queryExecer, event FeedEvent) (FeedEvent, error) {
	authorization, err := json.Marshal(event.AuthorizationAASIDs)
	if err != nil {
		return FeedEvent{}, fmt.Errorf("EVENTFEED-SAVE-AUTHJSON: %w", err)
	}
	query, args, err := r.dialect.Insert("feed_events").Rows(goqu.Record{
		"id":                    event.ID,
		"event_type":            event.Type,
		"subject":               event.Subject,
		"source":                event.Source,
		"dataschema_full":       event.DataSchemaFull,
		"dataschema_compact":    event.DataSchemaCompact,
		"data_full":             goqu.L("?::jsonb", event.DataFull),
		"data_compact":          goqu.L("?::jsonb", event.DataCompact),
		"authorization_aas_ids": goqu.L("?::jsonb", string(authorization)),
	}).Returning("seq", "time").ToSQL()
	if err != nil {
		return FeedEvent{}, fmt.Errorf("EVENTFEED-SAVE-BUILDSQL: %w", err)
	}
	if err = exec.QueryRowContext(ctx, query, args...).Scan(&event.Seq, &event.Time); err != nil {
		return FeedEvent{}, fmt.Errorf("EVENTFEED-SAVE-EXEC: %w", err)
	}
	return event, nil
}

// FindByID looks up a single event by its CloudEvents id. found is false if
// no such event exists. e.PublishSeq is 0 if the event has not yet been
// assigned a publish_seq (see Service.RunPublishAssignment) - callers must
// not treat such an event as safe to resume from.
func (r *Repository) FindByID(ctx context.Context, id string) (FeedEvent, bool, error) {
	query, args, err := r.dialect.From("feed_events").
		Select("seq", "publish_seq", "id", "event_type", "subject", "source", "time",
			"dataschema_full", "dataschema_compact", "data_full", "data_compact", "authorization_aas_ids").
		Where(goqu.C("id").Eq(id)).
		ToSQL()
	if err != nil {
		return FeedEvent{}, false, fmt.Errorf("EVENTFEED-FINDBYID-BUILDSQL: %w", err)
	}
	var e FeedEvent
	var publishSeq sql.NullInt64
	var authorization []byte
	err = r.db.QueryRowContext(ctx, query, args...).Scan(
		&e.Seq, &publishSeq, &e.ID, &e.Type, &e.Subject, &e.Source, &e.Time,
		&e.DataSchemaFull, &e.DataSchemaCompact, &e.DataFull, &e.DataCompact, &authorization,
	)
	if err == sql.ErrNoRows {
		return FeedEvent{}, false, nil
	}
	if err != nil {
		return FeedEvent{}, false, fmt.Errorf("EVENTFEED-FINDBYID-SCAN: %w", err)
	}
	if err = decodeAuthorizationAASIDs(&e, authorization); err != nil {
		return FeedEvent{}, false, err
	}
	e.PublishSeq = publishSeq.Int64
	return e, true, nil
}

// FindPage returns up to q.Limit+1 events matching q, in the given
// presentation. The caller uses the extra record to detect whether more
// pages remain.
func (r *Repository) FindPage(ctx context.Context, q domainQuery, presentation Presentation) ([]FeedEvent, error) {
	ds, err := r.pageDataset(q, presentation)
	if err != nil {
		return nil, err
	}
	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("EVENTFEED-FINDPAGE-BUILDSQL: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("EVENTFEED-FINDPAGE-QUERY: %w", err)
	}
	defer func() { _ = rows.Close() }()
	events := make([]FeedEvent, 0, q.Limit+1)
	for rows.Next() {
		event, err := scanPageEvent(rows, presentation)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("EVENTFEED-FINDPAGE-ROWS: %w", err)
	}
	return events, nil
}

func scanPageEvent(rows *sql.Rows, presentation Presentation) (FeedEvent, error) {
	var event FeedEvent
	var schema, data string
	var authorization []byte
	if err := rows.Scan(&event.PublishSeq, &event.ID, &event.Type, &event.Subject, &event.Source, &event.Time,
		&schema, &data, &authorization); err != nil {
		return FeedEvent{}, fmt.Errorf("EVENTFEED-FINDPAGE-SCAN: %w", err)
	}
	if err := decodeAuthorizationAASIDs(&event, authorization); err != nil {
		return FeedEvent{}, err
	}
	if presentation == PresentationCompact {
		event.DataSchemaCompact, event.DataCompact = schema, data
	} else {
		event.DataSchemaFull, event.DataFull = schema, data
	}
	return event, nil
}

func (r *Repository) pageDataset(q domainQuery, presentation Presentation) (*goqu.SelectDataset, error) {
	compact := presentation == PresentationCompact
	schemaCol := "dataschema_full"
	dataCol := "data_full"
	if compact {
		schemaCol = "dataschema_compact"
		dataCol = "data_compact"
	}
	ds := r.dialect.From("feed_events").
		Select("publish_seq", "id", "event_type", "subject", "source", "time",
			goqu.C(schemaCol), goqu.C(dataCol), "authorization_aas_ids")

	retentionFloor := r.now().Add(-r.maxAge)
	ds = ds.Where(goqu.C("time").Gte(retentionFloor))

	// Only rows that have been assigned a publish_seq (i.e. whose writer
	// transaction was already visible to a RunPublishAssignment pass) are
	// ever returned - see database/patches/1_2_0.sql.
	ds = ds.Where(goqu.C("publish_seq").IsNotNull())
	if q.AfterSeq > 0 {
		ds = ds.Where(goqu.C("publish_seq").Gt(q.AfterSeq))
	}
	if q.Since != nil {
		ds = ds.Where(goqu.C("time").Gte(*q.Since))
	}
	if q.Filter != nil {
		expr, err := q.Filter.expression(presentation)
		if err != nil {
			return nil, err
		}
		ds = ds.Where(expr)
	}

	ds = ds.Order(goqu.C("publish_seq").Asc()).Limit(uint(q.Limit + 1))

	return ds, nil
}

// AssignPublishSeq publishes a committed batch atomically on the connection
// holding the transaction advisory lock. The cursor sequence is allocated only
// after writer transactions are visible, so late commits remain discoverable.
func (r *Repository) AssignPublishSeq(ctx context.Context, batchSize int) (int64, error) {
	if batchSize < 1 {
		return 0, fmt.Errorf("EVENTFEED-PUBLISH-BATCH batch size must be positive")
	}
	return r.withTransactionLock(ctx, publishLockKey, "EVENTFEED-PUBLISH", func(tx *sql.Tx) (int64, error) {
		candidates := r.dialect.From("feed_events").Select("seq").
			Where(goqu.C("publish_seq").IsNull()).Order(goqu.C("seq").Asc()).Limit(uint(batchSize))
		assignments := r.dialect.From("candidates").Select("seq",
			goqu.Func("nextval", "feed_events_publish_seq_seq").As("publish_seq")).Order(goqu.C("seq").Asc())
		query, args, err := r.dialect.Update("feed_events").With("candidates", candidates).
			With("assignments", assignments).From("assignments").
			Set(goqu.Record{"publish_seq": goqu.I("assignments.publish_seq")}).
			Where(goqu.I("feed_events.seq").Eq(goqu.I("assignments.seq"))).ToSQL()
		if err != nil {
			return 0, fmt.Errorf("EVENTFEED-PUBLISH-BUILDSQL: %w", err)
		}
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return 0, fmt.Errorf("EVENTFEED-PUBLISH-ASSIGN: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("EVENTFEED-PUBLISH-COUNT: %w", err)
		}
		return count, nil
	})
}

func (r *Repository) withTransactionLock(ctx context.Context, key int64, errPrefix string, fn func(*sql.Tx) (int64, error)) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%s-BEGIN: %w", errPrefix, err)
	}
	defer func() { _ = tx.Rollback() }()
	query, args, err := r.dialect.Select(goqu.Func("pg_try_advisory_xact_lock", key)).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("%s-LOCK-BUILDSQL: %w", errPrefix, err)
	}
	var locked bool
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&locked); err != nil {
		return 0, fmt.Errorf("%s-LOCK: %w", errPrefix, err)
	}
	if !locked {
		return 0, nil
	}
	count, err := fn(tx)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("%s-COMMIT: %w", errPrefix, err)
	}
	return count, nil
}

// DeleteOlderThan commits each bounded deletion batch before acquiring the next
// transaction lock. Cancellation rolls back only the active batch; the returned
// count includes all earlier batches already committed by this worker.
func (r *Repository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	for {
		count, err := r.withTransactionLock(ctx, retentionLockKey, "EVENTFEED-RETENTION", func(tx *sql.Tx) (int64, error) {
			return r.deleteExpiredBatch(ctx, tx, cutoff)
		})
		if err != nil {
			return total, err
		}
		total += count
		if count < int64(retentionBatchSize) {
			return total, nil
		}
	}
}

func (r *Repository) deleteExpiredBatch(ctx context.Context, tx *sql.Tx, cutoff time.Time) (int64, error) {
	query, args, err := r.dialect.Delete("feed_events").
		Where(goqu.C("seq").In(r.dialect.From("feed_events").Select("seq").
			Where(goqu.C("time").Lt(cutoff.UTC())).Order(goqu.C("seq").Asc()).Limit(uint(retentionBatchSize)))).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("EVENTFEED-DELETE-BUILDSQL: %w", err)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("EVENTFEED-DELETE-EXEC: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("EVENTFEED-DELETE-COUNT: %w", err)
	}
	return count, nil
}

func filterExpression(column string, cmp comparison) (goqu.Expression, error) {
	switch cmp.Operator {
	case "==":
		if len(cmp.Values) != 1 {
			return nil, newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
		}
		return goqu.C(column).Eq(cmp.Values[0]), nil
	case "!=":
		if len(cmp.Values) != 1 {
			return nil, newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
		}
		return goqu.C(column).Neq(cmp.Values[0]), nil
	case "=in=":
		vals := make([]any, len(cmp.Values))
		for i, v := range cmp.Values {
			vals[i] = v
		}
		return goqu.C(column).In(vals...), nil
	case "=out=":
		vals := make([]any, len(cmp.Values))
		for i, v := range cmp.Values {
			vals[i] = v
		}
		return goqu.C(column).NotIn(vals...), nil
	default:
		return nil, newQueryError("EVENTFEED-FILTER-MALFORMED", "malformed RSQL filter expression")
	}
}

func decodeAuthorizationAASIDs(event *FeedEvent, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &event.AuthorizationAASIDs); err != nil {
		return fmt.Errorf("EVENTFEED-READ-AUTHJSON: %w", err)
	}
	return nil
}
