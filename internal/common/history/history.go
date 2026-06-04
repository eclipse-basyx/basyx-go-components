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

// Package history stores append-only snapshots for v3.2 history and recent-change endpoints.
package history

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

type latestVersion struct {
	historyID         int64
	snapshot          map[string]any
	deleted           bool
	rowHash           string
	rowsSinceSnapshot int
}

type historyQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type historyPayload struct {
	payloadType string
	json        []byte
	hash        string
}

type storedHistoryRow struct {
	EntityType          string
	HistoryID           int64
	Identifier          string
	ChangeType          string
	PayloadType         string
	Snapshot            sql.NullString
	Diff                sql.NullString
	Deleted             bool
	CreatedAt           sql.NullString
	UpdatedAt           sql.NullString
	OperationAt         time.Time
	ContentHash         sql.NullString
	PayloadHash         sql.NullString
	PreviousHash        sql.NullString
	RowHash             sql.NullString
	RequestID           sql.NullString
	CorrelationID       sql.NullString
	ActorSubject        sql.NullString
	ActorIssuer         sql.NullString
	ClientID            sql.NullString
	AuthorizationResult sql.NullString
	PolicyID            sql.NullString
	MatchedRuleID       sql.NullString
	SourceIP            sql.NullString
	UserAgent           sql.NullString
	Operation           sql.NullString
	Endpoint            sql.NullString
	HTTPMethod          sql.NullString
}

func latestVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string) (latestVersion, error) {
	query, args, err := goqu.From(table).
		Select(goqu.C("history_id")).
		Where(goqu.C("identifier").Eq(identifier)).
		Order(goqu.C("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return latestVersion{}, common.NewInternalServerError("HISTORY-MUTATE-BUILDLATESTID " + err.Error())
	}

	var historyID int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&historyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return latestVersion{}, common.NewErrNotFound("HISTORY-MUTATE-NOTFOUND no historical version found")
		}
		return latestVersion{}, common.NewInternalServerError("HISTORY-MUTATE-READLATESTID " + err.Error())
	}

	return restoreVersionByHistoryID(ctx, tx, table, identifier, historyID)
}

// SnapshotByDate returns the snapshot that was valid for identifier at the requested instant.
func SnapshotByDate(ctx context.Context, db *sql.DB, table string, identifier string, at time.Time) (map[string]any, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("HISTORY-GET-NILDB database handle must not be nil")
	}
	historyAlias := goqu.T(table).As("history")
	query, args, err := goqu.From(historyAlias).
		Select(historyAlias.Col("history_id")).
		Where(
			historyAlias.Col("identifier").Eq(identifier),
			historyAlias.Col("valid_from").Lte(at.UTC()),
		).
		Order(historyAlias.Col("valid_from").Desc(), historyAlias.Col("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-GET-BUILDSQL " + err.Error())
	}
	var historyID int64
	if err = db.QueryRowContext(ctx, query, args...).Scan(&historyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("HISTORY-GET-NOTFOUND no historical version found")
		}
		return nil, common.NewInternalServerError("HISTORY-GET-EXECSQL " + err.Error())
	}

	version, err := restoreVersionByHistoryID(ctx, db, table, identifier, historyID)
	if err != nil {
		return nil, err
	}
	if version.deleted {
		return nil, common.NewErrNotFound("HISTORY-GET-DELETED historical version is deleted at the requested date")
	}
	return version.snapshot, nil
}

func restoreVersionByHistoryID(ctx context.Context, queryer historyQueryer, table string, identifier string, historyID int64) (latestVersion, error) {
	versions, err := restoreVersionsThroughHistoryID(ctx, queryer, table, identifier, historyID)
	if err != nil {
		return latestVersion{}, err
	}
	version, ok := versions[historyID]
	if !ok {
		return latestVersion{}, common.NewInternalServerError("HISTORY-RESTORE-MISSINGTARGET restored chain does not contain requested history row")
	}
	return version, nil
}

func restoreVersionsThroughHistoryID(ctx context.Context, queryer historyQueryer, table string, identifier string, historyID int64) (map[int64]latestVersion, error) {
	payloadTable, err := historyPayloadTable(table)
	if err != nil {
		return nil, err
	}
	checkpointID, err := nearestSnapshotHistoryID(ctx, queryer, table, identifier, historyID)
	if err != nil {
		return nil, err
	}
	rows, err := loadVersionChain(ctx, queryer, table, payloadTable, identifier, checkpointID, historyID)
	if err != nil {
		return nil, err
	}
	return restoreVersionChainRows(rows)
}

func nearestSnapshotHistoryID(ctx context.Context, queryer historyQueryer, table string, identifier string, historyID int64) (int64, error) {
	query, args, err := goqu.From(table).
		Select(goqu.C("history_id")).
		Where(
			goqu.C("identifier").Eq(identifier),
			goqu.C("history_id").Lte(historyID),
			goqu.C("payload_type").Eq(PayloadTypeSnapshot),
		).
		Order(goqu.C("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("HISTORY-RESTORE-BUILDCHECKPOINT " + err.Error())
	}
	var checkpointID int64
	if err = queryer.QueryRowContext(ctx, query, args...).Scan(&checkpointID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewInternalServerError("HISTORY-RESTORE-NOCHECKPOINT no full snapshot checkpoint found")
		}
		return 0, common.NewInternalServerError("HISTORY-RESTORE-READCHECKPOINT " + err.Error())
	}
	return checkpointID, nil
}

func loadVersionChain(
	ctx context.Context,
	queryer historyQueryer,
	table string,
	payloadTable string,
	identifier string,
	checkpointID int64,
	historyID int64,
) ([]storedHistoryRow, error) {
	historyAlias := goqu.T(table).As("history")
	payloadAlias := goqu.T(payloadTable).As("payload")
	query, args, err := baseVersionChainQuery(historyAlias, payloadAlias).
		Where(
			historyAlias.Col("identifier").Eq(identifier),
			historyAlias.Col("history_id").Gte(checkpointID),
			historyAlias.Col("history_id").Lte(historyID),
		).
		Order(historyAlias.Col("history_id").Asc()).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-BUILDCHAIN " + err.Error())
	}
	sqlRows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-EXECCHAIN " + err.Error())
	}
	defer func() {
		_ = sqlRows.Close()
	}()
	return scanStoredHistoryRows(sqlRows, table)
}

func baseVersionChainQuery(historyAlias exp.AliasedExpression, payloadAlias exp.AliasedExpression) *goqu.SelectDataset {
	return goqu.From(historyAlias).
		InnerJoin(payloadAlias, goqu.On(historyAlias.Col("history_id").Eq(payloadAlias.Col("history_id")))).
		Select(
			historyAlias.Col("history_id"),
			historyAlias.Col("identifier"),
			historyAlias.Col("change_type"),
			historyAlias.Col("payload_type"),
			goqu.L(`"payload"."snapshot"::text`),
			goqu.L(`"payload"."diff"::text`),
			historyAlias.Col("deleted"),
			historyAlias.Col("administration_created_at_text"),
			historyAlias.Col("administration_updated_at_text"),
			historyAlias.Col("operation_time"),
			historyAlias.Col("content_hash"),
			historyAlias.Col("payload_hash"),
			historyAlias.Col("previous_hash"),
			historyAlias.Col("row_hash"),
			historyAlias.Col("request_id"),
			historyAlias.Col("correlation_id"),
			historyAlias.Col("actor_subject"),
			historyAlias.Col("actor_issuer"),
			historyAlias.Col("client_id"),
			historyAlias.Col("authorization_result"),
			historyAlias.Col("policy_id"),
			historyAlias.Col("matched_rule_id"),
			goqu.L(`"history"."source_ip"::text`),
			historyAlias.Col("user_agent"),
			historyAlias.Col("operation"),
			historyAlias.Col("endpoint"),
			historyAlias.Col("http_method"),
		)
}

func scanStoredHistoryRows(sqlRows *sql.Rows, table string) ([]storedHistoryRow, error) {
	rows := make([]storedHistoryRow, 0)
	for sqlRows.Next() {
		var row storedHistoryRow
		if err := sqlRows.Scan(
			&row.HistoryID,
			&row.Identifier,
			&row.ChangeType,
			&row.PayloadType,
			&row.Snapshot,
			&row.Diff,
			&row.Deleted,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.OperationAt,
			&row.ContentHash,
			&row.PayloadHash,
			&row.PreviousHash,
			&row.RowHash,
			&row.RequestID,
			&row.CorrelationID,
			&row.ActorSubject,
			&row.ActorIssuer,
			&row.ClientID,
			&row.AuthorizationResult,
			&row.PolicyID,
			&row.MatchedRuleID,
			&row.SourceIP,
			&row.UserAgent,
			&row.Operation,
			&row.Endpoint,
			&row.HTTPMethod,
		); err != nil {
			return nil, common.NewInternalServerError("HISTORY-RESTORE-SCANCHAIN " + err.Error())
		}
		row.EntityType = table
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-ROWS " + err.Error())
	}
	if len(rows) == 0 {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-EMPTYCHAIN no history rows found for restore")
	}
	return rows, nil
}

func restoreVersionChainRows(rows []storedHistoryRow) (map[int64]latestVersion, error) {
	var snapshot map[string]any
	rowsSinceSnapshot := 0
	versions := make(map[int64]latestVersion, len(rows))
	previousRowHash := ""
	hasPreviousRow := false
	for _, row := range rows {
		var err error
		switch row.PayloadType {
		case PayloadTypeSnapshot:
			snapshot, err = restoreSnapshotPayload(row)
			rowsSinceSnapshot = 1
		case PayloadTypeDiff:
			if snapshot == nil {
				return nil, common.NewInternalServerError("HISTORY-RESTORE-DIFFWITHOUTBASE diff row has no preceding snapshot")
			}
			snapshot, err = restoreDiffPayload(snapshot, row)
			rowsSinceSnapshot++
		default:
			return nil, common.NewInternalServerError("HISTORY-RESTORE-PAYLOADTYPE unsupported payload type '" + row.PayloadType + "'")
		}
		if err != nil {
			return nil, err
		}
		if err = verifyCanonicalHash(snapshot, row.ContentHash, "HISTORY-RESTORE-CONTENTHASH"); err != nil {
			return nil, err
		}
		if err = verifyStoredHistoryRowHash(row); err != nil {
			return nil, err
		}
		if hasPreviousRow {
			if err = verifyStoredHistoryChainLink(row, previousRowHash); err != nil {
				return nil, err
			}
		}
		versionSnapshot, cloneErr := cloneSnapshotMap(snapshot)
		if cloneErr != nil {
			return nil, cloneErr
		}
		versions[row.HistoryID] = latestVersion{
			historyID:         row.HistoryID,
			snapshot:          versionSnapshot,
			deleted:           row.Deleted,
			rowHash:           row.RowHash.String,
			rowsSinceSnapshot: rowsSinceSnapshot,
		}
		previousRowHash = nullStringValue(row.RowHash)
		hasPreviousRow = true
	}
	if len(versions) == 0 {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-NOLATEST no latest row found after restore")
	}
	return versions, nil
}

func verifyStoredHistoryChainLink(row storedHistoryRow, previousRowHash string) error {
	if previousRowHash == "" {
		return nil
	}
	if nullStringValue(row.PreviousHash) != previousRowHash {
		return common.NewInternalServerError("HISTORY-RESTORE-CHAINLINK previous hash does not match preceding restored row")
	}
	return nil
}

func verifyStoredHistoryRowHash(row storedHistoryRow) error {
	expected := strings.TrimSpace(nullStringValue(row.RowHash))
	if expected == "" {
		return nil
	}
	event := historyRowHashEvent(row)
	matches, err := historyRowHashMatches(event, expected)
	if err != nil {
		return common.NewInternalServerError("HISTORY-RESTORE-ROWHASH " + err.Error())
	}
	if matches {
		return nil
	}
	return common.NewInternalServerError("HISTORY-RESTORE-ROWHASH stored row hash does not match row metadata")
}

func historyRowHashMatches(event ChangeEvent, expected string) (bool, error) {
	actual, err := ComputeHistoryRowHash(event)
	if err != nil {
		return false, err
	}
	if actual == expected {
		return true, nil
	}
	legacy, err := computeLegacyHistoryRowHash(event)
	if err != nil {
		return false, err
	}
	return legacy == expected, nil
}

func historyRowHashEvent(row storedHistoryRow) ChangeEvent {
	return ChangeEvent{
		EntityType:          row.EntityType,
		Identifier:          row.Identifier,
		ChangeType:          row.ChangeType,
		Timestamp:           row.OperationAt,
		Deleted:             row.Deleted,
		RequestID:           nullStringValue(row.RequestID),
		CorrelationID:       nullStringValue(row.CorrelationID),
		ActorSubject:        nullStringValue(row.ActorSubject),
		ActorIssuer:         nullStringValue(row.ActorIssuer),
		ClientID:            nullStringValue(row.ClientID),
		AuthorizationResult: nullStringValue(row.AuthorizationResult),
		PolicyID:            nullStringValue(row.PolicyID),
		MatchedRuleID:       nullStringValue(row.MatchedRuleID),
		SourceIP:            nullStringValue(row.SourceIP),
		UserAgent:           nullStringValue(row.UserAgent),
		Operation:           nullStringValue(row.Operation),
		Endpoint:            nullStringValue(row.Endpoint),
		HTTPMethod:          nullStringValue(row.HTTPMethod),
		PayloadType:         row.PayloadType,
		ContentHash:         nullStringValue(row.ContentHash),
		PayloadHash:         nullStringValue(row.PayloadHash),
		PreviousHash:        nullStringValue(row.PreviousHash),
		RowHash:             nullStringValue(row.RowHash),
	}
}

func restoreSnapshotPayload(row storedHistoryRow) (map[string]any, error) {
	if !row.Snapshot.Valid {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-MISSINGSNAPSHOT snapshot payload is missing")
	}
	var snapshot map[string]any
	if err := decodeJSONPreservingNumbers([]byte(row.Snapshot.String), &snapshot); err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-UNMARSHALSNAPSHOT " + err.Error())
	}
	if err := verifyCanonicalHash(snapshot, row.PayloadHash, "HISTORY-RESTORE-SNAPSHOTPAYLOADHASH"); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func restoreDiffPayload(base map[string]any, row storedHistoryRow) (map[string]any, error) {
	if !row.Diff.Valid {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-MISSINGDIFF diff payload is missing")
	}
	var patch []map[string]any
	if err := decodeJSONPreservingNumbers([]byte(row.Diff.String), &patch); err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-UNMARSHALDIFF " + err.Error())
	}
	if err := verifyCanonicalHash(patch, row.PayloadHash, "HISTORY-RESTORE-DIFFPAYLOADHASH"); err != nil {
		return nil, err
	}
	snapshot, err := ApplyJSONPatch(base, patch)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func verifyCanonicalHash(value any, expected sql.NullString, errorCode string) error {
	if !expected.Valid || strings.TrimSpace(expected.String) == "" {
		return common.NewInternalServerError(errorCode + " expected hash is missing")
	}
	actual, err := CanonicalJSONHash(value)
	if err != nil {
		return common.NewInternalServerError(errorCode + " " + err.Error())
	}
	if actual != expected.String {
		return common.NewInternalServerError(errorCode + " stored hash does not match reconstructed payload")
	}
	return nil
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func historyPayloadTable(table string) (string, error) {
	payloadTable, ok := payloadTables[table]
	if !ok {
		return "", common.NewInternalServerError("HISTORY-PAYLOADTABLE-UNSUPPORTED unsupported history table '" + table + "'")
	}
	return payloadTable, nil
}
