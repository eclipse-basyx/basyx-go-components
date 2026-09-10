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

//nolint:all
package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

// TestAASRepositoryEventFeedInterleavedCommitsAreNotPermanentlyLost reproduces
// the exact race a reviewer flagged against BIGSERIAL-based cursors: a
// transaction (A) mints the lowest seq but stays open while later
// transactions (B, C) mint higher seqs and commit first. A naive
// "cursor = last delivered seq" design would advance the cursor past B once
// B is observed, permanently skipping A once it finally commits.
//
// The fix assigns a SEPARATE publish_seq to each row only once it is
// already visible (Repository.AssignPublishSeq / Service.RunPublishAssignment),
// and cursors/lastEventId are based on publish_seq, not the raw seq. Since a
// still-open transaction's row is invisible to that assignment scan, it
// simply cannot receive a publish_seq before it commits - there is no
// window in which a cursor can move past it.
//
// This test drives the eventfeed package directly against the live
// integration Postgres (bypassing the running HTTP server so the assignment
// job can be run on demand instead of waiting for its ticker) and asserts
// that A is still delivered once it commits, never silently dropped - and,
// as a bonus, that delivery ends up in true commit order (B, C, A) even
// though A minted the lowest raw seq.
func TestAASRepositoryEventFeedInterleavedCommitsAreNotPermanentlyLost(t *testing.T) {
	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	cfg := eventfeed.DefaultConfig()
	cfg.Enabled = true
	require.NoError(t, cfg.Validate())
	repo := eventfeed.NewRepository(db, cfg.MaxAge)
	svc := eventfeed.NewService(repo, cfg)

	stamp := time.Now().UnixNano()
	subjectA := fmt.Sprintf("urn:example:event-feed:race:a:%d", stamp)
	subjectB := fmt.Sprintf("urn:example:event-feed:race:b:%d", stamp)
	subjectC := fmt.Sprintf("urn:example:event-feed:race:c:%d", stamp)
	filter := fmt.Sprintf("rsql:event.subject=in=(%s,%s,%s)", subjectA, subjectB, subjectC)

	newEvent := func(id, subject string) eventfeed.FeedEvent {
		return eventfeed.FeedEvent{
			ID:                id,
			Type:              eventfeed.TypeAASCreated,
			Subject:           subject,
			Source:            "http://localhost/test",
			DataSchemaFull:    "https://admin-shell.io/events/schemas/aas-created-full.json",
			DataSchemaCompact: "https://admin-shell.io/events/schemas/aas-created-compact.json",
			DataFull:          `{"aasId":"` + subject + `"}`,
			DataCompact:       `{"aasId":"` + subject + `"}`,
		}
	}

	ctx := context.Background()

	// A mints the lowest seq first, but does not commit yet - simulating a
	// stalled writer transaction.
	txA, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	committedA := false
	t.Cleanup(func() {
		if !committedA {
			_ = txA.Rollback()
		}
	})
	eventA, err := repo.SaveTx(ctx, txA, newEvent(fmt.Sprintf("race-a-%d", stamp), subjectA))
	require.NoError(t, err)

	// B and C mint higher seqs and commit immediately, exactly as in the
	// reviewer's PostgreSQL repro (B inserts seq=2 and commits while A, at
	// seq=1, is still open).
	txB, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	eventB, err := repo.SaveTx(ctx, txB, newEvent(fmt.Sprintf("race-b-%d", stamp), subjectB))
	require.NoError(t, err)
	require.NoError(t, txB.Commit())

	txC, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	eventC, err := repo.SaveTx(ctx, txC, newEvent(fmt.Sprintf("race-c-%d", stamp), subjectC))
	require.NoError(t, err)
	require.NoError(t, txC.Commit())

	require.Less(t, eventA.Seq, eventB.Seq)
	require.Less(t, eventB.Seq, eventC.Seq)

	query := eventfeed.FeedQuery{Filter: filter, Limit: 1, Presentation: eventfeed.PresentationRegular}

	// A is still open and was never assigned a publish_seq, so it cannot
	// appear - and, crucially, the cursor issued here is based on B's
	// publish_seq, not A's (still nonexistent) one, so nothing is skipped.
	// AssignPublishSeq batches table-wide (500 rows/call, shared with other
	// tests in this suite), so repeat it until B surfaces rather than
	// assuming one call covers our rows.
	firstPage := publishUntilRecordSeen(ctx, t, svc, query, eventB.ID)
	require.NotEmpty(t, firstPage.Cursor)

	// A finally resolves.
	require.NoError(t, txA.Commit())
	committedA = true

	// Resuming from B's cursor must surface C next, then A last - true
	// commit order, and A is not permanently skipped just because it had
	// the lowest raw seq.
	query.Cursor = firstPage.Cursor
	page2 := publishUntilRecordSeen(ctx, t, svc, query, eventC.ID)
	require.NotEmpty(t, page2.Cursor)

	// Now A is visible too. The assignment job picks it up and gives it a
	// publish_seq after B's and C's - not seq order, commit order.
	query.Cursor = page2.Cursor
	page3 := publishUntilRecordSeen(ctx, t, svc, query, eventA.ID)
	require.Empty(t, page3.Cursor, "no more matching events left")
}

// publishUntilRecordSeen repeatedly runs the publish_seq assignment job and
// re-reads query until its single result is the event identified by
// wantID, or fails the test after 5s. AssignPublishSeq only ever processes
// a bounded batch per call and is shared with whatever else is writing to
// this database, so a row is not guaranteed to be published on the first
// call - this must not be confused with the correctness guarantee under
// test (that a committed row is never permanently skipped), which does not
// depend on how many calls it takes.
func publishUntilRecordSeen(ctx context.Context, t *testing.T, svc *eventfeed.Service, query eventfeed.FeedQuery, wantID string) eventfeed.FeedResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := svc.RunPublishAssignment(ctx)
		require.NoError(t, err)

		resp, err := svc.Read(ctx, query)
		require.NoError(t, err)
		if len(resp.Records) == 1 && resp.Records[0].ID == wantID {
			return resp
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected event %s not observed before deadline; got %+v", wantID, resp.Records)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
