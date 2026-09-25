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
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/evidence"
)

func TestArchiveNextAcknowledgesOnlyVerifiedEvidence(t *testing.T) {
	db, mock := archiveMock(t)
	expectPendingDelivery(mock)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE \"audit_delivery\" SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	store := &validArchiveStore{}

	done, err := (Archiver{DB: db, Store: store}).ArchiveNext(context.TODO())
	if err != nil || !done {
		t.Fatalf("archive result done=%t err=%v", done, err)
	}
	if store.puts != 1 || store.verifies != 1 || store.retentionChecks != 1 {
		t.Fatalf("unexpected store calls %+v", store)
	}
	mock.ExpectClose()
	if err = db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveNextDoesNotAcknowledgeFailedEvidence(t *testing.T) {
	cases := []struct {
		name  string
		store validArchiveStore
		want  string
	}{
		{"put", validArchiveStore{putErr: errors.New("put failed")}, "AUDIT-ARCHIVE-PUT"},
		{"missing version", validArchiveStore{missingVersion: true}, "AUDIT-ARCHIVE-RECEIPT"},
		{"bad hash", validArchiveStore{receiptHash: "bad"}, "AUDIT-ARCHIVE-RECEIPT"},
		{"retention", validArchiveStore{retentionErr: errors.New("retention failed")}, "AUDIT-ARCHIVE-RETENTION"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			db, mock := archiveMock(t)
			expectPendingDelivery(mock)
			done, err := (Archiver{DB: db, Store: &testCase.store}).ArchiveNext(context.TODO())
			if done || err == nil || !regexp.MustCompile(testCase.want).MatchString(err.Error()) {
				t.Fatalf("unexpected archive result done=%t err=%v", done, err)
			}
			mock.ExpectClose()
			if err = db.Close(); err != nil {
				t.Fatalf("close database: %v", err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func archiveMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	return db, mock
}

func expectPendingDelivery(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"event_id", "stream", "sequence", "occurred_at", "actor", "resource", "outcome", "correlation_id", "event", "previous_hash", "content_hash"}).AddRow("123e4567-e89b-12d3-a456-426614174000", "access", int64(1), time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC), "user:alice", "asset:one", "allowed", "request-1", []byte(`{"action":"read"}`), "", "chain-hash"))
}

type validArchiveStore struct {
	puts, verifies, retentionChecks int
	putErr, retentionErr            error
	versionID, receiptHash          string
	missingVersion                  bool
}

func (store *validArchiveStore) PutArtifact(_ context.Context, artifact evidence.Artifact) (*evidence.Receipt, error) {
	store.puts++
	if store.putErr != nil {
		return nil, store.putErr
	}
	versionID := store.versionID
	if versionID == "" && !store.missingVersion {
		versionID = "version-1"
	}
	hash := store.receiptHash
	if hash == "" {
		hash = evidence.SHA256Hex(artifact.Data)
	}
	return &evidence.Receipt{Reference: evidence.Reference{Provider: evidence.ProviderS3, Bucket: "evidence", ObjectKey: artifact.ObjectKey, VersionID: versionID}, SHA256: hash, RetentionMode: "governance", RetainUntil: ptr(time.Now().Add(time.Hour))}, nil
}
func (store *validArchiveStore) GetArtifact(context.Context, evidence.Reference) (*evidence.Object, error) {
	return nil, errors.New("not used")
}
func (store *validArchiveStore) VerifyArtifact(_ context.Context, ref evidence.Reference, hash string) (*evidence.Receipt, error) {
	store.verifies++
	return &evidence.Receipt{Reference: ref, SHA256: hash}, nil
}
func (store *validArchiveStore) VerifyArtifactRetention(context.Context, evidence.Reference, evidence.Receipt) error {
	store.retentionChecks++
	return store.retentionErr
}
func ptr(value time.Time) *time.Time { return &value }
