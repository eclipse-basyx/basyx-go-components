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
	"encoding/json"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

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
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "row_hash" FROM "aas_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "aas_history_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeCreated, map[string]any{"id": "aas-1"}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxSnapshotIntervalOneUsesPreviousHashOnly(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 1, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "row_hash" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"row_hash"}).AddRow("previous"))
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, map[string]any{"id": "aas-1", "counter": json.Number("9007199254740993")}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildLockIdentifierQueryUsesPostgresPlaceholders(t *testing.T) {
	query, args, err := buildLockIdentifierQuery(TableAAS, "aas-1")

	require.NoError(t, err)
	require.Equal(t, "SELECT pg_advisory_xact_lock(hashtextextended($1, $2))", query)
	require.Equal(t, []any{"aas_history:aas-1", int64(0)}, args)
}

func TestAppendMutatedVersionTxUsesLatestSnapshotAndRowHash(t *testing.T) {
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
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	latestSnapshot := map[string]any{"id": "sm-1", "submodelElements": []any{}}
	expectLatestSnapshotRestore(mock, TableSubmodel, "submodel_history_payload", "sm-1", 7, latestSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "submodel_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO "submodel_history_payload"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendMutatedVersionTx(context.Background(), tx, TableSubmodel, "sm-1", ChangeUpdated, func(snapshot map[string]any) error {
		snapshot["idShort"] = "updated"
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendMutatedVersionTxStoresDiffFromOriginalSnapshot(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 3, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	largeUnchangedValue := strings.Repeat("unchanged-", 40)
	latestSnapshot := map[string]any{"id": "sm-1", "idShort": "before", "description": largeUnchangedValue}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLatestSnapshotRestore(mock, TableSubmodel, "submodel_history_payload", "sm-1", 7, latestSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "submodel_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(8))
	mock.ExpectExec(`INSERT INTO "submodel_history_payload".*"diff".*/idShort.*after`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendMutatedVersionTx(context.Background(), tx, TableSubmodel, "sm-1", ChangeUpdated, func(snapshot map[string]any) error {
		snapshot["idShort"] = "after"
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxStoresDiffWhenIntervalAllows(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 3, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	largeUnchangedValue := strings.Repeat("unchanged-", 40)
	baseSnapshot := map[string]any{"id": "aas-1", "idShort": "before", "description": largeUnchangedValue}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLatestSnapshotRestore(mock, TableAAS, "aas_history_payload", "aas-1", 1, baseSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"diff"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, map[string]any{"id": "aas-1", "idShort": "after", "description": largeUnchangedValue}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendVersionTxStoresSnapshotWhenDiffWouldBeLarger(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 3, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	baseSnapshot := map[string]any{"items": []any{"a", "b", "c", "d", "e", "f"}}
	targetSnapshot := map[string]any{"items": []any{"f", "e", "d", "c", "b", "a"}}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLatestSnapshotRestore(mock, TableAAS, "aas_history_payload", "aas-1", 1, baseSnapshot, false)
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, targetSnapshot, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildHistoryPayloadFallsBackToSnapshotWhenDiffIsNotSmaller(t *testing.T) {
	baseSnapshot := map[string]any{"items": []any{"a", "b", "c", "d", "e", "f"}}
	targetSnapshot := map[string]any{"items": []any{"f", "e", "d", "c", "b", "a"}}
	latest := &latestVersion{
		snapshot:          baseSnapshot,
		rowsSinceSnapshot: 1,
	}

	payload, err := buildHistoryPayload(targetSnapshot, latest, Config{FullSnapshotInterval: 3})
	require.NoError(t, err)

	require.Equal(t, PayloadTypeSnapshot, payload.payloadType)
	expectedHash, err := CanonicalJSONHash(targetSnapshot)
	require.NoError(t, err)
	require.Equal(t, expectedHash, payload.hash)
}

func TestBuildHistoryPayloadHashesSelectedDiffPayload(t *testing.T) {
	largeUnchangedValue := strings.Repeat("unchanged-", 40)
	baseSnapshot := map[string]any{"id": "aas-1", "idShort": "before", "description": largeUnchangedValue}
	targetSnapshot := map[string]any{"id": "aas-1", "idShort": "after", "description": largeUnchangedValue}
	latest := &latestVersion{
		snapshot:          baseSnapshot,
		rowsSinceSnapshot: 1,
	}

	payload, err := buildHistoryPayload(targetSnapshot, latest, Config{FullSnapshotInterval: 3})
	require.NoError(t, err)

	patch, err := BuildJSONPatch(baseSnapshot, targetSnapshot)
	require.NoError(t, err)
	expectedHash, err := CanonicalJSONHash(patch)
	require.NoError(t, err)
	require.Equal(t, PayloadTypeDiff, payload.payloadType)
	require.Equal(t, expectedHash, payload.hash)
}

func TestAppendVersionTxStoresScheduledSnapshotAtIntervalBoundary(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, FullSnapshotInterval: 3, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	checkpoint := map[string]any{"id": "aas-1", "idShort": "v1"}
	v2 := map[string]any{"id": "aas-1", "idShort": "v2"}
	v3 := map[string]any{"id": "aas-1", "idShort": "v3"}
	patch12, err := BuildJSONPatch(checkpoint, v2)
	require.NoError(t, err)
	patch23, err := BuildJSONPatch(v2, v3)
	require.NoError(t, err)
	operationTime := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(3)))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(newHistoryChainRows(TableAAS,
			historyChainRowSpec{HistoryID: 1, Identifier: "aas-1", ChangeType: ChangeCreated, PayloadType: PayloadTypeSnapshot, Snapshot: checkpoint, Deleted: false, OperationTime: operationTime},
			historyChainRowSpec{HistoryID: 2, Identifier: "aas-1", ChangeType: ChangeUpdated, PayloadType: PayloadTypeDiff, Patch: patch12, ContentSnapshot: v2, Deleted: false, OperationTime: operationTime},
			historyChainRowSpec{HistoryID: 3, Identifier: "aas-1", ChangeType: ChangeUpdated, PayloadType: PayloadTypeDiff, Patch: patch23, ContentSnapshot: v3, Deleted: false, OperationTime: operationTime},
		))
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(4))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"snapshot"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, map[string]any{"id": "aas-1", "idShort": "v4"}, false)
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

func TestNullableTimestampAcceptsSharedISO8601Formats(t *testing.T) {
	offsetTimestamp, ok := nullableTimestamp("2026-05-28T14:30:00+0200").(time.Time)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 5, 28, 12, 30, 0, 0, time.UTC), offsetTimestamp)

	utcTimestamp, ok := nullableTimestamp("2026-05-28T12:30:00.123456789 UTC").(time.Time)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 5, 28, 12, 30, 0, 123456789, time.UTC), utcTimestamp)
}
