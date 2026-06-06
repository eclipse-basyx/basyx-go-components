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
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

func TestBuildHistoryManifestComputesDeterministicRangeDigest(t *testing.T) {
	generatedAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	rows := []ManifestRangeRow{
		{HistoryID: 1, RowHash: strings.Repeat("a", 64)},
		{HistoryID: 2, RowHash: strings.Repeat("b", 64)},
	}

	first, err := BuildHistoryManifest(HistoryManifestOptions{HistoryTable: TableAAS, Identifier: "aas-1", Rows: rows, GeneratedAt: generatedAt})
	require.NoError(t, err)
	second, err := BuildHistoryManifest(HistoryManifestOptions{HistoryTable: TableAAS, Identifier: "aas-1", Rows: rows, GeneratedAt: generatedAt})
	require.NoError(t, err)

	require.Equal(t, first.RangeDigest, second.RangeDigest)
	require.Equal(t, int64(2), first.RowCount)
	require.Equal(t, rows[0].RowHash, first.FirstRowHash)
	require.Equal(t, rows[1].RowHash, first.LastRowHash)
	firstJSON, err := CanonicalManifestJSON(first)
	require.NoError(t, err)
	secondJSON, err := CanonicalManifestJSON(second)
	require.NoError(t, err)
	require.Equal(t, firstJSON, secondJSON)
}

func TestBuildHistoryManifestRejectsReorderedRows(t *testing.T) {
	_, err := BuildHistoryManifest(HistoryManifestOptions{
		HistoryTable: TableAAS,
		Rows: []ManifestRangeRow{
			{HistoryID: 2, RowHash: strings.Repeat("b", 64)},
			{HistoryID: 1, RowHash: strings.Repeat("a", 64)},
		},
	})

	require.ErrorContains(t, err, "HISTORY-MANIFEST-ORDER")
}

func TestBuildManifestEvidenceArtifactSignsCanonicalManifest(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	manifest, err := BuildHistoryManifest(HistoryManifestOptions{
		HistoryTable: TableSubmodel,
		Identifier:   "submodel-1",
		Rows: []ManifestRangeRow{
			{HistoryID: 10, RowHash: strings.Repeat("c", 64)},
		},
		GeneratedAt: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	artifact, signedManifest, err := BuildManifestEvidenceArtifact(manifest, "prefix", &ManifestJWSSigner{PrivateKey: privateKey, KeyID: "test-key"})
	require.NoError(t, err)
	decoded, signed, err := DecodeManifestArtifact(artifact.Data, artifact.ContentType)
	require.NoError(t, err)

	require.True(t, signed)
	require.Equal(t, manifestJWSContentType, artifact.ContentType)
	require.Equal(t, SignatureStateSigned, signedManifest.SignatureState)
	require.Equal(t, "test-key", signedManifest.Signer.KeyID)
	require.Equal(t, signedManifest.RangeDigest, decoded.RangeDigest)
	require.True(t, strings.HasPrefix(artifact.ObjectKey, "prefix/history-manifests/"))
}

func TestDecodeManifestArtifactTreatsJSONContentTypeAsJSONWhenPayloadContainsDots(t *testing.T) {
	manifest, err := BuildHistoryManifest(HistoryManifestOptions{
		HistoryTable: TableAAS,
		Identifier:   "urn.example.component",
		Rows: []ManifestRangeRow{
			{HistoryID: 1, RowHash: strings.Repeat("a", 64)},
		},
		GeneratedAt: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	artifact, finalManifest, err := BuildManifestEvidenceArtifact(manifest, "", nil)
	require.NoError(t, err)
	require.Equal(t, manifestJSONContentType, artifact.ContentType)
	require.Equal(t, 2, strings.Count(string(artifact.Data), "."))

	decoded, signed, err := DecodeManifestArtifact(artifact.Data, manifestJSONContentType)

	require.NoError(t, err)
	require.False(t, signed)
	require.Equal(t, finalManifest.Identifier, decoded.Identifier)
	require.Equal(t, finalManifest.RangeDigest, decoded.RangeDigest)
}

func TestStoreManifestArtifactRejectsNilReceipt(t *testing.T) {
	manifest, err := BuildHistoryManifest(HistoryManifestOptions{
		HistoryTable: TableAAS,
		Rows: []ManifestRangeRow{
			{HistoryID: 1, RowHash: strings.Repeat("a", 64)},
		},
	})
	require.NoError(t, err)

	receipt, _, err := storeManifestArtifact(t.Context(), nilReceiptEvidenceStore{}, manifest, nil)

	require.Nil(t, receipt)
	require.ErrorContains(t, err, "HISTORY-EVIDENCE-WRITE-NILMANIFESTRECEIPT")
}

func TestEvidenceEventArtifactCatalogRowLinksPublishedEventsToManifest(t *testing.T) {
	receipt := EvidenceReceipt{
		Reference: EvidenceReference{
			Provider:  EvidenceProviderS3,
			Bucket:    "history-evidence",
			ObjectKey: "history-events/aas_history/aas-1/1-rowhash.json",
			VersionID: "version-1",
		},
		SHA256:      strings.Repeat("a", 64),
		SizeBytes:   42,
		ContentType: manifestJSONContentType,
		StoredAt:    time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC),
	}
	eventReceipt := EvidenceCatalogEventReceipt{
		Candidate: EventArtifactCandidate{
			HistoryTable: TableAAS,
			Identifier:   "aas-1",
			HistoryID:    1,
			RowHash:      "rowhash",
			ContentHash:  "contenthash",
		},
		Receipt: receipt,
	}

	row := evidenceEventArtifactCatalogRow(99, eventReceipt)

	require.Equal(t, int64(99), row["manifest_id"])
	require.Equal(t, EvidenceArtifactHistoryEvent, row["artifact_type"])
	require.Equal(t, int64(1), row["history_id"])
}

func TestS3EvidenceStoreReceiptAppliesPrefixAndRetention(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	store := &S3EvidenceStore{
		cfg: S3EvidenceStoreConfig{
			Bucket:        "evidence",
			Prefix:        "tenant-a",
			RetentionMode: "governance",
			RetentionDays: 7,
		},
		now: func() time.Time { return now },
	}

	key := store.objectKey("manifests/one.json")
	receipt := store.receiptForArtifact(key, EvidenceArtifact{ArtifactType: EvidenceArtifactManifest, ContentType: manifestJSONContentType, Data: []byte(`{"ok":true}`)})

	require.Equal(t, "tenant-a/manifests/one.json", receipt.Reference.ObjectKey)
	require.Equal(t, "governance", receipt.RetentionMode)
	require.NotNil(t, receipt.RetainUntil)
	require.Equal(t, now.AddDate(0, 0, 7), *receipt.RetainUntil)
	require.Equal(t, SHA256Hex([]byte(`{"ok":true}`)), receipt.SHA256)
}

func TestS3EvidenceStorePutRequiresRetention(t *testing.T) {
	store := &S3EvidenceStore{
		client: &s3.Client{},
		cfg:    S3EvidenceStoreConfig{Bucket: "evidence", Region: "us-east-1"},
		now:    func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) },
	}

	_, err := store.PutArtifact(t.Context(), EvidenceArtifact{ArtifactType: EvidenceArtifactHistoryEvent, ObjectKey: "events/one.json", ContentType: manifestJSONContentType, Data: []byte(`{}`)})

	require.ErrorContains(t, err, "HISTORY-EVIDENCE-S3-RETENTION")
}

func TestStoreHistoryEventArtifactsBackfillsSnapshotAndDiffRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	store := &recordingEvidenceStore{}
	baseSnapshot := map[string]any{"id": "aas-1", "idShort": "before", "description": strings.Repeat("unchanged-", 40)}
	nextSnapshot := map[string]any{"id": "aas-1", "idShort": "after", "description": strings.Repeat("unchanged-", 40)}
	patch, err := BuildJSONPatch(baseSnapshot, nextSnapshot)
	require.NoError(t, err)
	operationTime := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(newHistoryChainRows(TableAAS,
			historyChainRowSpec{HistoryID: 1, Identifier: "aas-1", ChangeType: ChangeCreated, PayloadType: PayloadTypeSnapshot, Snapshot: baseSnapshot, Deleted: false, OperationTime: operationTime},
			historyChainRowSpec{HistoryID: 2, Identifier: "aas-1", ChangeType: ChangeUpdated, PayloadType: PayloadTypeDiff, Patch: patch, ContentSnapshot: nextSnapshot, Deleted: false, OperationTime: operationTime},
		))

	receipts, err := storeHistoryEventArtifacts(t.Context(), WriteHistoryEvidenceOptions{
		DB:             db,
		Store:          store,
		HistoryTable:   TableAAS,
		Identifier:     "aas-1",
		FirstHistoryID: 1,
		LastHistoryID:  2,
	})

	require.NoError(t, err)
	require.Len(t, receipts, 2)
	require.Len(t, store.artifacts, 2)
	require.Equal(t, EvidenceArtifactHistoryEvent, store.artifacts[0].ArtifactType)
	require.Equal(t, EvidenceArtifactHistoryEvent, store.artifacts[1].ArtifactType)
	var diffArtifact map[string]any
	require.NoError(t, decodeJSONPreservingNumbers(store.artifacts[1].Data, &diffArtifact))
	require.Equal(t, PayloadTypeDiff, diffArtifact["payload_type"])
	payloadDiff, ok := diffArtifact["payload"].([]any)
	require.True(t, ok)
	effectiveDiff, ok := diffArtifact["effective_diff"].([]any)
	require.True(t, ok)
	require.Equal(t, payloadDiff, effectiveDiff)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadSnapshotArtifactCandidatesRejectsEmptyRowHash(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	snapshot := map[string]any{"id": "aas-1"}
	snapshotData, err := CanonicalJSON(snapshot)
	require.NoError(t, err)
	snapshotHash := SHA256Hex(snapshotData)
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{
			"history_id",
			"identifier",
			"content_hash",
			"payload_hash",
			"row_hash",
			"snapshot",
		}).AddRow(int64(1), "aas-1", snapshotHash, snapshotHash, "", string(snapshotData)))

	candidates, err := LoadSnapshotArtifactCandidates(t.Context(), db, TableAAS, "aas-1", 1, 1)

	require.Nil(t, candidates)
	require.ErrorContains(t, err, "HISTORY-EVIDENCE-SNAPSHOT-ROWHASH")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyHistoryRangeReportsMissingHistoryEventArtifact(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	snapshot := map[string]any{"id": "aas-1"}
	spec := historyChainRowSpec{HistoryID: 1, Identifier: "aas-1", ChangeType: ChangeCreated, PayloadType: PayloadTypeSnapshot, Snapshot: snapshot, Deleted: false, OperationTime: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)}
	_, rowHash := historyChainRow(TableAAS, spec, "")

	mock.ExpectQuery(`SELECT "history_id", "identifier", "row_hash" FROM "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id", "identifier", "row_hash"}).AddRow(1, "aas-1", rowHash))
	mock.ExpectQuery(`SELECT "history_id" FROM "aas_history".*"payload_type" = 'snapshot'`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(1))
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(newHistoryChainRows(TableAAS, spec))
	mock.ExpectQuery(`SELECT .*FROM "aas_history" AS "history" INNER JOIN "aas_history_payload" AS "payload"`).
		WillReturnRows(newHistoryChainRows(TableAAS, spec))
	mock.ExpectQuery(`SELECT .*FROM "history_evidence_artifacts"`).
		WillReturnRows(sqlmock.NewRows(eventArtifactReceiptColumns()))

	report, err := VerifyHistoryRange(t.Context(), db, VerifyHistoryRangeOptions{
		HistoryTable:   TableAAS,
		Identifier:     "aas-1",
		FirstHistoryID: 1,
		LastHistoryID:  1,
	})

	require.NoError(t, err)
	require.False(t, report.Valid)
	require.Contains(t, verificationFindingCodes(report), "HISTORY-EVIDENCE-VERIFY-EVENTMISSING")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyManifestRangeReportsDigestMismatch(t *testing.T) {
	report := &HistoryEvidenceVerificationReport{
		Valid:          true,
		HistoryTable:   TableAAS,
		Identifier:     "aas-1",
		FirstHistoryID: 1,
		LastHistoryID:  2,
		FirstRowHash:   "first",
		LastRowHash:    "last",
		RowCount:       2,
		RangeDigest:    "actual",
	}
	manifest := &HistoryManifest{
		HistoryTable:    TableAAS,
		Identifier:      "aas-1",
		FirstHistoryID:  1,
		LastHistoryID:   2,
		FirstRowHash:    "first",
		LastRowHash:     "last",
		RowCount:        2,
		RangeDigest:     "expected",
		SignatureState:  SignatureStateUnsigned,
		ManifestVersion: HistoryManifestVersion,
	}

	verifyManifestRange(report, manifest)

	require.False(t, report.Valid)
	require.Len(t, report.Findings, 1)
	require.Equal(t, "HISTORY-EVIDENCE-VERIFY-MANIFESTDIGEST", report.Findings[0].Code)
}

func eventArtifactReceiptColumns() []string {
	return []string{
		"artifact_id",
		"identifier",
		"history_id",
		"row_hash",
		"content_hash",
		"provider",
		"bucket",
		"object_key",
		"object_version_id",
		"sha256",
		"size_bytes",
		"content_type",
		"retention_mode",
		"retain_until",
		"legal_hold",
	}
}

func verificationFindingCodes(report *HistoryEvidenceVerificationReport) []string {
	codes := make([]string, 0, len(report.Findings))
	for _, finding := range report.Findings {
		codes = append(codes, finding.Code)
	}
	return codes
}

type nilReceiptEvidenceStore struct{}

func (nilReceiptEvidenceStore) PutArtifact(_ context.Context, _ EvidenceArtifact) (*EvidenceReceipt, error) {
	return nil, nil
}

func (nilReceiptEvidenceStore) GetArtifact(_ context.Context, _ EvidenceReference) (*EvidenceObject, error) {
	return nil, nil
}

func (nilReceiptEvidenceStore) VerifyArtifact(_ context.Context, _ EvidenceReference, _ string) (*EvidenceReceipt, error) {
	return nil, nil
}
