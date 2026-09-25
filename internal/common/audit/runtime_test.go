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
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql/driver"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	jose "gopkg.in/go-jose/go-jose.v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManifestSignerIsDeterministicAndVerifiable(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer := rsaManifestSigner{key: key, keyID: "key-rotation-1"}
	payload := []byte(`{"sequence":42}`)
	first, err := signer.SignManifest(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := signer.SignManifest(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("archive retry signature changed")
	}
	signed, err := jose.ParseSigned(string(first))
	if err != nil {
		t.Fatal(err)
	}
	verified, err := signed.Verify(&key.PublicKey)
	if err != nil || !bytes.Equal(verified, payload) {
		t.Fatalf("verification failed: %v", err)
	}
	if signed.Signatures[0].Header.KeyID != "key-rotation-1" {
		t.Fatal("missing rotation key identifier")
	}
}

func TestRuntimeConfigurationFailsClosed(t *testing.T) {
	if _, err := NewRuntime(context.Background(), nil, RuntimeConfig{Enabled: true}); err == nil {
		t.Fatal("missing database accepted")
	}
	if _, err := configuredSigner(RuntimeConfig{SigningRequired: true}); err == nil {
		t.Fatal("missing required key accepted")
	}
	runtime, err := NewRuntime(context.Background(), nil, RuntimeConfig{})
	if err != nil || runtime.Enabled {
		t.Fatalf("disabled runtime failed: %v", err)
	}
}

func TestAuditHandlerRejectsMissingAdministrator(t *testing.T) {
	handler := AdminHandler{Stream: "authorization"}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/security/rebac/audit", nil))
	if response.Code != 403 {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestSearchFilterRejectsUnboundedOrInvalidRanges(t *testing.T) {
	for _, query := range []string{"limit=1001", "limit=0", "after=-1", "from=invalid", "from=2026-09-25T00:00:00Z&until=2026-09-24T00:00:00Z"} {
		if _, err := parseSearchFilter(httptest.NewRequest("GET", "/security/rebac/audit?"+query, nil), "authorization"); err == nil {
			t.Fatalf("accepted invalid filter %s", query)
		}
	}
}

type correlatedAuditEvent struct{}

func (correlatedAuditEvent) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	var event Event
	return json.Unmarshal([]byte(raw), &event) == nil && strings.TrimSpace(event.CorrelationID) != ""
}

func TestAuditHTTPPersistsCorrelationBeforeReturningData(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT .* FROM "audit_record"`).WillReturnRows(sqlmock.NewRows([]string{"event_id", "sequence", "occurred_at", "actor", "resource", "outcome", "correlation_id", "event", "previous_hash", "content_hash"}))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "audit_stream"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "audit_stream"`).WillReturnRows(sqlmock.NewRows([]string{"last_sequence", "last_hash"}).AddRow(0, ""))
	mock.ExpectExec(`INSERT INTO "audit_record"`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), correlatedAuditEvent{}, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "audit_stream"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	handler := AdminHandler{Repository: Repository{DB: db}, Stream: "authorization", Authorize: func(*http.Request) (string, error) { return "admin", nil }}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/security/rebac/audit", nil))
	if response.Code != 200 {
		t.Fatalf("audit request returned %d: %s", response.Code, response.Body.String())
	}
	mock.ExpectClose()
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
