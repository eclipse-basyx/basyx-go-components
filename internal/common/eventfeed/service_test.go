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
		Presentation: PresentationFull,
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
	if caps.Presentation.Default != "FULL" {
		t.Fatalf("default presentation=%s", caps.Presentation.Default)
	}
}

func TestReadWithSQLMock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

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
		"id", "event_type", "subject", "source", "time",
		"dataschema_full", "dataschema_compact", "data_full", "data_compact",
	}).
		AddRow("e1", TypeAASCreated, "aas-1", "http://localhost/shells", ts1,
			"https://s/full", "https://s/compact", `{"aasId":"aas-1"}`, `{"aasId":"aas-1"}`).
		AddRow("e2", TypeAASUpdated, "aas-1", "http://localhost/shells", ts2,
			"https://s/full", "https://s/compact", `{"aasId":"aas-1"}`, `{"aasId":"aas-1"}`).
		AddRow("e3", TypeAASUpdated, "aas-2", "http://localhost/shells", ts2.Add(time.Minute),
			"https://s/full", "https://s/compact", `{"aasId":"aas-2"}`, `{"aasId":"aas-2"}`)

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

func TestHTTPHandlers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

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
		"id", "event_type", "subject", "source", "time",
		"dataschema_full", "dataschema_compact", "data_full", "data_compact",
	}).AddRow("e1", TypeAASCreated, "aas-1", "http://localhost/shells", fixedNow.Add(-time.Hour),
		"https://s/full", "https://s/compact", `{"aasId":"aas-1"}`, `{"aasId":"aas-1"}`)
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
	defer db.Close()

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
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := svc.Write(context.Background(), ev); err != nil {
		t.Fatalf("write: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM`)).
		WillReturnResult(sqlmock.NewResult(0, 3))
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
