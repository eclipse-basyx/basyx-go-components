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

// Repository persists and queries event feed records.
type Repository struct {
	db      *sql.DB
	dialect goqu.DialectWrapper
	maxAge  time.Duration
	now     func() time.Time
}

// NewRepository creates an event feed repository backed by db.
func NewRepository(db *sql.DB, maxAge time.Duration) *Repository {
	return &Repository{
		db:      db,
		dialect: goqu.Dialect("postgres"),
		maxAge:  maxAge,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Save persists event.
func (r *Repository) Save(ctx context.Context, event FeedEvent) error {
	query, args, err := r.dialect.Insert("feed_events").Rows(goqu.Record{
		"id":                 event.ID,
		"event_type":         event.Type,
		"subject":            event.Subject,
		"source":             event.Source,
		"time":               event.Time.UTC(),
		"dataschema_full":    event.DataSchemaFull,
		"dataschema_compact": event.DataSchemaCompact,
		"data_full":          goqu.L("?::jsonb", event.DataFull),
		"data_compact":       goqu.L("?::jsonb", event.DataCompact),
	}).ToSQL()
	if err != nil {
		return fmt.Errorf("EVENTFEED-SAVE-BUILDSQL: %w", err)
	}
	if _, err = r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("EVENTFEED-SAVE-EXEC: %w", err)
	}
	return nil
}

// FindByID finds an event by its identifier.
func (r *Repository) FindByID(ctx context.Context, id string) (FeedEvent, bool, error) {
	query, args, err := r.dialect.From("feed_events").
		Select("id", "event_type", "subject", "source", "time",
			"dataschema_full", "dataschema_compact", "data_full", "data_compact").
		Where(goqu.C("id").Eq(id)).
		ToSQL()
	if err != nil {
		return FeedEvent{}, false, fmt.Errorf("EVENTFEED-FINDBYID-BUILDSQL: %w", err)
	}
	var e FeedEvent
	err = r.db.QueryRowContext(ctx, query, args...).Scan(
		&e.ID, &e.Type, &e.Subject, &e.Source, &e.Time,
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

// FindPage returns one ordered page of retained events.
func (r *Repository) FindPage(ctx context.Context, q domainQuery, presentation Presentation) ([]FeedEvent, error) {
	ds := r.dialect.From("feed_events").
		Select("id", "event_type", "subject", "source", "time",
			"dataschema_full", "dataschema_compact", "data_full", "data_compact")

	retentionFloor := r.now().Add(-r.maxAge)
	ds = ds.Where(goqu.C("time").Gte(retentionFloor))

	if q.AfterID != "" && q.AfterTime != nil {
		ds = ds.Where(goqu.Or(
			goqu.C("time").Gt(*q.AfterTime),
			goqu.And(goqu.C("time").Eq(*q.AfterTime), goqu.C("id").Gt(q.AfterID)),
		))
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

	ds = ds.Order(goqu.C("time").Asc(), goqu.C("id").Asc()).Limit(uint(q.Limit + 1))
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
		if err = rows.Scan(
			&e.ID, &e.Type, &e.Subject, &e.Source, &e.Time,
			&e.DataSchemaFull, &e.DataSchemaCompact, &e.DataFull, &e.DataCompact,
		); err != nil {
			return nil, fmt.Errorf("EVENTFEED-FINDPAGE-SCAN: %w", err)
		}
		events = append(events, e)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("EVENTFEED-FINDPAGE-ROWS: %w", err)
	}
	return events, nil
}

// DeleteOlderThan deletes events created before cutoff.
func (r *Repository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	query, args, err := r.dialect.Delete("feed_events").
		Where(goqu.C("time").Lt(cutoff.UTC())).
		ToSQL()
	if err != nil {
		return 0, fmt.Errorf("EVENTFEED-DELETE-BUILDSQL: %w", err)
	}
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("EVENTFEED-DELETE-EXEC: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
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
