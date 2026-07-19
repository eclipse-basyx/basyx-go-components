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
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// MutationEvidenceVerificationReport summarizes one independent evidence chain.
type MutationEvidenceVerificationReport struct {
	Valid         bool                  `json:"valid"`
	EntityType    string                `json:"entity_type"`
	Identifier    string                `json:"identifier"`
	FirstSequence int64                 `json:"first_sequence"`
	LastSequence  int64                 `json:"last_sequence"`
	EventCount    int64                 `json:"event_count"`
	ContentHash   string                `json:"content_hash,omitempty"`
	EventHash     string                `json:"event_hash,omitempty"`
	ChangeType    string                `json:"change_type,omitempty"`
	Deleted       bool                  `json:"deleted"`
	OperationTime string                `json:"operation_time,omitempty"`
	Audit         map[string]any        `json:"audit,omitempty"`
	Snapshot      map[string]any        `json:"snapshot,omitempty"`
	Findings      []VerificationFinding `json:"findings,omitempty"`
}

type mutationVerificationRow struct {
	artifactID   int64
	sequence     int64
	eventHash    string
	previousHash string
	contentHash  string
	payloadHash  string
	payloadType  string
	receipt      EvidenceReceipt
}

type mutationVerificationDocument struct {
	ArtifactVersion          string                       `json:"artifact_version"`
	HashContract             string                       `json:"hash_contract"`
	EntityType               string                       `json:"entity_type"`
	Identifier               string                       `json:"identifier"`
	EventSequence            int64                        `json:"event_sequence"`
	EventHash                string                       `json:"event_hash"`
	PreviousEventHash        string                       `json:"previous_event_hash"`
	PayloadType              string                       `json:"payload_type"`
	Payload                  any                          `json:"payload"`
	EffectiveDiff            []map[string]any             `json:"effective_diff"`
	ExpectedBinaryReferences []BinaryReferenceExpectation `json:"binary_references_expected"`
	ContentHash              string                       `json:"content_hash"`
	PayloadHash              string                       `json:"payload_hash"`
	ChangeType               string                       `json:"change_type"`
	Deleted                  bool                         `json:"deleted"`
	OperationTime            string                       `json:"operation_time"`
	Audit                    map[string]any               `json:"audit"`
}

// VerifyMutationEvidenceRange verifies and reconstructs v2 evidence without PostgreSQL history payloads.
func VerifyMutationEvidenceRange(ctx context.Context, db *sql.DB, store EvidenceStore, entityType string, identifier string, firstSequence int64, lastSequence int64) (*MutationEvidenceVerificationReport, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("HISTORY-MUTATIONVERIFY-NILDB database handle must not be nil")
	}
	entityType = strings.TrimSpace(entityType)
	identifier = strings.TrimSpace(identifier)
	if entityType == "" || identifier == "" || firstSequence < 1 || lastSequence < firstSequence {
		return nil, common.NewErrBadRequest("HISTORY-MUTATIONVERIFY-RANGE entity type, identifier, and a valid sequence range are required")
	}
	checkpoint, err := mutationCheckpointSequence(ctx, db, entityType, identifier, firstSequence)
	if err != nil {
		return nil, err
	}
	rows, err := loadMutationVerificationRows(ctx, db, entityType, identifier, checkpoint, lastSequence)
	if err != nil {
		return nil, err
	}
	report := &MutationEvidenceVerificationReport{
		Valid: true, EntityType: entityType, Identifier: identifier,
		FirstSequence: firstSequence, LastSequence: lastSequence,
	}
	if len(rows) == 0 {
		report.addFinding("HISTORY-MUTATIONVERIFY-EMPTY", "no mutation evidence exists in the requested range", 0)
		return report, nil
	}
	if rows[0].sequence != checkpoint {
		report.addFinding("HISTORY-MUTATIONVERIFY-MISSINGCHECKPOINT", "the selected snapshot checkpoint is missing from the catalog range", checkpoint)
	}
	if rows[len(rows)-1].sequence != lastSequence {
		report.addFinding("HISTORY-MUTATIONVERIFY-MISSINGTAIL", "the requested terminal mutation evidence is missing", lastSequence)
	}
	verifyMutationRows(ctx, db, store, rows, report)
	report.Valid = len(report.Findings) == 0
	return report, nil
}

func mutationCheckpointSequence(ctx context.Context, db *sql.DB, entityType string, identifier string, firstSequence int64) (int64, error) {
	query, args, err := goqu.From(TableMutationEvidenceEvents).Select("event_sequence").
		Where(goqu.Ex{"entity_type": entityType, "identifier": identifier, "payload_type": PayloadTypeSnapshot}, goqu.C("event_sequence").Lte(firstSequence)).
		Order(goqu.C("event_sequence").Desc()).Limit(1).ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-BUILDCHECKPOINT " + err.Error())
	}
	var sequence int64
	if err = db.QueryRowContext(ctx, query, args...).Scan(&sequence); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewErrNotFound("HISTORY-MUTATIONVERIFY-NOCHECKPOINT snapshot checkpoint not found")
		}
		return 0, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-CHECKPOINT " + err.Error())
	}
	return sequence, nil
}

func loadMutationVerificationRows(ctx context.Context, db *sql.DB, entityType string, identifier string, firstSequence int64, lastSequence int64) ([]mutationVerificationRow, error) {
	query, args, err := goqu.From(TableMutationEvidenceEvents).Select(
		"artifact_id", "event_sequence", "event_hash", "previous_event_hash", "content_hash", "payload_hash", "payload_type",
		"provider", "bucket", "object_key", "object_version_id", "sha256", "size_bytes", "content_type", "retention_mode", "retain_until", "legal_hold",
	).Where(
		goqu.Ex{"entity_type": entityType, "identifier": identifier},
		goqu.C("event_sequence").Gte(firstSequence), goqu.C("event_sequence").Lte(lastSequence),
	).Order(goqu.C("event_sequence").Asc()).ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-BUILDROWS " + err.Error())
	}
	sqlRows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-ROWS " + err.Error())
	}
	defer func() { _ = sqlRows.Close() }()
	rows := make([]mutationVerificationRow, 0)
	for sqlRows.Next() {
		row, scanErr := scanMutationVerificationRow(sqlRows)
		if scanErr != nil {
			return nil, scanErr
		}
		rows = append(rows, row)
	}
	if err = sqlRows.Err(); err != nil {
		return nil, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-ROWITERATION " + err.Error())
	}
	return rows, nil
}

func scanMutationVerificationRow(rows *sql.Rows) (mutationVerificationRow, error) {
	var row mutationVerificationRow
	var previous, bucket, version, retention sql.NullString
	var retainUntil sql.NullTime
	if err := rows.Scan(
		&row.artifactID, &row.sequence, &row.eventHash, &previous, &row.contentHash, &row.payloadHash, &row.payloadType,
		&row.receipt.Reference.Provider, &bucket, &row.receipt.Reference.ObjectKey, &version,
		&row.receipt.SHA256, &row.receipt.SizeBytes, &row.receipt.ContentType, &retention, &retainUntil, &row.receipt.LegalHold,
	); err != nil {
		return mutationVerificationRow{}, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-SCAN " + err.Error())
	}
	row.previousHash = previous.String
	row.receipt.Reference.Bucket = bucket.String
	row.receipt.Reference.VersionID = version.String
	row.receipt.RetentionMode = retention.String
	if retainUntil.Valid {
		row.receipt.RetainUntil = &retainUntil.Time
	}
	return row, nil
}

func verifyMutationRows(ctx context.Context, db *sql.DB, store EvidenceStore, rows []mutationVerificationRow, report *MutationEvidenceVerificationReport) {
	var snapshot map[string]any
	previousHash := rows[0].previousHash
	expectedSequence := rows[0].sequence
	for _, row := range rows {
		if row.sequence != expectedSequence || row.previousHash != previousHash {
			report.addFinding("HISTORY-MUTATIONVERIFY-CHAIN", "mutation evidence sequence or predecessor hash is discontinuous", row.sequence)
		}
		document, ok := verifyMutationObject(ctx, store, row, report)
		if ok {
			snapshot = applyVerifiedMutationPayload(snapshot, row, document, report)
			verifyMutationBinaryReferences(ctx, db, store, row, document, report)
			if row.sequence == report.LastSequence {
				report.EventHash = document.EventHash
				report.ChangeType = document.ChangeType
				report.Deleted = document.Deleted
				report.OperationTime = document.OperationTime
				report.Audit = document.Audit
			}
		}
		previousHash = row.eventHash
		expectedSequence = row.sequence + 1
		if row.sequence >= report.FirstSequence {
			report.EventCount++
		}
	}
	report.Snapshot = snapshot
	if len(rows) > 0 {
		report.ContentHash = rows[len(rows)-1].contentHash
	}
}

func verifyMutationObject(ctx context.Context, store EvidenceStore, row mutationVerificationRow, report *MutationEvidenceVerificationReport) (mutationVerificationDocument, bool) {
	if store == nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-NOSTORE", "evidence store is required to verify mutation objects", row.sequence)
		return mutationVerificationDocument{}, false
	}
	object, err := store.GetArtifact(ctx, row.receipt.Reference)
	if err != nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-OBJECT", err.Error(), row.sequence)
		return mutationVerificationDocument{}, false
	}
	if !strings.EqualFold(SHA256Hex(object.Data), row.receipt.SHA256) {
		report.addFinding("HISTORY-MUTATIONVERIFY-SHA", "mutation object SHA-256 differs from its receipt", row.sequence)
		return mutationVerificationDocument{}, false
	}
	document, err := decodeAndValidateMutationDocument(object.Data, row, report.EntityType, report.Identifier)
	if err != nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-DOCUMENT", err.Error(), row.sequence)
		return mutationVerificationDocument{}, false
	}
	verifyReceiptRetention(ctx, store, row.receipt, report, row.sequence)
	return document, true
}

func decodeAndValidateMutationDocument(data []byte, row mutationVerificationRow, entityType string, identifier string) (mutationVerificationDocument, error) {
	var document mutationVerificationDocument
	if err := decodeJSONPreservingNumbers(data, &document); err != nil {
		return document, fmt.Errorf("decode failed: %w", err)
	}
	if document.ArtifactVersion != mutationEventArtifactVersion || document.HashContract != mutationEventHashContract {
		return document, fmt.Errorf("unsupported artifact version or hash contract")
	}
	if document.EntityType != entityType || document.Identifier != identifier || document.EventSequence != row.sequence || document.PreviousEventHash != row.previousHash {
		return document, fmt.Errorf("artifact identity or chain fields differ from the catalog")
	}
	if document.EventHash != row.eventHash || document.ContentHash != row.contentHash || document.PayloadHash != row.payloadHash || document.PayloadType != row.payloadType {
		return document, fmt.Errorf("artifact hashes or payload type differ from the catalog")
	}
	if document.ChangeType != ChangeCreated && document.ChangeType != ChangeUpdated && document.ChangeType != ChangeDeleted {
		return document, fmt.Errorf("unsupported change type %q", document.ChangeType)
	}
	if (document.ChangeType == ChangeDeleted) != document.Deleted {
		return document, fmt.Errorf("change type and deletion state are inconsistent")
	}
	if _, err := time.Parse(time.RFC3339Nano, document.OperationTime); err != nil {
		return document, fmt.Errorf("operation time is invalid")
	}
	var body map[string]any
	if err := decodeJSONPreservingNumbers(data, &body); err != nil {
		return document, err
	}
	delete(body, "event_hash")
	eventHash, err := CanonicalJSONHash(body)
	if err != nil || !strings.EqualFold(eventHash, row.eventHash) {
		return document, fmt.Errorf("event hash is invalid")
	}
	payloadHash, err := CanonicalJSONHash(document.Payload)
	if err != nil || !strings.EqualFold(payloadHash, row.payloadHash) {
		return document, fmt.Errorf("payload hash is invalid")
	}
	return document, nil
}

func applyVerifiedMutationPayload(snapshot map[string]any, row mutationVerificationRow, document mutationVerificationDocument, report *MutationEvidenceVerificationReport) map[string]any {
	var err error
	switch row.payloadType {
	case PayloadTypeSnapshot:
		snapshot, err = mutationSnapshotPayload(document.Payload)
	case PayloadTypeDiff:
		var patch []map[string]any
		patch, err = mutationDiffPayload(document.Payload)
		if err == nil {
			snapshot, err = ApplyJSONPatch(snapshot, patch)
		}
	default:
		err = fmt.Errorf("unsupported payload type %q", row.payloadType)
	}
	if err != nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-PAYLOAD", err.Error(), row.sequence)
		return snapshot
	}
	contentHash, hashErr := CanonicalJSONHash(snapshot)
	if hashErr != nil || !strings.EqualFold(contentHash, row.contentHash) {
		report.addFinding("HISTORY-MUTATIONVERIFY-CONTENTHASH", "reconstructed content hash differs from the catalog", row.sequence)
	}
	return snapshot
}

func verifyReceiptRetention(ctx context.Context, store EvidenceStore, receipt EvidenceReceipt, report *MutationEvidenceVerificationReport, sequence int64) {
	if receipt.RetentionMode == "" || receipt.RetainUntil == nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-RETENTION", "evidence receipt is missing WORM retention metadata", sequence)
		return
	}
	verifier, ok := store.(EvidenceRetentionVerifier)
	if !ok {
		report.addFinding("HISTORY-MUTATIONVERIFY-RETENTIONVERIFIER", "evidence store cannot verify the current WORM retention state", sequence)
		return
	}
	if err := verifier.VerifyArtifactRetention(ctx, receipt.Reference, receipt); err != nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-RETENTIONSTATE", err.Error(), sequence)
	}
}

func (report *MutationEvidenceVerificationReport) addFinding(code string, message string, sequence int64) {
	report.Findings = append(report.Findings, VerificationFinding{
		Severity: VerificationSeverityError, Code: code, Message: message,
		Identifier: report.Identifier, HistoryID: sequence,
	})
	report.Valid = false
}

type binaryReferenceVerificationRow struct {
	modelPath string
	receipt   EvidenceReceipt
}

func verifyMutationBinaryReferences(ctx context.Context, db *sql.DB, store EvidenceStore, row mutationVerificationRow, document mutationVerificationDocument, report *MutationEvidenceVerificationReport) {
	expectedReferences := make(map[string]BinaryReferenceExpectation, len(document.ExpectedBinaryReferences))
	for _, expectation := range document.ExpectedBinaryReferences {
		if !validBinaryReferenceExpectation(expectation) {
			report.addFinding("HISTORY-MUTATIONVERIFY-BINARYEXPECTATION", "mutation contains an invalid binary-reference descriptor", row.sequence)
			continue
		}
		if _, exists := expectedReferences[expectation.ModelPath]; exists {
			report.addFinding("HISTORY-MUTATIONVERIFY-BINARYEXPECTATION", "mutation contains duplicate binary-reference descriptors", row.sequence)
		}
		expectedReferences[expectation.ModelPath] = expectation
	}
	if len(expectedReferences) == 0 {
		return
	}
	references, err := loadBinaryReferenceVerificationRows(ctx, db, row.artifactID)
	if err != nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYREFLOAD", err.Error(), row.sequence)
		return
	}
	byPath := make(map[string]binaryReferenceVerificationRow, len(references))
	for _, reference := range references {
		byPath[reference.modelPath] = reference
	}
	for expectedPath, expectation := range expectedReferences {
		reference, exists := byPath[expectedPath]
		if !exists {
			report.addFinding("HISTORY-MUTATIONVERIFY-BINARYREFMISSING", "binary-reference evidence is missing for "+expectedPath, row.sequence)
			continue
		}
		verifyBinaryReferenceObject(ctx, db, store, row, reference, expectation, report)
	}
}

func loadBinaryReferenceVerificationRows(ctx context.Context, db *sql.DB, mutationID int64) ([]binaryReferenceVerificationRow, error) {
	query, args, err := goqu.From(TableBinaryReferenceEvidence).Select(
		"model_path", "provider", "bucket", "object_key", "object_version_id", "sha256", "size_bytes", "content_type", "retention_mode", "retain_until", "legal_hold",
	).Where(goqu.C("mutation_artifact_id").Eq(mutationID)).ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-BUILDBINARYREFS " + err.Error())
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-BINARYREFS " + err.Error())
	}
	defer func() { _ = rows.Close() }()
	result := make([]binaryReferenceVerificationRow, 0)
	for rows.Next() {
		var row binaryReferenceVerificationRow
		var bucket, version, retention sql.NullString
		var retainUntil sql.NullTime
		if err = rows.Scan(
			&row.modelPath, &row.receipt.Reference.Provider, &bucket, &row.receipt.Reference.ObjectKey, &version,
			&row.receipt.SHA256, &row.receipt.SizeBytes, &row.receipt.ContentType, &retention, &retainUntil, &row.receipt.LegalHold,
		); err != nil {
			return nil, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-SCANBINARYREF " + err.Error())
		}
		row.receipt.Reference.Bucket = bucket.String
		row.receipt.Reference.VersionID = version.String
		row.receipt.RetentionMode = retention.String
		if retainUntil.Valid {
			row.receipt.RetainUntil = &retainUntil.Time
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func verifyBinaryReferenceObject(ctx context.Context, db *sql.DB, store EvidenceStore, mutation mutationVerificationRow, reference binaryReferenceVerificationRow, expected BinaryReferenceExpectation, report *MutationEvidenceVerificationReport) {
	if store == nil {
		return
	}
	object, err := store.GetArtifact(ctx, reference.receipt.Reference)
	if err != nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYREFOBJECT", err.Error(), mutation.sequence)
		return
	}
	if !strings.EqualFold(SHA256Hex(object.Data), reference.receipt.SHA256) {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYREFSHA", "binary-reference object SHA-256 differs from its receipt", mutation.sequence)
		return
	}
	var document binaryReferenceVerificationDocument
	if err = decodeJSONPreservingNumbers(object.Data, &document); err != nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYREFDECODE", err.Error(), mutation.sequence)
		return
	}
	if document.ArtifactVersion != binaryReferenceVersion || document.EventHash != mutation.eventHash || document.ModelPath != reference.modelPath {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYREFMISMATCH", "binary-reference object does not match its mutation", mutation.sequence)
	}
	actual := document.expectation()
	if actual != expected {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYREFBINDING", "binary-reference object differs from the descriptor committed by the mutation", mutation.sequence)
	}
	verifyReferencedBinaryObject(ctx, db, store, actual, report, mutation.sequence)
	verifyReceiptRetention(ctx, store, reference.receipt, report, mutation.sequence)
}

func verifyReferencedBinaryObject(ctx context.Context, db *sql.DB, store EvidenceStore, expectation BinaryReferenceExpectation, report *MutationEvidenceVerificationReport, sequence int64) {
	if !validSHA256(expectation.SHA256) || expectation.SizeBytes < 0 {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYDOCUMENT", "binary-reference digest or size is invalid", sequence)
		return
	}
	receipt, err := loadBinaryEvidenceReceiptByDigest(ctx, db, expectation.SHA256, expectation.SizeBytes)
	if err != nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYRECEIPT", err.Error(), sequence)
		return
	}
	if expectation.BinaryReference != receipt.Reference {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYRECEIPTMISMATCH", "binary-reference object does not match the immutable binary receipt", sequence)
	}
	if _, err = store.VerifyArtifact(ctx, receipt.Reference, expectation.SHA256); err != nil {
		report.addFinding("HISTORY-MUTATIONVERIFY-BINARYOBJECT", err.Error(), sequence)
	}
	verifyReceiptRetention(ctx, store, receipt, report, sequence)
}

type binaryReferenceVerificationDocument struct {
	ArtifactVersion string            `json:"artifact_version"`
	EventHash       string            `json:"event_hash"`
	ModelPath       string            `json:"model_path"`
	SHA256          string            `json:"sha256"`
	SizeBytes       int64             `json:"size_bytes"`
	FileName        string            `json:"file_name"`
	ContentType     string            `json:"content_type"`
	BinaryReference EvidenceReference `json:"binary_reference"`
}

func (document binaryReferenceVerificationDocument) expectation() BinaryReferenceExpectation {
	return BinaryReferenceExpectation{
		ModelPath: document.ModelPath, SHA256: document.SHA256, SizeBytes: document.SizeBytes,
		FileName: document.FileName, ContentType: document.ContentType, BinaryReference: document.BinaryReference,
	}
}

func loadBinaryEvidenceReceiptByDigest(ctx context.Context, db *sql.DB, digest string, size int64) (EvidenceReceipt, error) {
	query, args, err := goqu.From(TableBinaryEvidenceReceipt).Select(
		"provider", "bucket", "object_key", "object_version_id", "sha256", "size_bytes", "content_type", "retention_mode", "retain_until", "legal_hold",
	).Where(goqu.Ex{"sha256": digest, "size_bytes": size}).ToSQL()
	if err != nil {
		return EvidenceReceipt{}, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-BUILDBINARYRECEIPT " + err.Error())
	}
	var receipt EvidenceReceipt
	var bucket, version, retention sql.NullString
	var retainUntil sql.NullTime
	if err = db.QueryRowContext(ctx, query, args...).Scan(
		&receipt.Reference.Provider, &bucket, &receipt.Reference.ObjectKey, &version,
		&receipt.SHA256, &receipt.SizeBytes, &receipt.ContentType, &retention, &retainUntil, &receipt.LegalHold,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EvidenceReceipt{}, common.NewErrNotFound("HISTORY-MUTATIONVERIFY-BINARYRECEIPTMISSING immutable binary receipt is missing")
		}
		return EvidenceReceipt{}, common.NewInternalServerError("HISTORY-MUTATIONVERIFY-BINARYRECEIPT " + err.Error())
	}
	receipt.Reference.Bucket = bucket.String
	receipt.Reference.VersionID = version.String
	receipt.RetentionMode = retention.String
	if retainUntil.Valid {
		receipt.RetainUntil = &retainUntil.Time
	}
	return receipt, nil
}
