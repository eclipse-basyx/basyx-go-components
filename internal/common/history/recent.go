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
	"strconv"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

type recentRestoreTarget struct {
	Identifier string
	HistoryID  int64
}

type recentRestoreKey struct {
	Identifier string
	HistoryID  int64
}

type restoreChainKey struct {
	Identifier   string
	CheckpointID int64
}

type restoreChainGroup struct {
	key          restoreChainKey
	maxHistoryID int64
	targetIDs    map[int64]struct{}
	rows         []storedHistoryRow
}

// RecentRows returns history rows before cursor, ordered from newest to oldest with one look-ahead row for pagination.
func RecentRows(ctx context.Context, db *sql.DB, table string, limit int32, cursor string, createdFrom time.Time, updatedFrom time.Time) ([]Row, string, error) {
	if db == nil {
		return nil, "", common.NewErrBadRequest("HISTORY-RECENT-NILDB database handle must not be nil")
	}
	limit, err := normalizeRecentChangesLimit(limit)
	if err != nil {
		return nil, "", err
	}
	limitInt := int(limit)
	cursorID, err := parseCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	historyAlias := goqu.T(table).As("history")
	query := goqu.From(historyAlias).
		Select(
			historyAlias.Col("history_id"),
			historyAlias.Col("identifier"),
			historyAlias.Col("change_type"),
			historyAlias.Col("deleted"),
			historyAlias.Col("administration_created_at_text"),
			historyAlias.Col("administration_updated_at_text"),
			historyAlias.Col("operation_time"),
		).
		Order(historyAlias.Col("history_id").Desc()).
		Limit(uint(limitInt + 1)) //nolint:gosec // limit is positive int32 and therefore safe on supported platforms.
	if cursorID > 0 {
		query = query.Where(historyAlias.Col("history_id").Lt(cursorID))
	}
	if !createdFrom.IsZero() {
		query = query.Where(goqu.Or(
			historyAlias.Col("operation_time").Gte(createdFrom.UTC()),
			historyAlias.Col("administration_created_at").Gte(createdFrom.UTC()),
		))
	}
	if !updatedFrom.IsZero() {
		query = query.Where(goqu.Or(
			historyAlias.Col("operation_time").Gte(updatedFrom.UTC()),
			historyAlias.Col("administration_updated_at").Gte(updatedFrom.UTC()),
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
		var created sql.NullString
		var updated sql.NullString
		if err = rows.Scan(&row.HistoryID, &row.Identifier, &row.ChangeType, &row.Deleted, &created, &updated, &row.OperationAt); err != nil {
			return nil, "", common.NewInternalServerError("HISTORY-RECENT-SCAN " + err.Error())
		}
		if len(result) == limitInt {
			nextCursor = strconv.FormatInt(result[len(result)-1].HistoryID, 10)
			break
		}
		row.CreatedAt = timestampOrOperationTime(created.String, row.OperationAt)
		row.UpdatedAt = timestampOrOperationTime(updated.String, row.OperationAt)
		result = append(result, row)
	}
	if err = rows.Err(); err != nil {
		return nil, "", common.NewInternalServerError("HISTORY-RECENT-ROWS " + err.Error())
	}
	if err = restoreRecentRowSnapshots(ctx, db, table, result); err != nil {
		return nil, "", err
	}
	return result, nextCursor, nil
}

func restoreRecentRowSnapshots(ctx context.Context, queryer historyQueryer, table string, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	targets := recentRestoreTargets(rows)
	versions, err := restoreRecentVersions(ctx, queryer, table, targets)
	if err != nil {
		return err
	}
	for index := range rows {
		key := recentRestoreKey{Identifier: rows[index].Identifier, HistoryID: rows[index].HistoryID}
		version, ok := versions[key]
		if !ok {
			return common.NewInternalServerError("HISTORY-RECENT-MISSINGRESTORE restored history rows do not contain requested recent row")
		}
		rows[index].Snapshot = version.snapshot
	}
	return nil
}

func recentRestoreTargets(rows []Row) []recentRestoreTarget {
	targets := make([]recentRestoreTarget, 0, len(rows))
	seen := make(map[recentRestoreKey]struct{}, len(rows))
	for _, row := range rows {
		key := recentRestoreKey{Identifier: row.Identifier, HistoryID: row.HistoryID}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, recentRestoreTarget{Identifier: row.Identifier, HistoryID: row.HistoryID})
	}
	return targets
}

func restoreRecentVersions(ctx context.Context, queryer historyQueryer, table string, targets []recentRestoreTarget) (map[recentRestoreKey]latestVersion, error) {
	if len(targets) == 0 {
		return map[recentRestoreKey]latestVersion{}, nil
	}
	payloadTable, err := historyPayloadTable(table)
	if err != nil {
		return nil, err
	}
	checkpoints, err := nearestSnapshotHistoryIDs(ctx, queryer, table, targets)
	if err != nil {
		return nil, err
	}
	groups, err := buildRestoreChainGroups(targets, checkpoints)
	if err != nil {
		return nil, err
	}
	chainRows, err := loadVersionChains(ctx, queryer, table, payloadTable, groups)
	if err != nil {
		return nil, err
	}
	assignVersionChainRows(groups, chainRows)
	return restoreRecentVersionGroups(groups)
}

func nearestSnapshotHistoryIDs(ctx context.Context, queryer historyQueryer, table string, targets []recentRestoreTarget) (map[recentRestoreKey]int64, error) {
	historyAlias := goqu.T(table).As("history")
	targetAlias := goqu.T("targets").As("target")
	query, args, err := goqu.From(historyAlias).
		With("targets", recentRestoreTargetDataset(targets)).
		Select(
			targetAlias.Col("identifier"),
			targetAlias.Col("history_id"),
			goqu.MAX(historyAlias.Col("history_id")).As("checkpoint_id"),
		).
		InnerJoin(targetAlias, goqu.On(
			historyAlias.Col("identifier").Eq(targetAlias.Col("identifier")),
			historyAlias.Col("history_id").Lte(targetAlias.Col("history_id")),
		)).
		Where(historyAlias.Col("payload_type").Eq(PayloadTypeSnapshot)).
		GroupBy(targetAlias.Col("identifier"), targetAlias.Col("history_id")).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-BUILDCHECKPOINTS " + err.Error())
	}
	sqlRows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-READCHECKPOINTS " + err.Error())
	}
	defer func() {
		_ = sqlRows.Close()
	}()

	checkpoints := make(map[recentRestoreKey]int64, len(targets))
	for sqlRows.Next() {
		var identifier string
		var historyID int64
		var checkpointID int64
		if err = sqlRows.Scan(&identifier, &historyID, &checkpointID); err != nil {
			return nil, common.NewInternalServerError("HISTORY-RESTORE-SCANCHECKPOINTS " + err.Error())
		}
		checkpoints[recentRestoreKey{Identifier: identifier, HistoryID: historyID}] = checkpointID
	}
	if err = sqlRows.Err(); err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-CHECKPOINTROWS " + err.Error())
	}

	for _, target := range targets {
		if _, exists := checkpoints[recentRestoreKey(target)]; !exists {
			return nil, common.NewInternalServerError("HISTORY-RESTORE-NOCHECKPOINT no full snapshot checkpoint found")
		}
	}
	return checkpoints, nil
}

func recentRestoreTargetDataset(targets []recentRestoreTarget) *goqu.SelectDataset {
	targetRows := recentRestoreTargetRowDataset(targets[0])
	for _, target := range targets[1:] {
		targetRows = targetRows.UnionAll(recentRestoreTargetRowDataset(target))
	}
	return targetRows
}

func recentRestoreTargetRowDataset(target recentRestoreTarget) *goqu.SelectDataset {
	return goqu.From().Select(
		goqu.V(target.Identifier).As("identifier"),
		goqu.V(target.HistoryID).As("history_id"),
	)
}

func buildRestoreChainGroups(targets []recentRestoreTarget, checkpoints map[recentRestoreKey]int64) ([]*restoreChainGroup, error) {
	groups := make([]*restoreChainGroup, 0, len(targets))
	groupByKey := make(map[restoreChainKey]*restoreChainGroup, len(targets))
	for _, target := range targets {
		targetKey := recentRestoreKey(target)
		checkpointID, ok := checkpoints[targetKey]
		if !ok {
			return nil, common.NewInternalServerError("HISTORY-RESTORE-MISSINGCHECKPOINT checkpoint lookup did not return requested target")
		}
		groupKey := restoreChainKey{Identifier: target.Identifier, CheckpointID: checkpointID}
		group := groupByKey[groupKey]
		if group == nil {
			group = &restoreChainGroup{
				key:          groupKey,
				maxHistoryID: target.HistoryID,
				targetIDs:    make(map[int64]struct{}),
			}
			groupByKey[groupKey] = group
			groups = append(groups, group)
		}
		if target.HistoryID > group.maxHistoryID {
			group.maxHistoryID = target.HistoryID
		}
		group.targetIDs[target.HistoryID] = struct{}{}
	}
	return groups, nil
}

func loadVersionChains(ctx context.Context, queryer historyQueryer, table string, payloadTable string, groups []*restoreChainGroup) ([]storedHistoryRow, error) {
	conditions := make([]goqu.Expression, 0, len(groups))
	historyAlias := goqu.T(table).As("history")
	payloadAlias := goqu.T(payloadTable).As("payload")
	for _, group := range groups {
		conditions = append(conditions, goqu.And(
			historyAlias.Col("identifier").Eq(group.key.Identifier),
			historyAlias.Col("history_id").Gte(group.key.CheckpointID),
			historyAlias.Col("history_id").Lte(group.maxHistoryID),
		))
	}
	query, args, err := baseVersionChainQuery(historyAlias, payloadAlias).
		Where(goqu.Or(conditions...)).
		Order(historyAlias.Col("identifier").Asc(), historyAlias.Col("history_id").Asc()).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-BUILDCHAINS " + err.Error())
	}
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-EXECCHAINS " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()
	return scanStoredHistoryRows(rows, table)
}

func assignVersionChainRows(groups []*restoreChainGroup, rows []storedHistoryRow) {
	groupsByIdentifier := make(map[string][]*restoreChainGroup, len(groups))
	for _, group := range groups {
		groupsByIdentifier[group.key.Identifier] = append(groupsByIdentifier[group.key.Identifier], group)
	}
	for _, row := range rows {
		for _, group := range groupsByIdentifier[row.Identifier] {
			if row.HistoryID < group.key.CheckpointID || row.HistoryID > group.maxHistoryID {
				continue
			}
			group.rows = append(group.rows, row)
		}
	}
}

func restoreRecentVersionGroups(groups []*restoreChainGroup) (map[recentRestoreKey]latestVersion, error) {
	versionsByTarget := make(map[recentRestoreKey]latestVersion)
	for _, group := range groups {
		versions, err := restoreVersionChainRows(group.rows)
		if err != nil {
			return nil, err
		}
		for targetID := range group.targetIDs {
			version, ok := versions[targetID]
			if !ok {
				return nil, common.NewInternalServerError("HISTORY-RESTORE-MISSINGTARGET restored chain does not contain requested history row")
			}
			versionsByTarget[recentRestoreKey{Identifier: group.key.Identifier, HistoryID: targetID}] = version
		}
	}
	return versionsByTarget, nil
}

// FilterRecentRows fills a filtered page without exposing empty intermediary raw pages.
func FilterRecentRows(limit int32, cursor string, fetch RecentRowsFetcher, include RecentRowPredicate) ([]Row, string, error) {
	limit, err := normalizeRecentChangesLimit(limit)
	if err != nil {
		return nil, "", err
	}
	if fetch == nil {
		return nil, "", common.NewErrBadRequest("HISTORY-FILTERRECENT-NILFETCH fetch function must not be nil")
	}
	if include == nil {
		return nil, "", common.NewErrBadRequest("HISTORY-FILTERRECENT-NILPREDICATE predicate must not be nil")
	}

	result := make([]Row, 0, int(limit))
	scanCursor := cursor
	for {
		rows, nextCursor, fetchErr := fetch(limit, scanCursor)
		if fetchErr != nil {
			return nil, "", fetchErr
		}
		for index, row := range rows {
			included, includeErr := include(row)
			if includeErr != nil {
				return nil, "", includeErr
			}
			if !included {
				continue
			}
			result = append(result, row)
			if len(result) == int(limit) {
				if index < len(rows)-1 || nextCursor != "" {
					return result, strconv.FormatInt(row.HistoryID, 10), nil
				}
				return result, "", nil
			}
		}
		if nextCursor == "" {
			return result, "", nil
		}
		if nextCursor == scanCursor {
			return nil, "", common.NewInternalServerError("HISTORY-FILTERRECENT-CURSOR raw history cursor did not advance")
		}
		scanCursor = nextCursor
	}
}

func timestampOrOperationTime(timestamp string, operationTime time.Time) string {
	if strings.TrimSpace(timestamp) != "" {
		return timestamp
	}
	return operationTime.UTC().Format(time.RFC3339Nano)
}

func normalizeRecentChangesLimit(limit int32) (int32, error) {
	if limit <= 0 {
		return DefaultRecentChangesLimit, nil
	}
	if limit > MaxRecentChangesLimit {
		return 0, common.NewErrBadRequest("HISTORY-RECENT-LIMIT limit must not exceed " + strconv.FormatInt(int64(MaxRecentChangesLimit), 10))
	}
	return limit, nil
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
