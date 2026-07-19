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
	"errors"
	"fmt"
	"log"
	"net/url"
	"path"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	mutationEventArtifactVersion = "basyx-mutation-event-v2"
	mutationEventHashContract    = "basyx-mutation-event-v1"
	// TableMutationEvidenceState stores the committed head of each independent evidence chain.
	TableMutationEvidenceState = "mutation_evidence_state"
	// TableMutationEvidenceEvents catalogs committed mutation artifacts.
	TableMutationEvidenceEvents = "mutation_evidence_artifacts"
	// EvidenceArtifactMutation identifies canonical independent mutation artifacts.
	EvidenceArtifactMutation = "mutation_event"
)

type mutationEvidenceState struct {
	lastSequence        int64
	lastEventHash       string
	lastContentHash     string
	eventsSinceSnapshot int
}

// MutationEvidenceResult identifies a committed mutation artifact inside the
// caller's still-open transaction. Binary reference evidence can use this to
// bind its receipt to the exact model mutation.
type MutationEvidenceResult struct {
	ArtifactID    int64
	EventSequence int64
	EventHash     string
	Receipt       EvidenceReceipt
}

type mutationEvidenceWrite struct {
	table               string
	identifier          string
	changeType          string
	snapshot            map[string]any
	deleted             bool
	payload             historyPayload
	effectiveDiff       []map[string]any
	previousHash        string
	sequence            int64
	now                 time.Time
	historyID           int64
	historyRowHash      string
	eventsSinceSnapshot int
}

func loadMutationEvidenceStateTx(ctx context.Context, tx *sql.Tx, table string, identifier string) (*mutationEvidenceState, error) {
	query, args, err := goqu.From(TableMutationEvidenceState).
		Select("last_sequence", "last_event_hash", "last_content_hash", "events_since_snapshot").
		Where(goqu.Ex{"entity_type": table, "identifier": identifier}).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-STATE-BUILD " + err.Error())
	}
	var state mutationEvidenceState
	var previousHash, contentHash sql.NullString
	if err = tx.QueryRowContext(ctx, query, args...).Scan(
		&state.lastSequence, &previousHash, &contentHash, &state.eventsSinceSnapshot,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-STATE-QUERY " + err.Error())
	}
	state.lastEventHash = previousHash.String
	state.lastContentHash = contentHash.String
	if state.lastEventHash == "" || state.lastContentHash == "" {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-STATE-HEAD committed evidence state has no chain head")
	}
	return &state, nil
}

func mutationSnapshotPayload(value any) (map[string]any, error) {
	snapshot, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("snapshot payload must be an object")
	}
	return snapshot, nil
}

func mutationDiffPayload(value any) ([]map[string]any, error) {
	items, ok := value.([]any)
	if !ok {
		if typed, typedOK := value.([]map[string]any); typedOK {
			return typed, nil
		}
		return nil, fmt.Errorf("diff payload must be an array")
	}
	patch := make([]map[string]any, 0, len(items))
	for _, item := range items {
		operation, operationOK := item.(map[string]any)
		if !operationOK {
			return nil, fmt.Errorf("diff operation must be an object")
		}
		patch = append(patch, operation)
	}
	return patch, nil
}

func publishMutationEvidenceTx(ctx context.Context, tx *sql.Tx, cfg Config, write mutationEvidenceWrite) (*MutationEvidenceResult, error) {
	if !cfg.EvidenceEnabled {
		return nil, nil
	}
	if cfg.EvidenceStore == nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-NILSTORE evidence store is not initialized")
	}
	payloadValue, err := decodeHistoryEventPayload(write.payload)
	if err != nil {
		return nil, err
	}
	effectiveDiff, err := canonicalEffectiveDiffValue(write.effectiveDiff)
	if err != nil {
		return nil, err
	}
	contentHash, err := CanonicalJSONHash(write.snapshot)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-CONTENTHASH " + err.Error())
	}
	audit := FromContext(ctx)
	createdAt, updatedAt := administrationTimestamps(write.snapshot)
	body := map[string]any{
		"artifact_version":           mutationEventArtifactVersion,
		"hash_contract":              mutationEventHashContract,
		"entity_type":                write.table,
		"identifier":                 write.identifier,
		"event_sequence":             write.sequence,
		"change_type":                write.changeType,
		"deleted":                    write.deleted,
		"operation_time":             write.now.UTC().Format(time.RFC3339Nano),
		"administration":             map[string]any{"created_at_text": createdAt, "updated_at_text": updatedAt},
		"payload_type":               write.payload.payloadType,
		"payload":                    payloadValue,
		"effective_diff":             effectiveDiff,
		"content_hash":               contentHash,
		"payload_hash":               write.payload.hash,
		"previous_event_hash":        write.previousHash,
		"binary_references_expected": binaryReferencesExpected(ctx),
		"audit": map[string]any{
			"request_id": audit.RequestID, "correlation_id": audit.CorrelationID,
			"actor_subject": audit.ActorSubject, "actor_issuer": audit.ActorIssuer,
			"client_id": audit.ClientID, "authorization_result": audit.AuthorizationResult,
			"policy_id": audit.PolicyID, "matched_rule_id": audit.MatchedRuleID,
			"source_ip": audit.SourceIP, "user_agent": audit.UserAgent,
			"operation": audit.Operation, "endpoint": audit.Endpoint, "http_method": audit.HTTPMethod,
		},
	}
	eventHash, err := CanonicalJSONHash(body)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-EVENTHASH " + err.Error())
	}
	body["event_hash"] = eventHash
	data, err := CanonicalJSON(body)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-CANONICAL " + err.Error())
	}
	artifact := EvidenceArtifact{
		ArtifactType: EvidenceArtifactMutation,
		ObjectKey:    path.Join("mutation-events", url.PathEscape(write.table), url.PathEscape(write.identifier), fmt.Sprintf("%d-%s.json", write.sequence, eventHash)),
		ContentType:  manifestJSONContentType,
		Data:         data,
		Metadata: map[string]string{
			"artifact_type": EvidenceArtifactMutation, "entity_type": write.table,
			"identifier": write.identifier, "event_sequence": fmt.Sprintf("%d", write.sequence), "event_hash": eventHash,
		},
	}
	writeCtx := ctx
	cancel := func() {}
	if cfg.EvidenceWriteTimeout > 0 {
		writeCtx, cancel = context.WithTimeout(ctx, cfg.EvidenceWriteTimeout)
	}
	defer cancel()
	receipt, err := cfg.EvidenceStore.PutArtifact(writeCtx, artifact)
	if err != nil {
		log.Printf("HISTORY-EVIDENCE-MUTATION-PUT evidence store write failed: %v", err)
		return nil, common.NewErrServiceUnavailable("HISTORY-EVIDENCE-MUTATION-STORE mutation evidence could not be stored")
	}
	if receipt == nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-NILRECEIPT evidence store returned nil receipt")
	}
	if validationErr := validateCommittedEvidenceReceipt(*receipt, SHA256Hex(data), int64(len(data)), time.Now()); validationErr != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-RECEIPT " + validationErr.Error())
	}
	artifactID, err := recordMutationEvidenceTx(ctx, tx, write, eventHash, contentHash, *receipt)
	if err != nil {
		return nil, err
	}
	return &MutationEvidenceResult{ArtifactID: artifactID, EventSequence: write.sequence, EventHash: eventHash, Receipt: *receipt}, nil
}

func recordMutationEvidenceTx(ctx context.Context, tx *sql.Tx, write mutationEvidenceWrite, eventHash string, contentHash string, receipt EvidenceReceipt) (int64, error) {
	record := goqu.Record{
		"entity_type": write.table, "identifier": write.identifier, "event_sequence": write.sequence,
		"event_hash": eventHash, "previous_event_hash": nullableText(write.previousHash),
		"content_hash": contentHash, "payload_hash": write.payload.hash, "payload_type": write.payload.payloadType,
		"history_table": nullableText(write.table), "history_id": nil, "history_row_hash": nil,
		"provider": receipt.Reference.Provider, "bucket": nullableText(receipt.Reference.Bucket),
		"object_key": receipt.Reference.ObjectKey, "object_version_id": nullableText(receipt.Reference.VersionID),
		"sha256": receipt.SHA256, "size_bytes": receipt.SizeBytes, "content_type": receipt.ContentType,
		"retention_mode": nullableText(receipt.RetentionMode), "retain_until": nullableTime(receipt.RetainUntil),
		"legal_hold": receipt.LegalHold, "artifact_metadata": jsonbMetadata(receipt.Metadata),
	}
	if write.historyID > 0 {
		record["history_id"] = write.historyID
		record["history_row_hash"] = nullableText(write.historyRowHash)
	}
	query, args, err := goqu.Insert(TableMutationEvidenceEvents).Rows(record).Returning("artifact_id").ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-CATALOGBUILD " + err.Error())
	}
	var artifactID int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&artifactID); err != nil {
		return 0, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-CATALOGINSERT " + err.Error())
	}
	stateRecord := goqu.Record{
		"entity_type": write.table, "identifier": write.identifier, "last_sequence": write.sequence,
		"last_event_hash": eventHash, "last_content_hash": contentHash,
		"events_since_snapshot": write.eventsSinceSnapshot, "db_updated_at": time.Now().UTC(),
	}
	stateSQL, stateArgs, err := goqu.Insert(TableMutationEvidenceState).Rows(stateRecord).
		OnConflict(goqu.DoUpdate("entity_type,identifier", stateRecord)).ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-STATEUPSERTBUILD " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, stateSQL, stateArgs...); err != nil {
		return 0, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-STATEUPSERT " + err.Error())
	}
	return artifactID, nil
}
