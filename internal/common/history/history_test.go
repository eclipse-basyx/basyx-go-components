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
	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

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
		WillReturnRows(newHistoryChainRows(TableAAS,
			historyChainRowSpec{HistoryID: 1, Identifier: "aas-1", ChangeType: ChangeCreated, PayloadType: PayloadTypeSnapshot, Snapshot: checkpoint, Deleted: false, OperationTime: operationTime},
			historyChainRowSpec{HistoryID: 2, Identifier: "aas-1", ChangeType: ChangeUpdated, PayloadType: PayloadTypeDiff, Patch: patch, ContentSnapshot: target, Deleted: false, OperationTime: operationTime},
		))

	snapshot, err := SnapshotByDate(context.Background(), db, TableAAS, "aas-1", operationTime)
	require.NoError(t, err)
	require.Equal(t, target, snapshot)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSnapshotByDateRestoresFromEarlySizeFallbackSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	operationTime := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	earlyCheckpoint := map[string]any{"items": []any{"f", "e", "d", "c", "b", "a"}}
	target := map[string]any{"items": []any{"f", "e", "d", "c", "b", "z"}}
	patch, err := BuildJSONPatch(earlyCheckpoint, target)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT "history"."history_id" FROM "aas_history" AS "history".*ORDER BY "history"."valid_from" DESC, "history"."history_id" DESC LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(3)))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(2)))
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(newHistoryChainRows(TableAAS,
			historyChainRowSpec{HistoryID: 2, Identifier: "aas-1", ChangeType: ChangeUpdated, PayloadType: PayloadTypeSnapshot, Snapshot: earlyCheckpoint, Deleted: false, OperationTime: operationTime},
			historyChainRowSpec{HistoryID: 3, Identifier: "aas-1", ChangeType: ChangeUpdated, PayloadType: PayloadTypeDiff, Patch: patch, ContentSnapshot: target, Deleted: false, OperationTime: operationTime},
		))

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

	mock.ExpectQuery(`SELECT "history"."history_id" FROM "aas_history" AS "history".*ORDER BY "history"."valid_from" DESC, "history"."history_id" DESC LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(newHistoryChainRowsWithTamper(TableAAS, func(_ int, values []driver.Value) {
			values[10] = "wrong-content-hash"
		},
			historyChainRowSpec{HistoryID: 1, Identifier: "aas-1", ChangeType: ChangeCreated, PayloadType: PayloadTypeSnapshot, Snapshot: snapshot, Deleted: false, OperationTime: operationTime},
		))

	_, err = SnapshotByDate(context.Background(), db, TableAAS, "aas-1", operationTime)
	require.Error(t, err)
	require.Contains(t, err.Error(), "HISTORY-RESTORE-CONTENTHASH")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSnapshotByDateRejectsRowHashMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	operationTime := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	snapshot := map[string]any{"id": "aas-1"}

	mock.ExpectQuery(`SELECT "history"."history_id" FROM "aas_history" AS "history".*ORDER BY "history"."valid_from" DESC, "history"."history_id" DESC LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(newHistoryChainRowsWithTamper(TableAAS, func(_ int, values []driver.Value) {
			values[13] = "wrong-row-hash"
		},
			historyChainRowSpec{HistoryID: 1, Identifier: "aas-1", ChangeType: ChangeCreated, PayloadType: PayloadTypeSnapshot, Snapshot: snapshot, Deleted: false, OperationTime: operationTime},
		))

	_, err = SnapshotByDate(context.Background(), db, TableAAS, "aas-1", operationTime)
	require.Error(t, err)
	require.Contains(t, err.Error(), "HISTORY-RESTORE-ROWHASH")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBaseVersionChainQueryNormalizesSourceIPForRowHashVerification(t *testing.T) {
	t.Parallel()

	historyAlias := goqu.T(TableAAS).As("history")
	payloadAlias := goqu.T("aas_history_payload").As("payload")

	query, _, err := baseVersionChainQuery(historyAlias, payloadAlias).ToSQL()
	require.NoError(t, err)
	require.Contains(t, query, `host("history"."source_ip")`)
	require.NotContains(t, query, `"history"."source_ip"::text`)
}

func TestSnapshotByDateRejectsBrokenRowHashChainLink(t *testing.T) {
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
		WillReturnRows(newHistoryChainRowsWithTamper(TableAAS, func(index int, values []driver.Value) {
			if index == 1 {
				values[12] = "wrong-previous-hash"
				values[13] = mustHistoryRowHashForHelper(ChangeEvent{
					EntityType:   TableAAS,
					Identifier:   "aas-1",
					ChangeType:   ChangeUpdated,
					Timestamp:    operationTime,
					PayloadType:  PayloadTypeDiff,
					ContentHash:  values[10].(string),
					PayloadHash:  values[11].(string),
					PreviousHash: "wrong-previous-hash",
				})
			}
		},
			historyChainRowSpec{HistoryID: 1, Identifier: "aas-1", ChangeType: ChangeCreated, PayloadType: PayloadTypeSnapshot, Snapshot: checkpoint, Deleted: false, OperationTime: operationTime},
			historyChainRowSpec{HistoryID: 2, Identifier: "aas-1", ChangeType: ChangeUpdated, PayloadType: PayloadTypeDiff, Patch: patch, ContentSnapshot: target, Deleted: false, OperationTime: operationTime},
		))

	_, err = SnapshotByDate(context.Background(), db, TableAAS, "aas-1", operationTime)
	require.Error(t, err)
	require.Contains(t, err.Error(), "HISTORY-RESTORE-CHAINLINK")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRestoreSnapshotPayloadPreservesLargeIntegerHashes(t *testing.T) {
	snapshotJSON := `{"id":"aas-1","serial":9007199254740993}`
	payloadHash, err := CanonicalJSONHash(json.RawMessage(snapshotJSON))
	require.NoError(t, err)

	snapshot, err := restoreSnapshotPayload(storedHistoryRow{
		Snapshot:    sql.NullString{String: snapshotJSON, Valid: true},
		PayloadHash: sql.NullString{String: payloadHash, Valid: true},
	})

	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), snapshot["serial"])
}

func TestRestoreDiffPayloadPreservesLargeIntegerHashes(t *testing.T) {
	base := map[string]any{"id": "aas-1", "serial": json.Number("9007199254740993")}
	diffJSON := `[{"op":"replace","path":"/serial","value":9007199254740995}]`
	payloadHash, err := CanonicalJSONHash(json.RawMessage(diffJSON))
	require.NoError(t, err)

	snapshot, err := restoreDiffPayload(base, storedHistoryRow{
		Diff:        sql.NullString{String: diffJSON, Valid: true},
		PayloadHash: sql.NullString{String: payloadHash, Valid: true},
	})

	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740995"), snapshot["serial"])
}

func TestDecodeJSONPreservingNumbersRejectsTrailingTokens(t *testing.T) {
	var snapshot map[string]any

	err := decodeJSONPreservingNumbers([]byte(`{"id":"aas-1"}{"id":"aas-2"}`), &snapshot)

	require.Error(t, err)
}

func expectLatestSnapshotRestore(mock sqlmock.Sqlmock, table string, payloadTable string, identifier string, historyID int64, snapshot map[string]any, deleted bool) {
	mock.ExpectQuery(`SELECT "history_id" FROM "` + table + `"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(historyID))
	expectSnapshotRestore(mock, table, payloadTable, identifier, historyID, snapshot, deleted, time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC))
}

func expectSnapshotRestore(mock sqlmock.Sqlmock, table string, payloadTable string, identifier string, historyID int64, snapshot map[string]any, deleted bool, operationTime time.Time) {
	mock.ExpectQuery(`SELECT "history_id" FROM "` + table + `".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(historyID))
	mock.ExpectQuery(`SELECT .*FROM "` + table + `" AS "history" INNER JOIN "` + payloadTable + `" AS "payload"`).
		WillReturnRows(newHistoryChainRows(table,
			historyChainRowSpec{HistoryID: historyID, Identifier: identifier, ChangeType: ChangeUpdated, PayloadType: PayloadTypeSnapshot, Snapshot: snapshot, Deleted: deleted, OperationTime: operationTime},
		))
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
		"previous_hash",
		"row_hash",
		"request_id",
		"correlation_id",
		"actor_subject",
		"actor_issuer",
		"client_id",
		"authorization_result",
		"policy_id",
		"matched_rule_id",
		"source_ip",
		"user_agent",
		"operation",
		"endpoint",
		"http_method",
	}
}

type historyChainRowSpec struct {
	HistoryID       int64
	Identifier      string
	ChangeType      string
	PayloadType     string
	Snapshot        map[string]any
	Patch           []map[string]any
	ContentSnapshot map[string]any
	Deleted         bool
	OperationTime   time.Time
	Audit           AuditContext
}

func newHistoryChainRows(table string, specs ...historyChainRowSpec) *sqlmock.Rows {
	rows := sqlmock.NewRows(historyChainColumns())
	previousByIdentifier := make(map[string]string)
	for _, spec := range specs {
		values, rowHash := historyChainRow(table, spec, previousByIdentifier[spec.Identifier])
		rows.AddRow(values...)
		previousByIdentifier[spec.Identifier] = rowHash
	}
	return rows
}

func newHistoryChainRowsWithTamper(table string, tamper func(index int, values []driver.Value), specs ...historyChainRowSpec) *sqlmock.Rows {
	rows := sqlmock.NewRows(historyChainColumns())
	previousByIdentifier := make(map[string]string)
	for index, spec := range specs {
		values, rowHash := historyChainRow(table, spec, previousByIdentifier[spec.Identifier])
		tamper(index, values)
		rows.AddRow(values...)
		previousByIdentifier[spec.Identifier] = rowHash
	}
	return rows
}

func historyChainRow(table string, spec historyChainRowSpec, previousHash string) ([]driver.Value, string) {
	var snapshotText any
	var diffText any
	var payloadValue any
	contentSnapshot := spec.ContentSnapshot
	if spec.PayloadType == PayloadTypeSnapshot {
		snapshotText = mustJSONTextForHelper(spec.Snapshot)
		payloadValue = spec.Snapshot
		contentSnapshot = spec.Snapshot
	} else {
		diffText = mustJSONTextForHelper(spec.Patch)
		payloadValue = spec.Patch
		if contentSnapshot == nil {
			panic("diff history test row requires reconstructed content snapshot")
		}
	}
	contentHash := mustHashForHelper(contentSnapshot)
	payloadHash := mustHashForHelper(payloadValue)
	rowHash := mustHistoryRowHashForHelper(ChangeEvent{
		EntityType:          table,
		Identifier:          spec.Identifier,
		ChangeType:          spec.ChangeType,
		Timestamp:           spec.OperationTime,
		Deleted:             spec.Deleted,
		RequestID:           spec.Audit.RequestID,
		CorrelationID:       spec.Audit.CorrelationID,
		ActorSubject:        spec.Audit.ActorSubject,
		ActorIssuer:         spec.Audit.ActorIssuer,
		ClientID:            spec.Audit.ClientID,
		AuthorizationResult: spec.Audit.AuthorizationResult,
		PolicyID:            spec.Audit.PolicyID,
		MatchedRuleID:       spec.Audit.MatchedRuleID,
		SourceIP:            spec.Audit.SourceIP,
		UserAgent:           spec.Audit.UserAgent,
		Operation:           spec.Audit.Operation,
		Endpoint:            spec.Audit.Endpoint,
		HTTPMethod:          spec.Audit.HTTPMethod,
		PayloadType:         spec.PayloadType,
		ContentHash:         contentHash,
		PayloadHash:         payloadHash,
		PreviousHash:        previousHash,
	})
	return []driver.Value{
		spec.HistoryID,
		spec.Identifier,
		spec.ChangeType,
		spec.PayloadType,
		snapshotText,
		diffText,
		spec.Deleted,
		nil,
		nil,
		spec.OperationTime,
		contentHash,
		payloadHash,
		previousHash,
		rowHash,
		nullableStringValue(spec.Audit.RequestID),
		nullableStringValue(spec.Audit.CorrelationID),
		nullableStringValue(spec.Audit.ActorSubject),
		nullableStringValue(spec.Audit.ActorIssuer),
		nullableStringValue(spec.Audit.ClientID),
		nullableStringValue(spec.Audit.AuthorizationResult),
		nullableStringValue(spec.Audit.PolicyID),
		nullableStringValue(spec.Audit.MatchedRuleID),
		nullableStringValue(spec.Audit.SourceIP),
		nullableStringValue(spec.Audit.UserAgent),
		nullableStringValue(spec.Audit.Operation),
		nullableStringValue(spec.Audit.Endpoint),
		nullableStringValue(spec.Audit.HTTPMethod),
	}, rowHash
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

func mustHistoryRowHashForHelper(event ChangeEvent) string {
	hash, err := ComputeHistoryRowHash(event)
	if err != nil {
		panic(err)
	}
	return hash
}

func nullableStringValue(value string) driver.Value {
	if value == "" {
		return nil
	}
	return value
}
