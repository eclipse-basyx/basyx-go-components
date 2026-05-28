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

package history

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCanonicalJSONHashIsStableForObjectKeyOrder(t *testing.T) {
	t.Parallel()

	left := map[string]any{
		"b": []any{"x", "y"},
		"a": map[string]any{"z": float64(1), "c": true},
	}
	right := map[string]any{
		"a": map[string]any{"c": true, "z": float64(1)},
		"b": []any{"x", "y"},
	}

	leftHash, err := CanonicalJSONHash(left)
	require.NoError(t, err)
	rightHash, err := CanonicalJSONHash(right)
	require.NoError(t, err)

	require.Equal(t, leftHash, rightHash)
}

func TestComputeHistoryRowHashIncludesPreviousHash(t *testing.T) {
	t.Parallel()

	event := ChangeEvent{
		EntityType:   TableAAS,
		Identifier:   "aas-1",
		ChangeType:   ChangeUpdated,
		Timestamp:    time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		Deleted:      false,
		ContentHash:  "content",
		PreviousHash: "previous-a",
	}
	hashA, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	event.PreviousHash = "previous-b"
	hashB, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	require.NotEqual(t, hashA, hashB)
}

func TestAuditContextRoundTrip(t *testing.T) {
	t.Parallel()

	expected := AuditContext{
		ActorSubject:  "subject",
		ActorIssuer:   "issuer",
		ClientID:      "client",
		RequestID:     "request",
		CorrelationID: "correlation",
		Endpoint:      "/shells",
		HTTPMethod:    "POST",
	}

	actual := FromContext(ContextWithAudit(context.Background(), expected))

	require.Equal(t, expected, actual)
}

func TestComputeHistoryRowHashIncludesAuditMetadata(t *testing.T) {
	t.Parallel()

	event := ChangeEvent{
		EntityType:   TableAAS,
		Identifier:   "aas-1",
		ChangeType:   ChangeUpdated,
		Timestamp:    time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		ContentHash:  "content",
		ActorSubject: "subject-a",
	}
	hashA, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	event.ActorSubject = "subject-b"
	hashB, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	require.NotEqual(t, hashA, hashB)
}

func TestAppendVersionTxInsertsWithoutUpdatingPreviousRows(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})
	})
	Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "row_hash" FROM "aas_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO "aas_history"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeCreated, map[string]any{"id": "aas-1"}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxSkipsWritesWhenHistoryModeOff(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})
	})
	Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})

	err := AppendVersionTx(context.Background(), nil, TableAAS, "aas-1", ChangeCreated, map[string]any{"id": "aas-1"}, false)
	require.NoError(t, err)
}

func TestApplyPostgresGuardConfigEnablesGuardedMode(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})
	})
	Configure(Config{Mode: ModeAudit, Immutability: ImmutabilityPostgresGuarded, AuditIdentityMode: AuditIdentityMinimal})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectExec(`UPDATE "history_guard_config"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = ApplyPostgresGuardConfig(context.Background(), db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyPostgresGuardConfigDisablesGuardWhenHistoryIsOff(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityMinimal})
	})
	Configure(Config{Mode: ModeOff, Immutability: ImmutabilityPostgresGuarded, AuditIdentityMode: AuditIdentityMinimal})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectExec(`UPDATE "history_guard_config"`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = ApplyPostgresGuardConfig(context.Background(), db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
