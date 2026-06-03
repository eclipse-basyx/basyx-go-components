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
	mock.ExpectQuery(`SELECT "payload"."snapshot"::text, "history"."deleted", "history"."row_hash" FROM "submodel_history" AS "history" INNER JOIN "submodel_history_payload" AS "payload"`).
		WillReturnRows(sqlmock.NewRows([]string{"snapshot", "deleted", "row_hash"}).
			AddRow(`{"id":"sm-1","submodelElements":[]}`, false, "previous"))
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
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history".*INNER JOIN "aas_history_payload" AS "payload".*ORDER BY "history"."history_id" DESC LIMIT 2`).
		WillReturnRows(sqlmock.NewRows([]string{
			"history_id",
			"identifier",
			"change_type",
			"snapshot",
			"deleted",
			"administration_created_at_text",
			"administration_updated_at_text",
			"operation_time",
		}).
			AddRow(int64(12), "aas-3", ChangeUpdated, `{"id":"aas-3"}`, false, nil, nil, operationTime).
			AddRow(int64(11), "aas-2", ChangeCreated, `{"id":"aas-2"}`, false, nil, nil, operationTime))

	rows, cursor, err := RecentRows(context.Background(), db, TableAAS, 1, "", time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(12), rows[0].HistoryID)
	require.Equal(t, operationTime.Format(time.RFC3339Nano), rows[0].CreatedAt)
	require.Equal(t, operationTime.Format(time.RFC3339Nano), rows[0].UpdatedAt)
	require.Equal(t, "12", cursor)

	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history".*INNER JOIN "aas_history_payload" AS "payload".*WHERE \("history"\."history_id" < 12\).*ORDER BY "history"."history_id" DESC LIMIT 2`).
		WillReturnRows(sqlmock.NewRows([]string{
			"history_id",
			"identifier",
			"change_type",
			"snapshot",
			"deleted",
			"administration_created_at_text",
			"administration_updated_at_text",
			"operation_time",
		}).
			AddRow(int64(11), "aas-2", ChangeCreated, `{"id":"aas-2"}`, false, nil, nil, operationTime).
			AddRow(int64(10), "aas-1", ChangeCreated, `{"id":"aas-1"}`, false, nil, nil, operationTime))

	rows, cursor, err = RecentRows(context.Background(), db, TableAAS, 1, cursor, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(11), rows[0].HistoryID)
	require.Equal(t, "11", cursor)
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
