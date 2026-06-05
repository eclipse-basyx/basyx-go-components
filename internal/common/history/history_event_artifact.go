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
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const historyEventArtifactVersion = "basyx-history-event-v1"

// EventArtifactCandidate is one history row prepared for WORM event storage.
type EventArtifactCandidate struct {
	Artifact     EvidenceArtifact
	HistoryTable string
	Identifier   string
	HistoryID    int64
	RowHash      string
	ContentHash  string
}

// LoadEventArtifactCandidates loads snapshot and diff rows as WORM history-event artifacts.
//
// The returned artifacts use the same canonical representation as the
// synchronous append path. Callers can use this to backfill existing PostgreSQL
// history rows into WORM evidence storage before publishing a range manifest.
//
// Parameters:
//   - ctx: Request context for reading history rows.
//   - db: Database handle connected to the BaSyx PostgreSQL database.
//   - table: History table to read, for example aas_history.
//   - identifier: Optional entity identifier scope. Empty means all identifiers.
//   - firstHistoryID: Inclusive lower history_id bound.
//   - lastHistoryID: Inclusive upper history_id bound.
//
// Returns:
//   - []EventArtifactCandidate: Canonical per-row artifacts in history_id order.
//   - error: Error when the table is unsupported, hashes do not validate, or
//     rows cannot be read.
func LoadEventArtifactCandidates(ctx context.Context, db *sql.DB, table string, identifier string, firstHistoryID int64, lastHistoryID int64) ([]EventArtifactCandidate, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("HISTORY-EVIDENCE-EVENTARTIFACT-NILDB database handle must not be nil")
	}
	payloadTable, err := historyPayloadTable(table)
	if err != nil {
		return nil, err
	}
	query, args, err := historyEventArtifactCandidateQuery(table, payloadTable, identifier, firstHistoryID, lastHistoryID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-EVENTARTIFACT-EXECQUERY " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()
	return scanEventArtifactCandidates(rows, table)
}

func buildHistoryEventEvidenceArtifact(table string, historyID int64, event ChangeEvent, payload historyPayload, createdAt string, updatedAt string) (EvidenceArtifact, error) {
	payloadValue, err := decodeHistoryEventPayload(payload)
	if err != nil {
		return EvidenceArtifact{}, err
	}
	rowHash := strings.TrimSpace(event.RowHash)
	if rowHash == "" {
		return EvidenceArtifact{}, common.NewInternalServerError("HISTORY-EVIDENCE-EVENTARTIFACT-ROWHASH row hash is required")
	}
	data, err := CanonicalJSON(map[string]any{
		"artifact_version": historyEventArtifactVersion,
		"hash_contract":    historyRowHashContract,
		"history_table":    table,
		"history_id":       historyID,
		"identifier":       event.Identifier,
		"change_type":      event.ChangeType,
		"deleted":          event.Deleted,
		"valid_from":       event.Timestamp.UTC().Format(time.RFC3339Nano),
		"operation_time":   event.Timestamp.UTC().Format(time.RFC3339Nano),
		"administration": map[string]any{
			"created_at_text": createdAt,
			"updated_at_text": updatedAt,
		},
		"payload_type":  payload.payloadType,
		"payload":       payloadValue,
		"content_hash":  event.ContentHash,
		"payload_hash":  event.PayloadHash,
		"previous_hash": event.PreviousHash,
		"row_hash":      rowHash,
		"audit": map[string]any{
			"request_id":           event.RequestID,
			"correlation_id":       event.CorrelationID,
			"actor_subject":        event.ActorSubject,
			"actor_issuer":         event.ActorIssuer,
			"client_id":            event.ClientID,
			"authorization_result": event.AuthorizationResult,
			"policy_id":            event.PolicyID,
			"matched_rule_id":      event.MatchedRuleID,
			"source_ip":            event.SourceIP,
			"user_agent":           event.UserAgent,
			"operation":            event.Operation,
			"endpoint":             event.Endpoint,
			"http_method":          event.HTTPMethod,
		},
	})
	if err != nil {
		return EvidenceArtifact{}, common.NewInternalServerError("HISTORY-EVIDENCE-EVENTARTIFACT-CANONICAL " + err.Error())
	}
	return EvidenceArtifact{
		ArtifactType: EvidenceArtifactHistoryEvent,
		ObjectKey:    historyEventObjectKey(table, event.Identifier, historyID, rowHash),
		ContentType:  manifestJSONContentType,
		Data:         data,
		Metadata:     historyEventMetadata(table, event.Identifier, historyID, rowHash),
	}, nil
}

func publishHistoryEventEvidenceTx(ctx context.Context, tx *sql.Tx, cfg Config, table string, historyID int64, event ChangeEvent, payload historyPayload, createdAt string, updatedAt string) error {
	if !cfg.EvidenceEnabled {
		return nil
	}
	if cfg.EvidenceStore == nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-APPEND-NILSTORE evidence store is not initialized")
	}
	artifact, err := buildHistoryEventEvidenceArtifact(table, historyID, event, payload, createdAt, updatedAt)
	if err != nil {
		return err
	}
	writeCtx := ctx
	cancel := func() {}
	if cfg.EvidenceWriteTimeout > 0 {
		writeCtx, cancel = context.WithTimeout(ctx, cfg.EvidenceWriteTimeout)
	}
	defer cancel()
	receipt, err := cfg.EvidenceStore.PutArtifact(writeCtx, artifact)
	if err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-APPEND-PUTARTIFACT " + err.Error())
	}
	if receipt == nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-APPEND-NILRECEIPT evidence store returned nil receipt")
	}
	return RecordHistoryEventEvidenceArtifactTx(ctx, tx, EventEvidenceRecord{
		HistoryTable: table,
		Identifier:   event.Identifier,
		HistoryID:    historyID,
		RowHash:      event.RowHash,
		ContentHash:  event.ContentHash,
		Receipt:      *receipt,
	})
}

func decodeHistoryEventPayload(payload historyPayload) (any, error) {
	var payloadValue any
	if err := decodeJSONPreservingNumbers(payload.json, &payloadValue); err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-EVENTARTIFACT-DECODE " + err.Error())
	}
	return payloadValue, nil
}

func historyEventObjectKey(table string, identifier string, historyID int64, rowHash string) string {
	return path.Join(
		"history-events",
		url.PathEscape(table),
		url.PathEscape(identifier),
		fmt.Sprintf("%d-%s.json", historyID, strings.TrimSpace(rowHash)),
	)
}

func historyEventMetadata(table string, identifier string, historyID int64, rowHash string) map[string]string {
	return map[string]string{
		"artifact_type": EvidenceArtifactHistoryEvent,
		"history_table": table,
		"identifier":    identifier,
		"history_id":    fmt.Sprintf("%d", historyID),
		"row_hash":      strings.TrimSpace(rowHash),
	}
}

func historyEventArtifactCandidateQuery(table string, payloadTable string, identifier string, firstHistoryID int64, lastHistoryID int64) (string, []any, error) {
	historyAlias := goqu.T(table).As("history")
	payloadAlias := goqu.T(payloadTable).As("payload")
	dataset := baseVersionChainQuery(historyAlias, payloadAlias).
		Where(
			historyAlias.Col("history_id").Gte(firstHistoryID),
			historyAlias.Col("history_id").Lte(lastHistoryID),
		)
	if strings.TrimSpace(identifier) != "" {
		dataset = dataset.Where(historyAlias.Col("identifier").Eq(strings.TrimSpace(identifier)))
	}
	return dataset.Order(historyAlias.Col("history_id").Asc()).ToSQL()
}

func scanEventArtifactCandidates(rows *sql.Rows, table string) ([]EventArtifactCandidate, error) {
	storedRows, err := scanStoredHistoryRows(rows, table)
	if err != nil {
		return nil, err
	}
	candidates := make([]EventArtifactCandidate, 0, len(storedRows))
	for _, row := range storedRows {
		candidate, err := historyEventArtifactCandidateFromStoredRow(row)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func historyEventArtifactCandidateFromStoredRow(row storedHistoryRow) (EventArtifactCandidate, error) {
	payload, err := historyPayloadFromStoredRow(row)
	if err != nil {
		return EventArtifactCandidate{}, err
	}
	artifact, err := buildHistoryEventEvidenceArtifact(
		row.EntityType,
		row.HistoryID,
		historyRowHashEvent(row),
		payload,
		nullStringValue(row.CreatedAt),
		nullStringValue(row.UpdatedAt),
	)
	if err != nil {
		return EventArtifactCandidate{}, err
	}
	return EventArtifactCandidate{
		Artifact:     artifact,
		HistoryTable: row.EntityType,
		Identifier:   row.Identifier,
		HistoryID:    row.HistoryID,
		RowHash:      nullStringValue(row.RowHash),
		ContentHash:  nullStringValue(row.ContentHash),
	}, nil
}

func historyPayloadFromStoredRow(row storedHistoryRow) (historyPayload, error) {
	payloadJSON, err := storedPayloadJSON(row)
	if err != nil {
		return historyPayload{}, err
	}
	var payloadValue any
	if err = decodeJSONPreservingNumbers(payloadJSON, &payloadValue); err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-EVIDENCE-EVENTARTIFACT-DECODESTORED " + err.Error())
	}
	if err = verifyCanonicalHash(payloadValue, row.PayloadHash, "HISTORY-EVIDENCE-EVENTARTIFACT-PAYLOADHASH"); err != nil {
		return historyPayload{}, err
	}
	return historyPayload{
		payloadType: row.PayloadType,
		json:        payloadJSON,
		hash:        nullStringValue(row.PayloadHash),
	}, nil
}

func storedPayloadJSON(row storedHistoryRow) ([]byte, error) {
	switch row.PayloadType {
	case PayloadTypeSnapshot:
		if !row.Snapshot.Valid {
			return nil, common.NewInternalServerError("HISTORY-EVIDENCE-EVENTARTIFACT-MISSINGSNAPSHOT snapshot payload is missing")
		}
		return []byte(row.Snapshot.String), nil
	case PayloadTypeDiff:
		if !row.Diff.Valid {
			return nil, common.NewInternalServerError("HISTORY-EVIDENCE-EVENTARTIFACT-MISSINGDIFF diff payload is missing")
		}
		return []byte(row.Diff.String), nil
	default:
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-EVENTARTIFACT-PAYLOADTYPE unsupported payload type '" + row.PayloadType + "'")
	}
}
