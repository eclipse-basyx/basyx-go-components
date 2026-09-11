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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

func TestValidateQueryMutualExclusion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	svc := NewService(nil, cfg)
	since := time.Now().UTC().Add(-time.Hour)
	err := svc.validateQuery(FeedQuery{
		LastEventID:  "x",
		Since:        &since,
		Presentation: PresentationRegular,
		Limit:        10,
	})
	if !IsQueryError(err) {
		t.Fatalf("expected query error, got %v", err)
	}
}

func TestCapabilities(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.MaxPageSize = 50
	svc := NewService(nil, cfg)
	caps := svc.Capabilities()
	if caps.APIVersion != "1.0" {
		t.Fatalf("apiVersion=%s", caps.APIVersion)
	}
	if len(caps.EventTypes) != len(allEventTypes()) {
		t.Fatalf("eventTypes=%d", len(caps.EventTypes))
	}
	for _, eventType := range []string{TypeAssetDeleted, TypeAASDeleted, TypeSubmodelDeleted, TypePCN} {
		if _, ok := caps.EventTypes[eventType]; !ok {
			t.Fatalf("missing event type %s", eventType)
		}
	}
	if caps.MaxPageSize != 50 {
		t.Fatalf("maxPageSize=%d", caps.MaxPageSize)
	}
	if caps.Presentation.Default != "REGULAR" {
		t.Fatalf("default presentation=%s", caps.Presentation.Default)
	}
	if caps.Auth.Inherited != true {
		t.Fatal("expected inherited auth")
	}
}

func TestReadWithSQLMock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.MaxAge = 30 * 24 * time.Hour
	repo := NewRepository(db, cfg.MaxAge)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }
	svc := NewService(repo, cfg)
	svc.now = func() time.Time { return fixedNow }

	ts1 := fixedNow.Add(-2 * time.Hour)
	ts2 := fixedNow.Add(-1 * time.Hour)
	rows := sqlmock.NewRows([]string{
		"publish_seq", "id", "event_type", "subject", "source", "time",
		"dataschema_compact", "data_compact", "authorization_aas_ids",
	}).
		AddRow(int64(1), "e1", TypeAASCreated, "aas-1", "http://localhost/shells", ts1,
			"https://s/compact", `{"aasId":"aas-1"}`, nil).
		AddRow(int64(2), "e2", TypeAASUpdated, "aas-1", "http://localhost/shells", ts2,
			"https://s/compact", `{"aasId":"aas-1"}`, nil).
		AddRow(int64(3), "e3", TypeAASUpdated, "aas-2", "http://localhost/shells", ts2.Add(time.Minute),
			"https://s/compact", `{"aasId":"aas-2"}`, nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(rows)

	result, err := svc.Read(context.Background(), FeedQuery{
		Presentation: PresentationCompact,
		Limit:        2,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("records=%d", len(result.Records))
	}
	if result.Cursor == "" {
		t.Fatal("expected cursor for hasMore")
	}
	if result.Records[0].SpecVersion != "1.0" {
		t.Fatalf("specversion=%s", result.Records[0].SpecVersion)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

// TestReadLastEventIDRejectsUnpublishedEvent proves that resuming via
// lastEventId is gated the same way as the cursor: an event that exists but
// has not yet been assigned a publish_seq (PublishSeq == 0, i.e. its writer
// transaction has not yet been picked up by RunPublishAssignment) must not
// be usable as a resume point - a client naively using the id of the event
// it just received could otherwise skip an earlier-committing, not-yet-seen
// event forever.
func TestReadLastEventIDRejectsUnpublishedEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	repo := NewRepository(db, cfg.MaxAge)
	svc := NewService(repo, cfg)

	rows := sqlmock.NewRows([]string{
		"seq", "publish_seq", "id", "event_type", "subject", "source", "time",
		"dataschema_full", "dataschema_compact", "data_full", "data_compact", "authorization_aas_ids",
	}).AddRow(int64(5), nil, "e5", TypeAASCreated, "aas-1", "http://localhost/shells", time.Now().UTC(),
		"https://s/full", "https://s/compact", `{}`, `{}`, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	_, err = svc.Read(context.Background(), FeedQuery{
		LastEventID:  "e5",
		Presentation: PresentationRegular,
		Limit:        10,
	})
	if !IsQueryError(err) {
		t.Fatalf("expected query error for unpublished lastEventId, got %v", err)
	}
}

func TestHTTPHandlers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	repo := NewRepository(db, cfg.MaxAge)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }
	svc := NewService(repo, cfg)
	svc.now = func() time.Time { return fixedNow }

	r := chi.NewRouter()
	RegisterRoutes(r, svc)

	rows := sqlmock.NewRows([]string{
		"publish_seq", "id", "event_type", "subject", "source", "time",
		"dataschema_full", "data_full", "authorization_aas_ids",
	}).AddRow(int64(1), "e1", TypeAASCreated, "aas-1", "http://localhost/shells", fixedNow.Add(-time.Hour),
		"https://s/full", `{"aasId":"aas-1"}`, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/events?limit=10", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", rr.Code, rr.Body.String())
	}
	var feed FeedResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &feed); err != nil {
		t.Fatalf("feed json: %v", err)
	}
	if len(feed.Records) != 1 {
		t.Fatalf("records=%d", len(feed.Records))
	}
	if feed.Cursor != "" {
		t.Fatalf("unexpected cursor on last page: %s", feed.Cursor)
	}

	capReq := httptest.NewRequest(http.MethodGet, "/.well-known/event-feed.json", nil)
	capRR := httptest.NewRecorder()
	r.ServeHTTP(capRR, capReq)
	if capRR.Code != http.StatusOK {
		t.Fatalf("capabilities status=%d body=%s", capRR.Code, capRR.Body.String())
	}
}

func TestHTTPValidationErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	svc := NewService(NewRepository(nil, cfg.MaxAge), cfg)
	r := chi.NewRouter()
	RegisterRoutes(r, svc)

	req := httptest.NewRequest(http.MethodGet, "/events?limit=0", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
	var body []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v body=%s", err, rr.Body.String())
	}
	if len(body) != 1 {
		t.Fatalf("expected one error message, got %d", len(body))
	}
	if body[0]["messageType"] != "Error" {
		t.Fatalf("messageType=%v", body[0]["messageType"])
	}
	if body[0]["code"] != "400" {
		t.Fatalf("code=%v", body[0]["code"])
	}
	if _, ok := body[0]["correlationId"].(string); !ok || body[0]["correlationId"] == "" {
		t.Fatalf("missing correlationId: %v", body[0]["correlationId"])
	}
}

func TestSaveAndRetentionSQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	repo := NewRepository(db, cfg.MaxAge)
	svc := NewService(repo, cfg)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }
	repo.now = func() time.Time { return fixedNow }

	ev, err := svc.Builder().AASCreated("aas-1", "asset-1", nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO`)).
		WillReturnRows(sqlmock.NewRows([]string{"seq", "time"}).AddRow(int64(1), fixedNow))
	if err := svc.Write(context.Background(), ev); err != nil {
		t.Fatalf("write: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_xact_lock`)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM`)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()
	n, err := svc.RunRetention(context.Background())
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	if n != 3 {
		t.Fatalf("deleted=%d", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

func TestRunPublishAssignmentSQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	repo := NewRepository(db, cfg.MaxAge)
	svc := NewService(repo, cfg)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_xact_lock`)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(`WITH candidates AS .*UPDATE "feed_events" SET "publish_seq"="assignments"."publish_seq" FROM "assignments"`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	n, err := svc.RunPublishAssignment(context.Background())
	if err != nil {
		t.Fatalf("publish assignment: %v", err)
	}
	if n != 2 {
		t.Fatalf("assigned=%d", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

func TestRegisterRoutesDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	svc := NewService(nil, cfg)
	r := chi.NewRouter()
	RegisterRoutes(r, svc)

	for _, path := range []string{"/events", "/.well-known/event-feed.json"} {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d", path, rr.Code)
		}
	}
}

func TestHTTPOmittedLimitUsesMaxPageSize(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.MaxPageSize = 50
	repo := NewRepository(db, cfg.MaxAge)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }
	svc := NewService(repo, cfg)
	svc.now = func() time.Time { return fixedNow }

	r := chi.NewRouter()
	RegisterRoutes(r, svc)

	rows := sqlmock.NewRows([]string{
		"publish_seq", "id", "event_type", "subject", "source", "time",
		"dataschema_full", "data_full", "authorization_aas_ids",
	})
	mock.ExpectQuery(`LIMIT 51`).WillReturnRows(rows)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/events", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

func TestReadHidesUnauthorizedSubjects(t *testing.T) {
	t.Cleanup(func() { SetRecordAuthorizer(nil) })
	SetRecordAuthorizer(denySubjectAuthorizer{deny: "hidden"})

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	repo := NewRepository(db, cfg.MaxAge)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }
	svc := NewService(repo, cfg)
	svc.now = func() time.Time { return fixedNow }

	rows := sqlmock.NewRows([]string{
		"publish_seq", "id", "event_type", "subject", "source", "time",
		"dataschema_full", "data_full", "authorization_aas_ids",
	}).
		AddRow(int64(1), "e1", TypeAASCreated, "hidden", "http://localhost/shells", fixedNow.Add(-time.Hour),
			"https://s/full", `{"aasId":"hidden"}`, nil).
		AddRow(int64(2), "e2", TypeAASCreated, "visible", "http://localhost/shells", fixedNow.Add(-time.Minute),
			"https://s/full", `{"aasId":"visible"}`, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	result, err := svc.Read(context.Background(), FeedQuery{Presentation: PresentationRegular, Limit: 10})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(result.Records) != 1 || result.Records[0].Subject != "visible" {
		t.Fatalf("records=%v", result.Records)
	}
}

type denySubjectAuthorizer struct{ deny string }

func (d denySubjectAuthorizer) AllowEvent(_ context.Context, event FeedEvent) bool {
	return event.Subject != d.deny
}

type denyAllAuthorizer struct{}

func (denyAllAuthorizer) AllowEvent(context.Context, FeedEvent) bool { return false }

func TestReadKeepsCursorWhenAuthScanBudgetExhausted(t *testing.T) {
	t.Cleanup(func() { SetRecordAuthorizer(nil) })
	SetRecordAuthorizer(denyAllAuthorizer{})

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	repo := NewRepository(db, cfg.MaxAge)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }
	svc := NewService(repo, cfg)
	svc.now = func() time.Time { return fixedNow }

	for round := 0; round < authScanRounds; round++ {
		seq := int64(round*2 + 1)
		rows := sqlmock.NewRows([]string{
			"publish_seq", "id", "event_type", "subject", "source", "time",
			"dataschema_full", "data_full", "authorization_aas_ids",
		}).
			AddRow(seq, "e-hidden", TypeAASCreated, "hidden", "http://localhost/shells", fixedNow.Add(-time.Hour),
				"https://s/full", `{"aasId":"hidden"}`, nil).
			AddRow(seq+1, "e-hidden-more", TypeAASCreated, "hidden", "http://localhost/shells", fixedNow.Add(-time.Minute),
				"https://s/full", `{"aasId":"hidden"}`, nil)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)
	}

	result, err := svc.Read(context.Background(), FeedQuery{Presentation: PresentationRegular, Limit: 1})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(result.Records) != 0 {
		t.Fatalf("records=%d", len(result.Records))
	}
	if result.Cursor == "" {
		t.Fatal("expected continuation cursor after auth scan budget")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

func TestFindPageSQLShape(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewRepository(db, 30*24*time.Hour)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }

	rows := sqlmock.NewRows([]string{
		"publish_seq", "id", "event_type", "subject", "source", "time",
		"dataschema_compact", "data_compact", "authorization_aas_ids",
	})
	mock.ExpectQuery(`SELECT .*"dataschema_compact", "data_compact", "authorization_aas_ids" FROM "feed_events".*"publish_seq" IS NOT NULL.*ORDER BY "publish_seq" ASC`).
		WillReturnRows(rows)

	if _, err = repo.FindPage(context.Background(), domainQuery{Limit: 10, Filter: &parsedFilter{
		Comparisons: []comparison{{Field: "event.type", Operator: "==", Values: []string{TypeAASCreated}}},
	}}, PresentationCompact); err != nil {
		t.Fatalf("find: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

func TestCapabilitiesAdvertiseDraftOperators(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	caps := NewService(nil, cfg).Capabilities()
	want := map[string]bool{"==": true, "!=": true, "=in=": true, "=out=": true}
	if len(caps.Filter.RSQL.Operators) != len(want) {
		t.Fatalf("operators=%v", caps.Filter.RSQL.Operators)
	}
	for _, op := range caps.Filter.RSQL.Operators {
		if !want[op] {
			t.Fatalf("unexpected operator %q", op)
		}
	}
}

// TestReadCursorReachesEventAfterHiddenPrefix replays the reviewer's scenario:
// with limit=1, a prefix of hidden events longer than the authorization scan
// budget must not report end of feed. Reading again with the returned cursor
// has to reach the first readable event instead of restarting at the same
// hidden prefix.
func TestReadCursorReachesEventAfterHiddenPrefix(t *testing.T) {
	t.Cleanup(func() { SetRecordAuthorizer(nil) })
	SetRecordAuthorizer(denySubjectAuthorizer{deny: "hidden"})

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	repo := NewRepository(db, cfg.MaxAge)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }
	svc := NewService(repo, cfg)
	svc.now = func() time.Time { return fixedNow }

	feedRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{
			"publish_seq", "id", "event_type", "subject", "source", "time",
			"dataschema_full", "data_full", "authorization_aas_ids",
		})
	}

	// Every round returns limit+1 rows so the scan keeps going until the
	// budget is exhausted; seq 1..2*authScanRounds are all hidden. Only the
	// first row of each round is scanned, the extra row just signals that more
	// rows exist, so the last scanned row is the first row of the last round.
	var lastScannedSeq int64
	for round := 0; round < authScanRounds; round++ {
		seq := int64(round*2 + 1)
		lastScannedSeq = seq
		rows := feedRows().
			AddRow(seq, "e-hidden", TypeAASCreated, "hidden", "http://localhost/shells", fixedNow.Add(-time.Hour),
				"https://s/full", `{"aasId":"hidden"}`, nil).
			AddRow(seq+1, "e-hidden-more", TypeAASCreated, "hidden", "http://localhost/shells", fixedNow.Add(-time.Minute),
				"https://s/full", `{"aasId":"hidden"}`, nil)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)
	}

	first, err := svc.Read(context.Background(), FeedQuery{Presentation: PresentationRegular, Limit: 1})
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if len(first.Records) != 0 {
		t.Fatalf("records=%d want 0", len(first.Records))
	}
	if first.Cursor == "" {
		t.Fatal("expected a continuation cursor after the scan budget was exhausted")
	}
	decoded, err := decodeCursor(first.Cursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if decoded.AfterSeq != lastScannedSeq {
		t.Fatalf("cursor afterSeq=%d want %d (last scanned row)", decoded.AfterSeq, lastScannedSeq)
	}

	// Continuing from that cursor reaches the readable event instead of
	// restarting at the hidden prefix.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(
		feedRows().AddRow(lastScannedSeq+1, "e-visible", TypeAASCreated, "visible", "http://localhost/shells",
			fixedNow, "https://s/full", `{"aasId":"visible"}`, nil))

	second, err := svc.Read(context.Background(), FeedQuery{Presentation: PresentationRegular, Limit: 1, Cursor: first.Cursor})
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if len(second.Records) != 1 || second.Records[0].Subject != "visible" {
		t.Fatalf("records=%v", second.Records)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

// TestRunRetentionUnlocksAfterCancelledCleanup ensures rollback releases the
// transaction lock before the connection returns to the pool.
func TestRunRetentionUnlocksAfterCancelledCleanup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	repo := NewRepository(db, cfg.MaxAge)
	svc := NewService(repo, cfg)
	svc.now = func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_xact_lock`)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM`)).
		WillReturnError(context.Canceled)
	mock.ExpectRollback()

	if _, err = svc.RunRetention(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("retention err=%v want context.Canceled", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

func TestPublishAssignmentCompletesWithOneConnection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_try_advisory_xact_lock`).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectExec(`WITH candidates`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	svc := NewService(NewRepository(db, time.Hour), DefaultConfig())
	if _, err := svc.RunPublishAssignment(ctx); err != nil {
		t.Fatalf("publisher with a one-connection pool: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCursorPreservesExplicitSince(t *testing.T) {
	since := time.Now().UTC().Add(-time.Hour)
	cursor, err := encodeCursor(4)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, DefaultConfig())
	query, err := svc.buildDomainQuery(context.Background(), FeedQuery{Cursor: cursor, Since: &since, Limit: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if query.Since == nil || !query.Since.Equal(since) {
		t.Fatal("cursor discarded explicit since bound")
	}
}

func TestReadUpdatedIsNewestRecordTime(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"publish_seq", "id", "event_type", "subject", "source", "time", "dataschema_full", "data_full", "authorization_aas_ids"}).
		AddRow(1, "first", TypeAASCreated, "aas", "source", now, "schema", `{}`, nil).
		AddRow(2, "second", TypeAASCreated, "aas", "source", now.Add(-time.Minute), "schema", `{}`, nil))
	svc := NewService(NewRepository(db, time.Hour), DefaultConfig())
	result, err := svc.Read(context.Background(), FeedQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Records[0].Time.Before(result.Records[1].Time) {
		t.Fatal("response records are not chronological")
	}
	if !result.Updated.Equal(now) {
		t.Fatalf("updated=%s, newest=%s", result.Updated, now)
	}
}

func TestPublishAssignmentRollsBackBatchBeforeRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	svc := NewService(NewRepository(db, time.Hour), DefaultConfig())
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_try_advisory_xact_lock`).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectExec(`WITH candidates`).WillReturnError(context.Canceled)
	mock.ExpectRollback()
	count, err := svc.RunPublishAssignment(context.Background())
	if !errors.Is(err, context.Canceled) || count != 0 {
		t.Fatalf("failed batch: count=%d error=%v", count, err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_try_advisory_xact_lock`).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectExec(`WITH candidates`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	count, err = svc.RunPublishAssignment(context.Background())
	if err != nil || count != 2 {
		t.Fatalf("retry: count=%d error=%v", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerSkipsBusyTransactionLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	svc := NewService(NewRepository(db, time.Hour), DefaultConfig())
	for _, run := range []func(context.Context) (int64, error){svc.RunPublishAssignment, svc.RunRetention} {
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT pg_try_advisory_xact_lock`).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
		mock.ExpectRollback()
		count, err := run(context.Background())
		if err != nil || count != 0 {
			t.Fatalf("busy worker: count=%d error=%v", count, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
