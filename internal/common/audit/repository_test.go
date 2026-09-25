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

package audit

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAppendAddsHashChainedRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	event := testEvent()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO \"audit_stream\"")).WithArgs("access").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT \"last_sequence\", \"last_hash\" FROM \"audit_stream\" WHERE (\"stream\" = $1) FOR UPDATE")).WithArgs("access").WillReturnRows(sqlmock.NewRows([]string{"last_sequence", "last_hash"}).AddRow(0, ""))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO \"audit_record\"")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE \"audit_stream\" SET")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectRollback()

	receipt, err := (Repository{}).Append(t.Context(), tx, "access", event)
	if err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if receipt.Sequence != 1 || receipt.EventID != event.ID || receipt.ContentHash == "" {
		t.Fatalf("unexpected receipt %#v", receipt)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAppendReturnsRecordInsertFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO \"audit_stream\"")).WithArgs("access").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT \"last_sequence\", \"last_hash\" FROM \"audit_stream\" WHERE (\"stream\" = $1) FOR UPDATE")).WithArgs("access").WillReturnRows(sqlmock.NewRows([]string{"last_sequence", "last_hash"}).AddRow(0, ""))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO \"audit_record\"")).WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()

	if _, err = (Repository{}).Append(t.Context(), tx, "access", testEvent()); err == nil {
		t.Fatal("Append accepted a failed audit record insert")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHashEventChangesWhenRecordIsTampered(t *testing.T) {
	event := testEvent()
	hash, _, err := hashEvent("", 1, event)
	if err != nil {
		t.Fatalf("hash event: %v", err)
	}
	event.Outcome = "denied"
	tamperedHash, _, err := hashEvent("", 1, event)
	if err != nil {
		t.Fatalf("hash tampered event: %v", err)
	}
	if hash == tamperedHash {
		t.Fatal("tampering did not change audit content hash")
	}
}

func TestNormalizeEventRemovesCredentialDetails(t *testing.T) {
	event := normalizeEvent(Event{Actor: "user:alice", Resource: "asset:one", Outcome: "allowed", CorrelationID: "request", Payload: Payload{Action: "read", Details: map[string]string{"authorization": "secret", "method": "GET"}}})
	if event.ID == "" || event.OccurredAt.IsZero() {
		t.Fatal("normalize event did not assign ID and timestamp")
	}
	if _, found := event.Payload.Details["authorization"]; found {
		t.Fatal("credential detail was retained")
	}
	if event.Payload.Details["method"] != "GET" {
		t.Fatal("safe detail was lost")
	}
}

func testEvent() Event {
	return Event{ID: "123e4567-e89b-12d3-a456-426614174000", OccurredAt: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC), Actor: "user:alice", Resource: "asset:one", Outcome: "allowed", CorrelationID: "request-1", Payload: Payload{Action: "read"}}
}
