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
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
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

func TestConfigureDefaultsFullSnapshotInterval(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})

	Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	require.Equal(t, DefaultFullSnapshotInterval, ActiveConfig().FullSnapshotInterval)
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

func TestComputeHistoryRowHashIncludesPayloadMetadata(t *testing.T) {
	t.Parallel()

	event := ChangeEvent{
		EntityType:   TableAAS,
		Identifier:   "aas-1",
		ChangeType:   ChangeUpdated,
		Timestamp:    time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		PayloadType:  PayloadTypeSnapshot,
		ContentHash:  "content",
		PayloadHash:  "payload-a",
		PreviousHash: "previous",
	}
	hashA, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	event.PayloadHash = "payload-b"
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
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history"`).
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
	expectLatestSnapshotRestore(mock, TableSubmodel, "submodel_history_payload", "sm-1", 7, latestSnapshot, false, "previous")
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

	latestSnapshot := map[string]any{"id": "sm-1", "idShort": "before"}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLatestSnapshotRestore(mock, TableSubmodel, "submodel_history_payload", "sm-1", 7, latestSnapshot, false, "previous")
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

	baseSnapshot := map[string]any{"id": "aas-1", "idShort": "before"}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectLatestSnapshotRestore(mock, TableAAS, "aas_history_payload", "aas-1", 1, baseSnapshot, false, "previous")
	mock.ExpectQuery(`INSERT INTO "aas_history".*RETURNING "history_id"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO "aas_history_payload".*"diff"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)
	err = AppendVersionTx(context.Background(), tx, TableAAS, "aas-1", ChangeUpdated, map[string]any{"id": "aas-1", "idShort": "after"}, false)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
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
		WillReturnRows(sqlmock.NewRows(historyChainColumns()).
			AddRow(historyChainRow(1, "aas-1", ChangeCreated, PayloadTypeSnapshot, checkpoint, nil, nil, false, operationTime, "row-1")...).
			AddRow(historyChainRow(2, "aas-1", ChangeUpdated, PayloadTypeDiff, nil, patch12, v2, false, operationTime, "row-2")...).
			AddRow(historyChainRow(3, "aas-1", ChangeUpdated, PayloadTypeDiff, nil, patch23, v3, false, operationTime, "row-3")...))
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

func TestApplyPostgresGuardConfigEnablesGuardedMode(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAudit, Immutability: ImmutabilityPostgresGuarded, AuditIdentityMode: AuditIdentityMinimal})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectQuery(`INSERT INTO "history_guard_config".*ON CONFLICT.*RETURNING "enabled"`).
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(true))

	err = ApplyPostgresGuardConfig(context.Background(), db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyPostgresGuardConfigKeepsDisabledGuardDisabledWhenHistoryIsOff(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeOff, Immutability: ImmutabilityPostgresGuarded, AuditIdentityMode: AuditIdentityMinimal})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectQuery(`INSERT INTO "history_guard_config".*ON CONFLICT.*RETURNING "enabled"`).
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(false))

	err = ApplyPostgresGuardConfig(context.Background(), db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyPostgresGuardConfigRejectsStartupDowngrade(t *testing.T) {
	t.Cleanup(func() {
		Configure(Config{Mode: ModeOff, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})
	})
	Configure(Config{Mode: ModeAPI, Immutability: ImmutabilityNone, AuditIdentityMode: AuditIdentityNone})

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	mock.ExpectQuery(`INSERT INTO "history_guard_config".*ON CONFLICT.*RETURNING "enabled"`).
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(true))

	err = ApplyPostgresGuardConfig(context.Background(), db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "HISTORY-GUARD-CONFLICT")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecentRowsReturnsNewestRowsFirstAndCursorPaginatesOlderRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	operationTime := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history".*ORDER BY "history"."history_id" DESC LIMIT 2`).
		WillReturnRows(sqlmock.NewRows([]string{
			"history_id",
			"identifier",
			"change_type",
			"deleted",
			"administration_created_at_text",
			"administration_updated_at_text",
			"operation_time",
		}).
			AddRow(int64(12), "aas-3", ChangeUpdated, false, nil, nil, operationTime).
			AddRow(int64(11), "aas-2", ChangeCreated, false, nil, nil, operationTime))
	expectSnapshotRestore(mock, TableAAS, "aas_history_payload", "aas-3", 12, map[string]any{"id": "aas-3"}, false, operationTime, "row-12")

	rows, cursor, err := RecentRows(context.Background(), db, TableAAS, 1, "", time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(12), rows[0].HistoryID)
	require.Equal(t, operationTime.Format(time.RFC3339Nano), rows[0].CreatedAt)
	require.Equal(t, operationTime.Format(time.RFC3339Nano), rows[0].UpdatedAt)
	require.Equal(t, "12", cursor)

	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history".*WHERE \("history"\."history_id" < 12\).*ORDER BY "history"."history_id" DESC LIMIT 2`).
		WillReturnRows(sqlmock.NewRows([]string{
			"history_id",
			"identifier",
			"change_type",
			"deleted",
			"administration_created_at_text",
			"administration_updated_at_text",
			"operation_time",
		}).
			AddRow(int64(11), "aas-2", ChangeCreated, false, nil, nil, operationTime).
			AddRow(int64(10), "aas-1", ChangeCreated, false, nil, nil, operationTime))
	expectSnapshotRestore(mock, TableAAS, "aas_history_payload", "aas-2", 11, map[string]any{"id": "aas-2"}, false, operationTime, "row-11")

	rows, cursor, err = RecentRows(context.Background(), db, TableAAS, 1, cursor, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(11), rows[0].HistoryID)
	require.Equal(t, "11", cursor)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSnapshotByDateRestoresDiffBackedVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	operationTime := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	checkpoint := map[string]any{"id": "aas-1", "idShort": "v1"}
	target := map[string]any{"id": "aas-1", "idShort": "v2"}
	patch, err := BuildJSONPatch(checkpoint, target)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT "history"."history_id" FROM "aas_history" AS "history".*ORDER BY "history"."valid_from" DESC, "history"."history_id" DESC LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(2)))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(sqlmock.NewRows(historyChainColumns()).
			AddRow(historyChainRow(1, "aas-1", ChangeCreated, PayloadTypeSnapshot, checkpoint, nil, nil, false, operationTime, "row-1")...).
			AddRow(historyChainRow(2, "aas-1", ChangeUpdated, PayloadTypeDiff, nil, patch, target, false, operationTime, "row-2")...))

	snapshot, err := SnapshotByDate(context.Background(), db, TableAAS, "aas-1", operationTime)
	require.NoError(t, err)
	require.Equal(t, target, snapshot)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSnapshotByDateRejectsContentHashMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	operationTime := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	snapshot := map[string]any{"id": "aas-1"}
	snapshotJSON := mustJSONText(t, snapshot)
	payloadHash, err := CanonicalJSONHash(snapshot)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT "history"."history_id" FROM "aas_history" AS "history".*ORDER BY "history"."valid_from" DESC, "history"."history_id" DESC LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(sqlmock.NewRows(historyChainColumns()).
			AddRow(int64(1), "aas-1", ChangeCreated, PayloadTypeSnapshot, snapshotJSON, nil, false, nil, nil, operationTime, "wrong-content-hash", payloadHash, "row-1"))

	_, err = SnapshotByDate(context.Background(), db, TableAAS, "aas-1", operationTime)
	require.Error(t, err)
	require.Contains(t, err.Error(), "HISTORY-RESTORE-CONTENTHASH")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecentRowsRejectsLimitAboveMaximum(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	_, _, err = RecentRows(context.Background(), db, TableAAS, MaxRecentChangesLimit+1, "", time.Time{}, time.Time{})
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
}

func TestFilterRecentRowsScansUntilFilteredPageIsFull(t *testing.T) {
	fetchCalls := 0
	fetch := func(_ int32, cursor string) ([]Row, string, error) {
		fetchCalls++
		switch cursor {
		case "":
			return []Row{
				{HistoryID: 4, Identifier: "skip"},
				{HistoryID: 3, Identifier: "include"},
			}, "3", nil
		case "3":
			return []Row{
				{HistoryID: 2, Identifier: "include"},
				{HistoryID: 1, Identifier: "skip"},
			}, "2", nil
		default:
			t.Fatalf("unexpected cursor %q", cursor)
			return nil, "", nil
		}
	}

	rows, cursor, err := FilterRecentRows(2, "", fetch, func(row Row) (bool, error) {
		return row.Identifier == "include", nil
	})

	require.NoError(t, err)
	require.Equal(t, []Row{
		{HistoryID: 3, Identifier: "include"},
		{HistoryID: 2, Identifier: "include"},
	}, rows)
	require.Equal(t, "2", cursor)
	require.Equal(t, 2, fetchCalls)
}

func expectLatestSnapshotRestore(mock sqlmock.Sqlmock, table string, payloadTable string, identifier string, historyID int64, snapshot map[string]any, deleted bool, rowHash string) {
	mock.ExpectQuery(`SELECT "history_id" FROM "` + table + `"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(historyID))
	expectSnapshotRestore(mock, table, payloadTable, identifier, historyID, snapshot, deleted, time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC), rowHash)
}

func expectSnapshotRestore(mock sqlmock.Sqlmock, table string, payloadTable string, identifier string, historyID int64, snapshot map[string]any, deleted bool, operationTime time.Time, rowHash string) {
	mock.ExpectQuery(`SELECT "history_id" FROM "` + table + `".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(historyID))
	mock.ExpectQuery(`SELECT .*FROM "` + table + `" AS "history" INNER JOIN "` + payloadTable + `" AS "payload"`).
		WillReturnRows(sqlmock.NewRows(historyChainColumns()).
			AddRow(historyChainRow(historyID, identifier, ChangeUpdated, PayloadTypeSnapshot, snapshot, nil, nil, deleted, operationTime, rowHash)...))
}

func historyChainColumns() []string {
	return []string{
		"history_id",
		"identifier",
		"change_type",
		"payload_type",
		"snapshot",
		"diff",
		"deleted",
		"administration_created_at_text",
		"administration_updated_at_text",
		"operation_time",
		"content_hash",
		"payload_hash",
		"row_hash",
	}
}

func historyChainRow(
	historyID int64,
	identifier string,
	changeType string,
	payloadType string,
	snapshot map[string]any,
	patch []map[string]any,
	contentSnapshot map[string]any,
	deleted bool,
	operationTime time.Time,
	rowHash string,
) []driver.Value {
	var snapshotText any
	var diffText any
	var payloadValue any
	if payloadType == PayloadTypeSnapshot {
		snapshotText = mustJSONTextForHelper(snapshot)
		payloadValue = snapshot
		contentSnapshot = snapshot
	} else {
		diffText = mustJSONTextForHelper(patch)
		payloadValue = patch
		if contentSnapshot == nil {
			panic("diff history test row requires reconstructed content snapshot")
		}
	}
	contentHash := mustHashForHelper(contentSnapshot)
	payloadHash := mustHashForHelper(payloadValue)
	return []driver.Value{
		historyID,
		identifier,
		changeType,
		payloadType,
		snapshotText,
		diffText,
		deleted,
		nil,
		nil,
		operationTime,
		contentHash,
		payloadHash,
		rowHash,
	}
}

func mustJSONText(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func mustJSONTextForHelper(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func mustHashForHelper(value any) string {
	hash, err := CanonicalJSONHash(value)
	if err != nil {
		panic(err)
	}
	return hash
}
