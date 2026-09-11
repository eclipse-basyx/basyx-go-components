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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/stretchr/testify/require"
)

func newFeedDatabaseTest(t *testing.T, poolSize int) (*sql.DB, *eventfeed.Repository, *eventfeed.Service) {
	t.Helper()
	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	db.SetMaxOpenConns(poolSize)
	t.Cleanup(func() { _ = db.Close() })
	cfg := eventfeed.DefaultConfig()
	cfg.Enabled = true
	repo := eventfeed.NewRepository(db, cfg.MaxAge)
	return db, repo, eventfeed.NewService(repo, cfg)
}

func newDatabaseFeedEvent(t *testing.T, suffix string) eventfeed.FeedEvent {
	t.Helper()
	id := fmt.Sprintf("db-%s-%d", suffix, time.Now().UnixNano())
	return eventfeed.FeedEvent{
		ID: id, Subject: id, Type: eventfeed.TypeAASCreated, Source: "urn:database-test",
		DataSchemaFull: "urn:schema:regular", DataSchemaCompact: "urn:schema:compact",
		DataFull: `{"variant":"regular"}`, DataCompact: `{"variant":"compact"}`,
		AuthorizationAASIDs: []string{},
	}
}

func TestEventFeedWorkersCompleteWithBoundedConnectionPools(t *testing.T) {
	for _, size := range []int{1, 2} {
		t.Run(fmt.Sprintf("pool-%d", size), func(t *testing.T) {
			_, repo, svc := newFeedDatabaseTest(t, size)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			event := newDatabaseFeedEvent(t, "pool")
			require.NoError(t, repo.Save(ctx, event))
			var workers sync.WaitGroup
			errors := make(chan error, 2)
			for _, run := range []func(context.Context) (int64, error){svc.RunPublishAssignment, svc.RunRetention} {
				workers.Go(func() { _, err := run(ctx); errors <- err })
			}
			workers.Wait()
			close(errors)
			for err := range errors {
				require.NoError(t, err)
			}
			publishUntilRecordSeen(ctx, t, svc, eventfeed.FeedQuery{Limit: 1, Filter: "rsql:event.subject==" + event.Subject}, event.ID)
		})
	}
}

func TestEventFeedRollbackAndCancellationLeaveWorkersUsable(t *testing.T) {
	db, repo, svc := newFeedDatabaseTest(t, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	event := newDatabaseFeedEvent(t, "rollback")
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = repo.SaveTx(ctx, tx, event)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	_, found, err := repo.FindByID(ctx, event.ID)
	require.NoError(t, err)
	require.False(t, found)
	canceled, stop := context.WithCancel(ctx)
	stop()
	_, err = svc.RunPublishAssignment(canceled)
	require.ErrorIs(t, err, context.Canceled)
	_, err = svc.RunRetention(canceled)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, repo.Save(ctx, event))
	publishUntilRecordSeen(ctx, t, svc, eventfeed.FeedQuery{Limit: 1, Filter: "rsql:event.subject==" + event.Subject}, event.ID)
}

func setDatabaseEventTime(t *testing.T, db *sql.DB, eventID string, occurred time.Time) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").Update("feed_events").
		Set(goqu.Record{"time": occurred}).Where(goqu.C("id").Eq(eventID)).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
}

func TestEventFeedSinceCursorRetainsFilterAndCompactPresentation(t *testing.T) {
	db, repo, svc := newFeedDatabaseTest(t, 2)
	since := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	events := []eventfeed.FeedEvent{newDatabaseFeedEvent(t, "since-a"), newDatabaseFeedEvent(t, "since-b"), newDatabaseFeedEvent(t, "since-old")}
	for _, event := range events {
		require.NoError(t, repo.Save(t.Context(), event))
	}
	setDatabaseEventTime(t, db, events[2].ID, since.Add(-time.Minute))
	filter := fmt.Sprintf("rsql:(event.subject==%s,event.subject==%s,event.subject==%s);event.type==%s;event.source!=excluded;event.dataschema=in=(urn:schema:compact)", events[0].Subject, events[1].Subject, events[2].Subject, eventfeed.TypeAASCreated)
	query := eventfeed.FeedQuery{Since: &since, Filter: filter, Presentation: eventfeed.PresentationCompact, Limit: 1}
	publishUntilRecordSeen(t.Context(), t, svc, eventfeed.FeedQuery{Filter: "rsql:event.subject==" + events[2].Subject, Limit: 1}, events[2].ID)
	first := publishUntilRecordSeen(t.Context(), t, svc, query, events[0].ID)
	require.NotEmpty(t, first.Cursor)
	second, err := svc.Read(t.Context(), eventfeed.FeedQuery{Cursor: first.Cursor, Limit: 1})
	require.NoError(t, err)
	require.Len(t, second.Records, 1)
	require.Equal(t, events[1].ID, second.Records[0].ID)
	require.Equal(t, "compact", second.Records[0].Data["variant"])
	require.Empty(t, second.Cursor)
	query.Cursor = first.Cursor
	repeated, err := svc.Read(t.Context(), query)
	require.NoError(t, err)
	require.Equal(t, second.Records, repeated.Records)
}

func TestEventFeedChronologicalPagesKeepDurableCursorAndRetention(t *testing.T) {
	db, repo, svc := newFeedDatabaseTest(t, 2)
	events := []eventfeed.FeedEvent{newDatabaseFeedEvent(t, "time-a"), newDatabaseFeedEvent(t, "time-b"), newDatabaseFeedEvent(t, "time-c")}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i, event := range events {
		require.NoError(t, repo.Save(t.Context(), event))
		setDatabaseEventTime(t, db, event.ID, now.Add(-time.Duration(i)*time.Minute))
	}
	filter := fmt.Sprintf("rsql:event.subject=in=(%s,%s,%s)", events[0].Subject, events[1].Subject, events[2].Subject)
	publishUntilRecordSeen(t.Context(), t, svc, eventfeed.FeedQuery{Filter: "rsql:event.subject==" + events[2].Subject, Limit: 1}, events[2].ID)
	first, err := svc.Read(t.Context(), eventfeed.FeedQuery{Filter: filter, Limit: 2})
	require.NoError(t, err)
	require.Len(t, first.Records, 2)
	require.Equal(t, events[1].ID, first.Records[0].ID)
	require.Equal(t, events[0].ID, first.Records[1].ID)
	require.Equal(t, now, first.Updated)
	second, err := svc.Read(t.Context(), eventfeed.FeedQuery{Cursor: first.Cursor, Limit: 2})
	require.NoError(t, err)
	require.Len(t, second.Records, 1)
	require.Equal(t, events[2].ID, second.Records[0].ID)
	require.Empty(t, second.Cursor)
	resume, err := svc.Read(t.Context(), eventfeed.FeedQuery{LastEventID: first.Records[1].ID, Filter: filter, Limit: 3})
	require.NoError(t, err)
	require.Len(t, resume.Records, 2)
	require.Equal(t, events[2].ID, resume.Records[0].ID)
	require.Equal(t, events[1].ID, resume.Records[1].ID)
	setDatabaseEventTime(t, db, events[0].ID, now.Add(-90*24*time.Hour))
	_, err = svc.RunRetention(t.Context())
	require.NoError(t, err)
	_, found, err := repo.FindByID(t.Context(), events[0].ID)
	require.NoError(t, err)
	require.False(t, found)
	for _, event := range events[1:] {
		_, found, err = repo.FindByID(t.Context(), event.ID)
		require.NoError(t, err)
		require.True(t, found)
	}
}

func TestEventFeedPublicationAcrossBatchBoundary(t *testing.T) {
	db, repo, svc := newFeedDatabaseTest(t, 2)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	subject := fmt.Sprintf("batch-%d", time.Now().UnixNano())
	const count = 503
	var lastID string
	for i := range count {
		event := newDatabaseFeedEvent(t, fmt.Sprintf("batch-%d", i))
		event.Subject = subject
		_, err = repo.SaveTx(ctx, tx, event)
		require.NoError(t, err)
		lastID = event.ID
	}
	require.NoError(t, tx.Commit())
	require.Eventually(t, func() bool {
		_, err = svc.RunPublishAssignment(ctx)
		require.NoError(t, err)
		event, found, err := repo.FindByID(ctx, lastID)
		require.NoError(t, err)
		return found && event.PublishSeq != 0
	}, 5*time.Second, 10*time.Millisecond)
	query, args, err := goqu.Dialect("postgres").From("feed_events").Select("publish_seq").
		Where(goqu.C("subject").Eq(subject)).Order(goqu.C("seq").Asc()).ToSQL()
	require.NoError(t, err)
	rows, err := db.QueryContext(ctx, query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	previous, actual := int64(0), 0
	for rows.Next() {
		var sequence int64
		require.NoError(t, rows.Scan(&sequence))
		require.Greater(t, sequence, previous)
		previous = sequence
		actual++
	}
	require.NoError(t, rows.Err())
	require.Equal(t, count, actual)
}
