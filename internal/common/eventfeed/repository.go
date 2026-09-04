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
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
)

const (
	retentionBatchSize = 1000
	retentionLockKey   = int64(6471200)
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
	query, args, err := r.dialect.Insert("feed_events").Rows(goqu.Record{
		"id":                 event.ID,
		"event_type":         event.Type,
		"subject":            event.Subject,
		"source":             event.Source,
		"dataschema_full":    event.DataSchemaFull,
		"dataschema_compact": event.DataSchemaCompact,
		"data_full":          goqu.L("?::jsonb", event.DataFull),
		"data_compact":       goqu.L("?::jsonb", event.DataCompact),
	}).Returning("seq", "time").ToSQL()
	if err != nil {
		return FeedEvent{}, fmt.Errorf("EVENTFEED-SAVE-BUILDSQL: %w", err)
	}
	if err = exec.QueryRowContext(ctx, query, args...).Scan(&event.Seq, &event.Time); err != nil {
		return FeedEvent{}, fmt.Errorf("EVENTFEED-SAVE-EXEC: %w", err)
	}
	return event, nil
}

// FindByID looks up a single event by its CloudEvents id. found is false if no such event exists.
func (r *Repository) FindByID(ctx context.Context, id string) (FeedEvent, bool, error) {
	query, args, err := r.dialect.From("feed_events").
		Select("seq", "id", "event_type", "subject", "source", "time",
			"dataschema_full", "dataschema_compact", "data_full", "data_compact").
		Where(goqu.C("id").Eq(id)).
		ToSQL()
	if err != nil {
		return FeedEvent{}, false, fmt.Errorf("EVENTFEED-FINDBYID-BUILDSQL: %w", err)
	}
	var e FeedEvent
	err = r.db.QueryRowContext(ctx, query, args...).Scan(
		&e.Seq, &e.ID, &e.Type, &e.Subject, &e.Source, &e.Time,
		&e.DataSchemaFull, &e.DataSchemaCompact, &e.DataFull, &e.DataCompact,
	)
	if err == sql.ErrNoRows {
		return FeedEvent{}, false, nil
	}
	if err != nil {
		return FeedEvent{}, false, fmt.Errorf("EVENTFEED-FINDBYID-SCAN: %w", err)
	}
	return e, true, nil
}

// FindPage returns up to q.Limit+1 events matching q, in the given
// presentation. The caller uses the extra record to detect whether more
// pages remain.
func (r *Repository) FindPage(ctx context.Context, q domainQuery, presentation Presentation) ([]FeedEvent, error) {
	compact := presentation == PresentationCompact
	schemaCol := "dataschema_full"
	dataCol := "data_full"
	if compact {
		schemaCol = "dataschema_compact"
		dataCol = "data_compact"
	}
	ds := r.dialect.From("feed_events").
		Select("seq", "id", "event_type", "subject", "source", "time",
			goqu.C(schemaCol), goqu.C(dataCol))

	retentionFloor := r.now().Add(-r.maxAge)
	ds = ds.Where(goqu.C("time").Gte(retentionFloor))

	if q.AfterSeq > 0 {
		ds = ds.Where(goqu.C("seq").Gt(q.AfterSeq))
	}
	if q.Since != nil {
		ds = ds.Where(goqu.C("time").Gte(*q.Since))
	}
	if q.Filter != nil {
		for _, cmp := range q.Filter.Comparisons {
			col, err := columnForField(cmp.Field, presentation)
			if err != nil {
				return nil, err
			}
			expr, err := filterExpression(col, cmp)
			if err != nil {
				return nil, err
			}
			ds = ds.Where(expr)
		}
	}

	ds = ds.Order(goqu.C("seq").Asc()).Limit(uint(q.Limit + 1))
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
		var e FeedEvent
		var schema, data string
		if err = rows.Scan(
			&e.Seq, &e.ID, &e.Type, &e.Subject, &e.Source, &e.Time,
			&schema, &data,
		); err != nil {
			return nil, fmt.Errorf("EVENTFEED-FINDPAGE-SCAN: %w", err)
		}
		if compact {
			e.DataSchemaCompact = schema
			e.DataCompact = data
		} else {
			e.DataSchemaFull = schema
			e.DataFull = data
		}
		events = append(events, e)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("EVENTFEED-FINDPAGE-ROWS: %w", err)
	}
	return events, nil
}

// TryRetentionLock acquires a session advisory lock so only one replica cleans up.
func (r *Repository) TryRetentionLock(ctx context.Context) (bool, error) {
	var locked bool
	if err := r.db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", retentionLockKey).Scan(&locked); err != nil {
		return false, fmt.Errorf("EVENTFEED-RETENTION-LOCK: %w", err)
	}
	return locked, nil
}

// ReleaseRetentionLock releases the advisory lock acquired by TryRetentionLock.
func (r *Repository) ReleaseRetentionLock(ctx context.Context) {
	_, _ = r.db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", retentionLockKey)
}

// DeleteOlderThan deletes events created before cutoff in bounded batches.
func (r *Repository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	for {
		query, args, err := r.dialect.Delete("feed_events").
			Where(goqu.C("seq").In(
				r.dialect.From("feed_events").
					Select("seq").
					Where(goqu.C("time").Lt(cutoff.UTC())).
					Order(goqu.C("seq").Asc()).
					Limit(uint(retentionBatchSize)),
			)).
			ToSQL()
		if err != nil {
			return total, fmt.Errorf("EVENTFEED-DELETE-BUILDSQL: %w", err)
		}
		res, err := r.db.ExecContext(ctx, query, args...)
		if err != nil {
			return total, fmt.Errorf("EVENTFEED-DELETE-EXEC: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
		if n < int64(retentionBatchSize) {
			return total, nil
		}
	}
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
