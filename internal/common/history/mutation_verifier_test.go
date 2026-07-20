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
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestVerifyMutationEvidenceRangeRestoresSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	artifact := mutationVerificationTestArtifact(t, "")
	store := newMemoryEvidenceStore()
	store.put(artifact)
	expectMutationVerificationRows(mock, artifact, nil)

	report, err := VerifyMutationEvidenceRange(t.Context(), db, store, TableSubmodel, "sm-1", 1, 1, artifact.Metadata["event_hash"])
	require.NoError(t, err)
	require.True(t, report.Valid, report.Findings)
	require.Equal(t, "sm-1", report.Snapshot["id"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyMutationEvidenceRangeReportsMissingBinaryReference(t *testing.T) {
	managedPath := "/aasx/files/token/manual.pdf"
	artifact := mutationVerificationTestArtifact(t, managedPath)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	store := newMemoryEvidenceStore()
	store.put(artifact)
	expectMutationVerificationRows(mock, artifact, []string{})

	report, err := VerifyMutationEvidenceRange(t.Context(), db, store, TableSubmodel, "sm-1", 1, 1, artifact.Metadata["event_hash"])
	require.NoError(t, err)
	require.False(t, report.Valid)
	require.Contains(t, mutationVerificationFindingCodes(report), "HISTORY-MUTATIONVERIFY-BINARYREFMISSING")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyMutationEvidenceRangeReportsMissingRequestedTail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	artifact := mutationVerificationTestArtifact(t, "")
	store := newMemoryEvidenceStore()
	store.put(artifact)
	expectMutationVerificationRows(mock, artifact, nil)

	report, err := VerifyMutationEvidenceRange(t.Context(), db, store, TableSubmodel, "sm-1", 1, 2, artifact.Metadata["event_hash"])
	require.NoError(t, err)
	require.False(t, report.Valid)
	require.Contains(t, mutationVerificationFindingCodes(report), "HISTORY-MUTATIONVERIFY-MISSINGTAIL")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyMutationEvidenceRangeFailsWhenRetentionCannotBeChecked(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	artifact := mutationVerificationTestArtifact(t, "")
	memoryStore := newMemoryEvidenceStore()
	memoryStore.put(artifact)
	store := &evidenceStoreWithoutRetentionVerifier{delegate: memoryStore}
	expectMutationVerificationRows(mock, artifact, nil)

	report, err := VerifyMutationEvidenceRange(t.Context(), db, store, TableSubmodel, "sm-1", 1, 1, artifact.Metadata["event_hash"])
	require.NoError(t, err)
	require.False(t, report.Valid)
	require.Contains(t, mutationVerificationFindingCodes(report), "HISTORY-MUTATIONVERIFY-RETENTIONVERIFIER")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyMutationEvidenceRecoveryReportsDeletionState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	artifact := mutationVerificationTestArtifact(t, "")
	var body map[string]any
	require.NoError(t, decodeJSONPreservingNumbers(artifact.Data, &body))
	body["change_type"] = ChangeDeleted
	body["deleted"] = true
	delete(body, "event_hash")
	eventHash, hashErr := CanonicalJSONHash(body)
	require.NoError(t, hashErr)
	body["event_hash"] = eventHash
	artifact.Data, err = CanonicalJSON(body)
	require.NoError(t, err)
	artifact.Metadata["event_hash"] = eventHash
	store := newMemoryEvidenceStore()
	store.put(artifact)
	expectMutationVerificationRows(mock, artifact, nil)

	report, err := VerifyMutationEvidenceRange(t.Context(), db, store, TableSubmodel, "sm-1", 1, 1, eventHash)
	require.NoError(t, err)
	require.True(t, report.Valid, report.Findings)
	require.True(t, report.Deleted)
	require.Equal(t, ChangeDeleted, report.ChangeType)
	require.Equal(t, eventHash, report.EventHash)
	require.Equal(t, "2026-07-19T12:00:00Z", report.OperationTime)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyMutationEvidenceRangeRejectsUnexpectedHead(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	artifact := mutationVerificationTestArtifact(t, "")
	store := newMemoryEvidenceStore()
	store.put(artifact)
	expectMutationVerificationRows(mock, artifact, nil)

	report, err := VerifyMutationEvidenceRange(t.Context(), db, store, TableSubmodel, "sm-1", 1, 1, strings.Repeat("f", 64))
	require.NoError(t, err)
	require.False(t, report.Valid)
	require.Contains(t, mutationVerificationFindingCodes(report), "HISTORY-MUTATIONVERIFY-HEADMISMATCH")
	require.NoError(t, mock.ExpectationsWereMet())
}

func mutationVerificationTestArtifact(t *testing.T, expectedBinaryPath string) EvidenceArtifact {
	t.Helper()
	snapshot := map[string]any{"id": "sm-1", "submodelElements": []any{}}
	contentHash, err := CanonicalJSONHash(snapshot)
	require.NoError(t, err)
	payloadHash, err := CanonicalJSONHash(snapshot)
	require.NoError(t, err)
	expectedBinaryReferences := []BinaryReferenceExpectation{}
	if expectedBinaryPath != "" {
		expectedBinaryReferences = append(expectedBinaryReferences, BinaryReferenceExpectation{
			ModelPath: expectedBinaryPath, SHA256: strings.Repeat("a", 64), SizeBytes: 12,
			FileName: "manual.pdf", ContentType: "application/pdf",
			BinaryReference: EvidenceReference{
				Provider: EvidenceProviderS3, Bucket: "history-evidence",
				ObjectKey: "binary-content/aa/" + strings.Repeat("a", 64), VersionID: "binary-version-1",
			},
		})
	}
	body := map[string]any{
		"artifact_version": mutationEventArtifactVersion, "hash_contract": mutationEventHashContract,
		"entity_type": TableSubmodel, "identifier": "sm-1", "event_sequence": int64(1),
		"change_type": ChangeCreated, "deleted": false, "operation_time": "2026-07-19T12:00:00Z",
		"administration": map[string]any{"created_at_text": "", "updated_at_text": ""},
		"payload_type":   PayloadTypeSnapshot, "payload": snapshot, "effective_diff": []map[string]any{},
		"content_hash": contentHash, "payload_hash": payloadHash, "previous_event_hash": "",
		"binary_references_expected": expectedBinaryReferences,
		"audit":                      map[string]any{},
	}
	eventHash, err := CanonicalJSONHash(body)
	require.NoError(t, err)
	body["event_hash"] = eventHash
	data, err := CanonicalJSON(body)
	require.NoError(t, err)
	return EvidenceArtifact{
		ArtifactType: EvidenceArtifactMutation, ObjectKey: "mutation-events/submodel/sm-1/1.json",
		ContentType: manifestJSONContentType, Data: data,
		Metadata: map[string]string{"event_hash": eventHash, "content_hash": contentHash, "payload_hash": payloadHash},
	}
}

func expectMutationVerificationRows(mock sqlmock.Sqlmock, artifact EvidenceArtifact, expectedBinaryPaths []string) {
	mock.ExpectQuery(`SELECT "event_sequence" FROM "mutation_evidence_artifacts"`).
		WillReturnRows(sqlmock.NewRows([]string{"event_sequence"}).AddRow(1))
	retainUntil := time.Now().UTC().Add(24 * time.Hour)
	mock.ExpectQuery(`SELECT .* FROM "mutation_evidence_artifacts"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"artifact_id", "event_sequence", "event_hash", "previous_event_hash", "content_hash", "payload_hash", "payload_type",
			"provider", "bucket", "object_key", "object_version_id", "sha256", "size_bytes", "content_type", "retention_mode", "retain_until", "legal_hold",
		}).AddRow(
			1, 1, artifact.Metadata["event_hash"], nil, artifact.Metadata["content_hash"], artifact.Metadata["payload_hash"], PayloadTypeSnapshot,
			EvidenceProviderS3, "history-evidence", artifact.ObjectKey, "version-1", SHA256Hex(artifact.Data), len(artifact.Data), manifestJSONContentType, "governance", retainUntil, false,
		))
	if expectedBinaryPaths != nil {
		mock.ExpectQuery(`SELECT .* FROM "binary_reference_evidence_artifacts"`).
			WillReturnRows(sqlmock.NewRows([]string{
				"model_path", "provider", "bucket", "object_key", "object_version_id", "sha256", "size_bytes", "content_type", "retention_mode", "retain_until", "legal_hold",
			}))
	}
}

func mutationVerificationFindingCodes(report *MutationEvidenceVerificationReport) []string {
	codes := make([]string, 0, len(report.Findings))
	for _, finding := range report.Findings {
		codes = append(codes, finding.Code)
	}
	return codes
}

type evidenceStoreWithoutRetentionVerifier struct {
	delegate *memoryEvidenceStore
}

func (store *evidenceStoreWithoutRetentionVerifier) PutArtifact(ctx context.Context, artifact EvidenceArtifact) (*EvidenceReceipt, error) {
	return store.delegate.PutArtifact(ctx, artifact)
}

func (store *evidenceStoreWithoutRetentionVerifier) GetArtifact(ctx context.Context, ref EvidenceReference) (*EvidenceObject, error) {
	return store.delegate.GetArtifact(ctx, ref)
}

func (store *evidenceStoreWithoutRetentionVerifier) VerifyArtifact(ctx context.Context, ref EvidenceReference, expectedHash string) (*EvidenceReceipt, error) {
	return store.delegate.VerifyArtifact(ctx, ref, expectedHash)
}
