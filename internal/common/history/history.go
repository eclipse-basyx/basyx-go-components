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
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	// TableAAS stores Asset Administration Shell history snapshots.
	TableAAS = "aas_history"
	// TableSubmodel stores Submodel history snapshots.
	TableSubmodel = "submodel_history"
	// TableConcept stores Concept Description history snapshots.
	TableConcept = "concept_description_history"
	// TableDescriptor stores AAS descriptor history snapshots.
	TableDescriptor = "descriptor_history"

	// ChangeCreated marks a created entity version.
	ChangeCreated = "Created"
	// ChangeUpdated marks an updated entity version.
	ChangeUpdated = "Updated"
	// ChangeDeleted marks a deleted entity version.
	ChangeDeleted = "Deleted"
)

// Row is a normalized history entry loaded from one of the history tables.
type Row struct {
	HistoryID   int64
	Identifier  string
	ChangeType  string
	Snapshot    map[string]any
	Deleted     bool
	CreatedAt   string
	UpdatedAt   string
	OperationAt time.Time
}

// AppendVersionTx closes the current open version for identifier and appends a new snapshot version.
func AppendVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool) error {
	if tx == nil {
		return common.NewInternalServerError("HISTORY-APPEND-NILTX transaction must not be nil")
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return common.NewErrBadRequest("HISTORY-APPEND-EMPTYID identifier must not be empty")
	}

	now := time.Now().UTC()
	closeQuery, closeArgs, err := goqu.Update(table).
		Set(goqu.Record{"valid_to": now}).
		Where(goqu.C("identifier").Eq(identifier), goqu.C("valid_to").IsNull()).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-BUILDCLOSE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, closeQuery, closeArgs...); err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-EXECCLOSE " + err.Error())
	}

	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-MARSHAL " + err.Error())
	}
	createdAt, updatedAt := administrationTimestamps(snapshot)
	insertQuery, insertArgs, err := goqu.Insert(table).Rows(goqu.Record{
		"identifier":                     identifier,
		"change_type":                    changeType,
		"snapshot":                       goqu.L("?::jsonb", string(snapshotJSON)),
		"deleted":                        deleted,
		"valid_from":                     now,
		"operation_time":                 now,
		"administration_created_at_text": createdAt,
		"administration_updated_at_text": updatedAt,
	}).ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-BUILDINSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, insertQuery, insertArgs...); err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-EXECINSERT " + err.Error())
	}
	return nil
}

// SnapshotByDate returns the snapshot that was valid for identifier at the requested instant.
func SnapshotByDate(ctx context.Context, db *sql.DB, table string, identifier string, at time.Time) (map[string]any, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("HISTORY-GET-NILDB database handle must not be nil")
	}
	query, args, err := goqu.From(table).
		Select(goqu.L("snapshot::text"), goqu.C("deleted")).
		Where(
			goqu.C("identifier").Eq(identifier),
			goqu.C("valid_from").Lte(at.UTC()),
			goqu.Or(goqu.C("valid_to").IsNull(), goqu.C("valid_to").Gt(at.UTC())),
		).
		Order(goqu.C("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-GET-BUILDSQL " + err.Error())
	}
	var snapshotText string
	var deleted bool
	if err = db.QueryRowContext(ctx, query, args...).Scan(&snapshotText, &deleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("HISTORY-GET-NOTFOUND no historical version found")
		}
		return nil, common.NewInternalServerError("HISTORY-GET-EXECSQL " + err.Error())
	}
	if deleted {
		return nil, common.NewErrNotFound("HISTORY-GET-DELETED historical version is deleted at the requested date")
	}
	var snapshot map[string]any
	if err = json.Unmarshal([]byte(snapshotText), &snapshot); err != nil {
		return nil, common.NewInternalServerError("HISTORY-GET-UNMARSHAL " + err.Error())
	}
	return snapshot, nil
}

// RecentRows returns history rows after cursor, ordered by history id with one look-ahead row for pagination.
func RecentRows(ctx context.Context, db *sql.DB, table string, limit int32, cursor string, createdFrom time.Time, updatedFrom time.Time) ([]Row, string, error) {
	if db == nil {
		return nil, "", common.NewErrBadRequest("HISTORY-RECENT-NILDB database handle must not be nil")
	}
	if limit <= 0 {
		limit = 100
	}
	limitInt := int(limit)
	cursorID, err := parseCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	query := goqu.From(table).
		Select(
			goqu.C("history_id"),
			goqu.C("identifier"),
			goqu.C("change_type"),
			goqu.L("snapshot::text"),
			goqu.C("deleted"),
			goqu.C("administration_created_at_text"),
			goqu.C("administration_updated_at_text"),
			goqu.C("operation_time"),
		).
		Order(goqu.C("history_id").Asc()).
		Limit(uint(limitInt + 1)) //nolint:gosec // limit is positive int32 and therefore safe on supported platforms.
	if cursorID > 0 {
		query = query.Where(goqu.C("history_id").Gt(cursorID))
	}
	if !createdFrom.IsZero() {
		query = query.Where(goqu.Or(
			goqu.C("operation_time").Gte(createdFrom.UTC()),
			goqu.C("administration_created_at_text").Gte(createdFrom.Format(time.RFC3339Nano)),
		))
	}
	if !updatedFrom.IsZero() {
		query = query.Where(goqu.Or(
			goqu.C("operation_time").Gte(updatedFrom.UTC()),
			goqu.C("administration_updated_at_text").Gte(updatedFrom.Format(time.RFC3339Nano)),
		))
	}

	sqlQuery, args, err := query.ToSQL()
	if err != nil {
		return nil, "", common.NewInternalServerError("HISTORY-RECENT-BUILDSQL " + err.Error())
	}
	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("HISTORY-RECENT-EXECSQL " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	result := make([]Row, 0, limitInt)
	nextCursor := ""
	for rows.Next() {
		var row Row
		var snapshotText string
		var created sql.NullString
		var updated sql.NullString
		if err = rows.Scan(&row.HistoryID, &row.Identifier, &row.ChangeType, &snapshotText, &row.Deleted, &created, &updated, &row.OperationAt); err != nil {
			return nil, "", common.NewInternalServerError("HISTORY-RECENT-SCAN " + err.Error())
		}
		if len(result) == limitInt {
			nextCursor = strconv.FormatInt(row.HistoryID, 10)
			break
		}
		if err = json.Unmarshal([]byte(snapshotText), &row.Snapshot); err != nil {
			return nil, "", common.NewInternalServerError("HISTORY-RECENT-UNMARSHAL " + err.Error())
		}
		row.CreatedAt = created.String
		row.UpdatedAt = updated.String
		result = append(result, row)
	}
	if err = rows.Err(); err != nil {
		return nil, "", common.NewInternalServerError("HISTORY-RECENT-ROWS " + err.Error())
	}
	return result, nextCursor, nil
}

func parseCursor(cursor string) (int64, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil || value < 0 {
		return 0, common.NewErrBadRequest("HISTORY-CURSOR-INVALID cursor must be a non-negative history id")
	}
	return value, nil
}

func administrationTimestamps(snapshot map[string]any) (string, string) {
	administration, ok := snapshot["administration"].(map[string]any)
	if !ok {
		return "", ""
	}
	created, _ := administration["createdAt"].(string)
	updated, _ := administration["updatedAt"].(string)
	return created, updated
}
