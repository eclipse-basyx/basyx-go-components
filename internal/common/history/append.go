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
	"errors"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// AppendVersionTx appends an immutable snapshot event for identifier.
func AppendVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool) error {
	cfg := ActiveConfig()
	if cfg.Mode == ModeOff {
		return nil
	}

	identifier, err := validateAppendInputs(tx, identifier)
	if err != nil {
		return err
	}
	if err = lockIdentifierTx(ctx, tx, table, identifier); err != nil {
		return err
	}
	if cfg.FullSnapshotInterval == DefaultFullSnapshotInterval {
		previousHash, hashErr := latestRowHashTx(ctx, tx, table, identifier)
		if hashErr != nil {
			return hashErr
		}
		return appendSnapshotVersionWithPreviousHashTx(ctx, tx, table, identifier, changeType, snapshot, deleted, previousHash)
	}
	latest, err := latestVersionTx(ctx, tx, table, identifier)
	if err != nil && !common.IsErrNotFound(err) {
		return err
	}
	if common.IsErrNotFound(err) {
		return appendVersionWithLatestTx(ctx, tx, table, identifier, changeType, snapshot, deleted, nil)
	}
	return appendVersionWithLatestTx(ctx, tx, table, identifier, changeType, snapshot, deleted, &latest)
}

// AppendMutatedVersionTx derives and appends a version from the latest snapshot.
func AppendMutatedVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, mutate SnapshotMutator) error {
	if ActiveConfig().Mode == ModeOff {
		return nil
	}

	identifier, err := validateAppendInputs(tx, identifier)
	if err != nil {
		return err
	}
	if mutate == nil {
		return common.NewInternalServerError("HISTORY-MUTATE-NILMUTATOR snapshot mutator must not be nil")
	}
	if err = lockIdentifierTx(ctx, tx, table, identifier); err != nil {
		return err
	}

	latest, err := latestVersionTx(ctx, tx, table, identifier)
	if err != nil {
		return err
	}
	if latest.deleted {
		return common.NewErrNotFound("HISTORY-MUTATE-DELETED latest historical version is deleted")
	}
	currentSnapshot := latest.snapshot
	previousSnapshot, err := cloneSnapshotMap(latest.snapshot)
	if err != nil {
		return err
	}
	if err = mutate(currentSnapshot); err != nil {
		return err
	}
	previousVersion := latest
	previousVersion.snapshot = previousSnapshot
	return appendVersionWithLatestTx(ctx, tx, table, identifier, changeType, currentSnapshot, false, &previousVersion)
}

func validateAppendInputs(tx *sql.Tx, identifier string) (string, error) {
	if tx == nil {
		return "", common.NewInternalServerError("HISTORY-APPEND-NILTX transaction must not be nil")
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", common.NewErrBadRequest("HISTORY-APPEND-EMPTYID identifier must not be empty")
	}
	return identifier, nil
}

func appendVersionWithLatestTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool, latest *latestVersion) error {
	payload, err := buildHistoryPayload(snapshot, latest, ActiveConfig())
	if err != nil {
		return err
	}
	previousHash := ""
	if latest != nil {
		previousHash = latest.rowHash
	}
	return insertHistoryVersionTx(ctx, tx, table, identifier, changeType, snapshot, deleted, payload, previousHash)
}

func appendSnapshotVersionWithPreviousHashTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool, previousHash string) error {
	payload, err := buildSnapshotPayload(snapshot)
	if err != nil {
		return err
	}
	return insertHistoryVersionTx(ctx, tx, table, identifier, changeType, snapshot, deleted, payload, previousHash)
}

func insertHistoryVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool, payload historyPayload, previousHash string) error {
	payloadTable, err := historyPayloadTable(table)
	if err != nil {
		return err
	}
	now := databaseTimestamp(time.Now())
	contentHash, err := CanonicalJSONHash(snapshot)
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-CONTENTHASH " + err.Error())
	}
	audit := FromContext(ctx)
	event := ChangeEvent{
		EntityType:          table,
		Identifier:          identifier,
		ChangeType:          changeType,
		Timestamp:           now,
		Snapshot:            snapshot,
		Deleted:             deleted,
		PayloadType:         payload.payloadType,
		ContentHash:         contentHash,
		PayloadHash:         payload.hash,
		PreviousHash:        previousHash,
		RequestID:           audit.RequestID,
		CorrelationID:       audit.CorrelationID,
		ActorSubject:        audit.ActorSubject,
		ActorIssuer:         audit.ActorIssuer,
		ClientID:            audit.ClientID,
		AuthorizationResult: audit.AuthorizationResult,
		PolicyID:            audit.PolicyID,
		MatchedRuleID:       audit.MatchedRuleID,
		SourceIP:            audit.SourceIP,
		UserAgent:           audit.UserAgent,
		Operation:           audit.Operation,
		Endpoint:            audit.Endpoint,
		HTTPMethod:          audit.HTTPMethod,
	}
	rowHash, err := ComputeHistoryRowHash(event)
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-ROWHASH " + err.Error())
	}
	createdAt, updatedAt := administrationTimestamps(snapshot)
	insertQuery, insertArgs, err := goqu.Insert(table).Rows(goqu.Record{
		"identifier":                     identifier,
		"change_type":                    changeType,
		"deleted":                        deleted,
		"valid_from":                     now,
		"operation_time":                 now,
		"administration_created_at_text": createdAt,
		"administration_updated_at_text": updatedAt,
		"administration_created_at":      nullableTimestamp(createdAt),
		"administration_updated_at":      nullableTimestamp(updatedAt),
		"payload_type":                   payload.payloadType,
		"content_hash":                   contentHash,
		"payload_hash":                   payload.hash,
		"previous_hash":                  previousHash,
		"row_hash":                       rowHash,
		"actor_subject":                  audit.ActorSubject,
		"actor_issuer":                   audit.ActorIssuer,
		"client_id":                      audit.ClientID,
		"authorization_result":           audit.AuthorizationResult,
		"policy_id":                      audit.PolicyID,
		"matched_rule_id":                audit.MatchedRuleID,
		"request_id":                     audit.RequestID,
		"correlation_id":                 audit.CorrelationID,
		"source_ip":                      nullableINET(audit.SourceIP),
		"user_agent":                     audit.UserAgent,
		"operation":                      audit.Operation,
		"endpoint":                       audit.Endpoint,
		"http_method":                    audit.HTTPMethod,
	}).Returning(goqu.C("history_id")).ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-BUILDINSERT " + err.Error())
	}
	var historyID int64
	if err = tx.QueryRowContext(ctx, insertQuery, insertArgs...).Scan(&historyID); err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-EXECINSERT " + err.Error())
	}
	payloadRecord := goqu.Record{
		"history_id": historyID,
	}
	if payload.payloadType == PayloadTypeSnapshot {
		payloadRecord["snapshot"] = goqu.L("?::jsonb", string(payload.json))
	} else {
		payloadRecord["diff"] = goqu.L("?::jsonb", string(payload.json))
	}
	payloadQuery, payloadArgs, err := goqu.Insert(payloadTable).Rows(payloadRecord).ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-BUILDPAYLOADINSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, payloadQuery, payloadArgs...); err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-EXECPAYLOADINSERT " + err.Error())
	}
	return nil
}

func lockIdentifierTx(ctx context.Context, tx *sql.Tx, table string, identifier string) error {
	query, args, err := buildLockIdentifierQuery(table, identifier)
	if err != nil {
		return common.NewInternalServerError("HISTORY-LOCK-BUILDSQL " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("HISTORY-LOCK-EXECSQL " + err.Error())
	}
	return nil
}

func buildLockIdentifierQuery(table string, identifier string) (string, []any, error) {
	lockKey := table + ":" + identifier
	return goqu.
		Dialect("postgres").
		Select(goqu.Func("pg_advisory_xact_lock", goqu.Func("hashtextextended", lockKey, 0))).
		Prepared(true).
		ToSQL()
}

func latestRowHashTx(ctx context.Context, tx *sql.Tx, table string, identifier string) (string, error) {
	query, args, err := goqu.From(table).
		Select(goqu.C("row_hash")).
		Where(goqu.C("identifier").Eq(identifier)).
		Order(goqu.C("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return "", common.NewInternalServerError("HISTORY-HASH-BUILDPREVIOUS " + err.Error())
	}
	var previousHash sql.NullString
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&previousHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", common.NewInternalServerError("HISTORY-HASH-READPREVIOUS " + err.Error())
	}
	return previousHash.String, nil
}

func buildHistoryPayload(snapshot map[string]any, latest *latestVersion, cfg Config) (historyPayload, error) {
	if latest == nil || cfg.FullSnapshotInterval == DefaultFullSnapshotInterval || latest.rowsSinceSnapshot >= cfg.FullSnapshotInterval {
		return buildSnapshotPayload(snapshot)
	}
	patch, diffJSON, err := buildDiffPayloadJSON(latest.snapshot, snapshot)
	if err != nil {
		return historyPayload{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-APPEND-MARSHALSNAPSHOT " + err.Error())
	}
	if len(diffJSON) >= len(snapshotJSON) {
		return buildSnapshotPayloadWithJSON(snapshot, snapshotJSON)
	}
	return buildDiffPayloadWithJSON(patch, diffJSON)
}

func buildDiffPayloadJSON(base map[string]any, snapshot map[string]any) ([]map[string]any, []byte, error) {
	patch, err := BuildJSONPatch(base, snapshot)
	if err != nil {
		return nil, nil, common.NewInternalServerError("HISTORY-APPEND-BUILDDIFF " + err.Error())
	}
	payloadJSON, err := json.Marshal(patch)
	if err != nil {
		return nil, nil, common.NewInternalServerError("HISTORY-APPEND-MARSHALDIFF " + err.Error())
	}
	return patch, payloadJSON, nil
}

func buildDiffPayloadWithJSON(patch []map[string]any, payloadJSON []byte) (historyPayload, error) {
	payloadHash, err := CanonicalJSONHash(patch)
	if err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-APPEND-DIFFHASH " + err.Error())
	}
	return historyPayload{payloadType: PayloadTypeDiff, json: payloadJSON, hash: payloadHash}, nil
}

func buildSnapshotPayload(snapshot map[string]any) (historyPayload, error) {
	payloadJSON, err := json.Marshal(snapshot)
	if err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-APPEND-MARSHALSNAPSHOT " + err.Error())
	}
	return buildSnapshotPayloadWithJSON(snapshot, payloadJSON)
}

func buildSnapshotPayloadWithJSON(snapshot map[string]any, payloadJSON []byte) (historyPayload, error) {
	payloadHash, err := CanonicalJSONHash(snapshot)
	if err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-APPEND-SNAPSHOTHASH " + err.Error())
	}
	return historyPayload{payloadType: PayloadTypeSnapshot, json: payloadJSON, hash: payloadHash}, nil
}

func cloneSnapshotMap(snapshot map[string]any) (map[string]any, error) {
	cloned, err := cloneJSONValue(snapshot)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-MUTATE-CLONESNAPSHOT " + err.Error())
	}
	result, ok := cloned.(map[string]any)
	if !ok {
		return nil, common.NewInternalServerError("HISTORY-MUTATE-CLONESNAPSHOTTYPE cloned snapshot must be an object")
	}
	return result, nil
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

func nullableINET(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return goqu.L("?::inet", value)
}

func nullableTimestamp(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := common.ParseISO8601DateTime(value)
	if err != nil {
		return nil
	}
	return parsed.UTC()
}
