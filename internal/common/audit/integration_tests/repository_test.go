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

package integration_tests

import (
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/audit"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func auditTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BASYX_REBAC_TEST_DSN")
	if dsn == "" {
		t.Skip("BASYX_REBAC_TEST_DSN is required for live PostgreSQL tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAppendRollbackLeavesNoAuditRecord(t *testing.T) {
	db := auditTestDatabase(t)
	stream := auditStream()
	repository := audit.Repository{DB: db}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repository.Append(t.Context(), tx, stream, auditEvent("rollback")); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = auditQueryRow(t, db, goqu.Dialect("postgres").From("audit_record").Select(goqu.COUNT("*")).Where(goqu.C("stream").Eq(stream))).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled back append persisted %d records", count)
	}
}

func TestConcurrentAppendsProduceVerifiableChain(t *testing.T) {
	db := auditTestDatabase(t)
	stream := auditStream()
	repository := audit.Repository{DB: db}
	const writers = 12
	errorsByWriter := make(chan error, writers)
	var group sync.WaitGroup
	for index := 0; index < writers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := repository.Record(t.Context(), stream, auditEvent("concurrent"))
			errorsByWriter <- err
		}()
	}
	group.Wait()
	close(errorsByWriter)
	for err := range errorsByWriter {
		if err != nil {
			t.Fatal(err)
		}
	}
	verification, err := repository.Verify(t.Context(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Valid || verification.Records != writers {
		t.Fatalf("verification = %#v", verification)
	}
}

func TestVerifyDetectsControlledHashTamper(t *testing.T) {
	db := auditTestDatabase(t)
	stream := auditStream()
	repository := audit.Repository{DB: db}
	first, err := repository.Record(t.Context(), stream, auditEvent("first"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repository.Record(t.Context(), stream, auditEvent("second")); err != nil {
		t.Fatal(err)
	}
	query, args, err := goqu.Dialect("postgres").Update("audit_record").Set(goqu.Record{"content_hash": "tampered"}).Where(goqu.C("event_id").Eq(first.EventID)).Prepared(true).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
	verification, err := repository.Verify(t.Context(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Valid || verification.FailedSequence != first.Sequence {
		t.Fatalf("verification = %#v, want detected first-row tamper", verification)
	}
}

func TestWORMBacklogFailsClosed(t *testing.T) {
	db := auditTestDatabase(t)
	stream := auditStream()
	var pending int
	if err := auditQueryRow(t, db, goqu.Dialect("postgres").From("audit_delivery").Select(goqu.COUNT("*")).Where(goqu.C("archived_at").IsNull())).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	repository := audit.Repository{DB: db, WORM: true, BacklogLimit: pending + 1}
	if _, err := repository.Record(t.Context(), stream, auditEvent("queued")); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Record(t.Context(), stream, auditEvent("blocked")); err == nil {
		t.Fatal("WORM backlog limit did not fail closed")
	}
	var count int
	if err := auditQueryRow(t, db, goqu.Dialect("postgres").From("audit_record").Select(goqu.COUNT("*")).Where(goqu.C("stream").Eq(stream))).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("backlog failure persisted %d records", count)
	}
}

func TestRecordCommitsIndependentTransaction(t *testing.T) {
	db := auditTestDatabase(t)
	stream := auditStream()
	repository := audit.Repository{DB: db}
	receipt, err := repository.Record(t.Context(), stream, auditEvent("independent"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := repository.List(t.Context(), stream, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].EventID != receipt.EventID {
		t.Fatalf("records = %#v, receipt = %#v", records, receipt)
	}
}

func auditStream() string { return "audit-test-" + uuid.NewString() }

func auditEvent(action string) audit.Event {
	return audit.Event{OccurredAt: time.Now().UTC(), Actor: "user:integration", Resource: "aas:" + uuid.NewString(), Outcome: "allowed", CorrelationID: uuid.NewString(), Payload: audit.Payload{Action: action}}
}

func auditQueryRow(t *testing.T, db *sql.DB, query *goqu.SelectDataset) *sql.Row {
	t.Helper()
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	return db.QueryRowContext(t.Context(), statement, args...)
}

func TestVerifyDuringConcurrentAppend(t *testing.T) {
	db := auditTestDatabase(t)
	stream := auditStream()
	repository := audit.Repository{DB: db}
	if _, err := repository.Record(t.Context(), stream, auditEvent("initial")); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		for i := 0; i < 30; i++ {
			if _, err := repository.Record(t.Context(), stream, auditEvent("concurrent")); err != nil {
				finished <- err
				return
			}
		}
		finished <- nil
	}()
	for i := 0; i < 30; i++ {
		verification, err := repository.Verify(t.Context(), stream)
		if err != nil || !verification.Valid {
			t.Errorf("concurrent append reported tampering: %+v %v", verification, err)
			break
		}
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
