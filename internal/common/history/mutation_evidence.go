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
	"net/url"
	"path"
	"strings"
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
	eventsSinceSnapshot int
	snapshot            map[string]any
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
	table          string
	identifier     string
	changeType     string
	snapshot       map[string]any
	deleted        bool
	payload        historyPayload
	effectiveDiff  []map[string]any
	previousHash   string
	sequence       int64
	now            time.Time
	historyID      int64
	historyRowHash string
}

func loadMutationEvidenceStateTx(ctx context.Context, tx *sql.Tx, cfg Config, table string, identifier string) (*mutationEvidenceState, error) {
	query, args, err := goqu.From(TableMutationEvidenceState).
		Select("last_sequence", "last_event_hash", "events_since_snapshot").
		Where(goqu.Ex{"entity_type": table, "identifier": identifier}).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-STATE-BUILD " + err.Error())
	}
	var state mutationEvidenceState
	var previousHash sql.NullString
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&state.lastSequence, &previousHash, &state.eventsSinceSnapshot); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-STATE-QUERY " + err.Error())
	}
	state.lastEventHash = previousHash.String
	state.snapshot, err = restoreMutationEvidenceSnapshotTx(ctx, tx, cfg, table, identifier, state)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

type mutationEvidenceCatalogArtifact struct {
	sequence     int64
	payloadType  string
	contentHash  string
	eventHash    string
	previousHash string
	sha256       string
	reference    EvidenceReference
}

func restoreMutationEvidenceSnapshotTx(ctx context.Context, tx *sql.Tx, cfg Config, table string, identifier string, state mutationEvidenceState) (map[string]any, error) {
	limit := state.eventsSinceSnapshot + 1
	if limit <= 0 {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-LIMIT invalid evidence checkpoint distance")
	}
	query, args, err := goqu.From(TableMutationEvidenceEvents).
		Select("event_sequence", "payload_type", "content_hash", "event_hash", "previous_event_hash", "sha256", "provider", "bucket", "object_key", "object_version_id").
		Where(goqu.Ex{"entity_type": table, "identifier": identifier}).
		Order(goqu.C("event_sequence").Desc()).
		// #nosec G115 -- limit is a validated positive int, so conversion to the same-width unsigned type is safe.
		Limit(uint(limit)).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-BUILD " + err.Error())
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-QUERY " + err.Error())
	}
	defer func() { _ = rows.Close() }()
	artifacts := make([]mutationEvidenceCatalogArtifact, 0, limit)
	for rows.Next() {
		var artifact mutationEvidenceCatalogArtifact
		var bucket, version, previousHash sql.NullString
		if err = rows.Scan(&artifact.sequence, &artifact.payloadType, &artifact.contentHash, &artifact.eventHash, &previousHash, &artifact.sha256, &artifact.reference.Provider, &bucket, &artifact.reference.ObjectKey, &version); err != nil {
			return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-SCAN " + err.Error())
		}
		artifact.reference.Bucket = bucket.String
		artifact.reference.VersionID = version.String
		artifact.previousHash = previousHash.String
		artifacts = append(artifacts, artifact)
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-ROWS " + err.Error())
	}
	if len(artifacts) != limit || artifacts[len(artifacts)-1].payloadType != PayloadTypeSnapshot {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-CHECKPOINT evidence checkpoint chain is incomplete")
	}
	if cfg.EvidenceStore == nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-NILSTORE evidence store is not initialized")
	}
	var snapshot map[string]any
	expectedPreviousHash := ""
	for index := len(artifacts) - 1; index >= 0; index-- {
		artifact := artifacts[index]
		object, getErr := cfg.EvidenceStore.GetArtifact(ctx, artifact.reference)
		if getErr != nil {
			return nil, common.NewErrServiceUnavailable("HISTORY-EVIDENCE-RESTORE-GET " + getErr.Error())
		}
		if !strings.EqualFold(SHA256Hex(object.Data), artifact.sha256) {
			return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-ARTIFACTHASH mutation artifact hash does not match its catalog receipt")
		}
		document, decodeErr := decodeMutationEvidenceDocument(object.Data)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if document.EventSequence != artifact.sequence || !strings.EqualFold(document.EventHash, artifact.eventHash) || document.PreviousEventHash != artifact.previousHash || artifact.previousHash != expectedPreviousHash {
			return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-CHAIN mutation evidence chain does not match its catalog")
		}
		switch artifact.payloadType {
		case PayloadTypeSnapshot:
			snapshot, decodeErr = mutationSnapshotPayload(document.Payload)
		case PayloadTypeDiff:
			var patch []map[string]any
			patch, decodeErr = mutationDiffPayload(document.Payload)
			if decodeErr == nil {
				snapshot, decodeErr = ApplyJSONPatch(snapshot, patch)
			}
		default:
			decodeErr = fmt.Errorf("unsupported payload type %q", artifact.payloadType)
		}
		if decodeErr != nil {
			return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-PAYLOAD " + decodeErr.Error())
		}
		actualHash, hashErr := CanonicalJSONHash(snapshot)
		if hashErr != nil || !strings.EqualFold(actualHash, artifact.contentHash) {
			return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-HASH restored evidence content hash does not match")
		}
		expectedPreviousHash = artifact.eventHash
	}
	if !strings.EqualFold(expectedPreviousHash, state.lastEventHash) {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-HEAD restored evidence chain does not match its state head")
	}
	return snapshot, nil
}

type mutationEvidenceDocument struct {
	ArtifactVersion   string `json:"artifact_version"`
	EventSequence     int64  `json:"event_sequence"`
	EventHash         string `json:"event_hash"`
	PreviousEventHash string `json:"previous_event_hash"`
	PayloadType       string `json:"payload_type"`
	Payload           any    `json:"payload"`
}

func decodeMutationEvidenceDocument(data []byte) (mutationEvidenceDocument, error) {
	var document mutationEvidenceDocument
	if err := decodeJSONPreservingNumbers(data, &document); err != nil {
		return mutationEvidenceDocument{}, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-DECODE " + err.Error())
	}
	if document.ArtifactVersion != mutationEventArtifactVersion {
		return mutationEvidenceDocument{}, common.NewInternalServerError("HISTORY-EVIDENCE-RESTORE-VERSION unsupported mutation artifact version")
	}
	return document, nil
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
		return nil, common.NewErrServiceUnavailable("HISTORY-EVIDENCE-MUTATION-PUT " + err.Error())
	}
	if receipt == nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-NILRECEIPT evidence store returned nil receipt")
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
	eventsSinceSnapshot := 0
	if write.payload.payloadType == PayloadTypeDiff {
		eventsSinceSnapshot = int(write.sequence)
		stateQuery, stateArgs, stateErr := goqu.From(TableMutationEvidenceState).
			Select("events_since_snapshot").Where(goqu.Ex{"entity_type": write.table, "identifier": write.identifier}).ToSQL()
		if stateErr != nil {
			return 0, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-STATEBUILD " + stateErr.Error())
		}
		var previousCount int
		if stateErr = tx.QueryRowContext(ctx, stateQuery, stateArgs...).Scan(&previousCount); stateErr == nil {
			eventsSinceSnapshot = previousCount + 1
		} else if !errors.Is(stateErr, sql.ErrNoRows) {
			return 0, common.NewInternalServerError("HISTORY-EVIDENCE-MUTATION-STATEQUERY " + stateErr.Error())
		}
	}
	stateRecord := goqu.Record{
		"entity_type": write.table, "identifier": write.identifier, "last_sequence": write.sequence,
		"last_event_hash": eventHash, "events_since_snapshot": eventsSinceSnapshot, "db_updated_at": time.Now().UTC(),
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
