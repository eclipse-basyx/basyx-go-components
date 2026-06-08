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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecoverHistoryFromEvidenceRecoversSnapshotArtifact(t *testing.T) {
	store := newMemoryEvidenceStore()
	row := recoveryTestEvent(t, 1, PayloadTypeSnapshot, "", map[string]any{"id": "aas-1", "value": "v1"}, nil)
	store.put(row.Artifact)
	catalog := recoveryTestCatalog(row)

	report, err := RecoverHistoryFromEvidence(t.Context(), store, catalog)

	require.NoError(t, err)
	require.True(t, report.Valid)
	require.Len(t, report.RecoveredRows, 1)
	require.Equal(t, "v1", report.RecoveredRows[0].Snapshot["value"])
}

func TestRecoverHistoryFromEvidenceReplaysDiffArtifact(t *testing.T) {
	store := newMemoryEvidenceStore()
	v1 := map[string]any{"id": "aas-1", "value": "v1"}
	v2 := map[string]any{"id": "aas-1", "value": "v2"}
	row1 := recoveryTestEvent(t, 1, PayloadTypeSnapshot, "", v1, nil)
	row2 := recoveryTestEvent(t, 2, PayloadTypeDiff, row1.RowHash, v2, v1)
	store.put(row1.Artifact)
	store.put(row2.Artifact)
	catalog := recoveryTestCatalog(row1, row2)
	catalog.FirstHistoryID = 2

	report, err := RecoverHistoryFromEvidence(t.Context(), store, catalog)

	require.NoError(t, err)
	require.True(t, report.Valid)
	require.Len(t, report.RecoveredRows, 1)
	require.Equal(t, int64(2), report.RecoveredRows[0].HistoryID)
	require.Equal(t, "v2", report.RecoveredRows[0].Snapshot["value"])
}

func TestRecoverHistoryFromEvidenceDetectsMissingArtifact(t *testing.T) {
	row := recoveryTestEvent(t, 1, PayloadTypeSnapshot, "", map[string]any{"id": "aas-1"}, nil)
	catalog := recoveryTestCatalog(row)

	report, err := RecoverHistoryFromEvidence(t.Context(), newMemoryEvidenceStore(), catalog)

	require.NoError(t, err)
	require.False(t, report.Valid)
	require.Empty(t, report.RecoveredRows)
	require.Contains(t, report.Findings[0].Code, "HISTORY-RECOVERY-ARTIFACT")
}

func TestRecoverHistoryFromEvidenceDetectsTamperedArtifact(t *testing.T) {
	store := newMemoryEvidenceStore()
	row := recoveryTestEvent(t, 1, PayloadTypeSnapshot, "", map[string]any{"id": "aas-1", "value": "v1"}, nil)
	store.putTampered(row.Artifact, []byte(`{"tampered":true}`))
	catalog := recoveryTestCatalog(row)

	report, err := RecoverHistoryFromEvidence(t.Context(), store, catalog)

	require.NoError(t, err)
	require.False(t, report.Valid)
	require.Empty(t, report.RecoveredRows)
	require.Contains(t, report.Findings[0].Message, "SHA-256")
}

func TestRecoverHistoryFromEvidenceRequiresCheckpointForDiffRange(t *testing.T) {
	store := newMemoryEvidenceStore()
	v1 := map[string]any{"id": "aas-1", "value": "v1"}
	v2 := map[string]any{"id": "aas-1", "value": "v2"}
	row1 := recoveryTestEvent(t, 1, PayloadTypeSnapshot, "", v1, nil)
	row2 := recoveryTestEvent(t, 2, PayloadTypeDiff, row1.RowHash, v2, v1)
	store.put(row2.Artifact)
	catalog := recoveryTestCatalog(row2)
	catalog.FirstHistoryID = 2

	report, err := RecoverHistoryFromEvidence(t.Context(), store, catalog)

	require.NoError(t, err)
	require.False(t, report.Valid)
	require.Contains(t, report.Findings[0].Message, "diff row has no preceding snapshot")
}

type recoveryTestEventRow struct {
	Artifact    EvidenceArtifact
	Receipt     EvidenceReceipt
	Identifier  string
	HistoryID   int64
	RowHash     string
	ContentHash string
}

func recoveryTestEvent(t *testing.T, historyID int64, payloadType string, previousHash string, current map[string]any, previous map[string]any) recoveryTestEventRow {
	t.Helper()
	operationTime := time.Date(2026, 6, 5, 12, 0, 0, int(historyID), time.UTC)
	payloadValue := any(current)
	if payloadType == PayloadTypeDiff {
		patch, err := BuildJSONPatch(previous, current)
		require.NoError(t, err)
		payloadValue = patch
	}
	payloadJSON, err := CanonicalJSON(payloadValue)
	require.NoError(t, err)
	contentHash, err := CanonicalJSONHash(current)
	require.NoError(t, err)
	payloadHash, err := CanonicalJSONHash(payloadValue)
	require.NoError(t, err)
	event := ChangeEvent{
		EntityType:   TableAAS,
		Identifier:   "aas-1",
		ChangeType:   ChangeUpdated,
		Timestamp:    operationTime,
		Deleted:      false,
		PayloadType:  payloadType,
		ContentHash:  contentHash,
		PayloadHash:  payloadHash,
		PreviousHash: previousHash,
	}
	if historyID == 1 {
		event.ChangeType = ChangeCreated
	}
	rowHash, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)
	event.RowHash = rowHash
	effectiveDiff := []map[string]any{}
	if payloadType == PayloadTypeDiff {
		require.NoError(t, decodeJSONPreservingNumbers(payloadJSON, &effectiveDiff))
	}
	artifact, err := buildHistoryEventEvidenceArtifact(
		TableAAS,
		historyID,
		event,
		historyPayload{payloadType: payloadType, json: payloadJSON, hash: payloadHash},
		effectiveDiff,
		"",
		"",
	)
	require.NoError(t, err)
	receipt := EvidenceReceipt{
		Reference: EvidenceReference{
			Provider:  EvidenceProviderS3,
			Bucket:    "history-evidence",
			ObjectKey: artifact.ObjectKey,
			VersionID: "version-1",
		},
		SHA256:      SHA256Hex(artifact.Data),
		SizeBytes:   int64(len(artifact.Data)),
		ContentType: artifact.ContentType,
		StoredAt:    operationTime,
	}
	return recoveryTestEventRow{
		Artifact:    artifact,
		Receipt:     receipt,
		Identifier:  event.Identifier,
		HistoryID:   historyID,
		RowHash:     rowHash,
		ContentHash: contentHash,
	}
}

func recoveryTestCatalog(rows ...recoveryTestEventRow) EvidenceRecoveryCatalog {
	catalog := EvidenceRecoveryCatalog{
		HistoryTable:   TableAAS,
		Identifier:     "aas-1",
		FirstHistoryID: rows[0].HistoryID,
		LastHistoryID:  rows[len(rows)-1].HistoryID,
	}
	for _, row := range rows {
		catalog.EventArtifacts = append(catalog.EventArtifacts, EvidenceRecoveryArtifact{
			Identifier:  row.Identifier,
			HistoryID:   row.HistoryID,
			RowHash:     row.RowHash,
			ContentHash: row.ContentHash,
			Receipt:     row.Receipt,
		})
	}
	return catalog
}

type memoryEvidenceStore struct {
	objects map[string]EvidenceObject
}

func newMemoryEvidenceStore() *memoryEvidenceStore {
	return &memoryEvidenceStore{objects: map[string]EvidenceObject{}}
}

func (store *memoryEvidenceStore) put(artifact EvidenceArtifact) {
	store.putTampered(artifact, artifact.Data)
}

func (store *memoryEvidenceStore) putTampered(artifact EvidenceArtifact, data []byte) {
	ref := EvidenceReference{Provider: EvidenceProviderS3, Bucket: "history-evidence", ObjectKey: artifact.ObjectKey, VersionID: "version-1"}
	store.objects[artifact.ObjectKey] = EvidenceObject{Reference: ref, Data: data, ContentType: artifact.ContentType}
}

func (store *memoryEvidenceStore) PutArtifact(_ context.Context, _ EvidenceArtifact) (*EvidenceReceipt, error) {
	return nil, errors.New("not implemented")
}

func (store *memoryEvidenceStore) GetArtifact(_ context.Context, ref EvidenceReference) (*EvidenceObject, error) {
	object, ok := store.objects[ref.ObjectKey]
	if !ok {
		return nil, errors.New("missing artifact")
	}
	return &object, nil
}

func (store *memoryEvidenceStore) VerifyArtifact(_ context.Context, ref EvidenceReference, expectedHash string) (*EvidenceReceipt, error) {
	object, err := store.GetArtifact(context.TODO(), ref)
	if err != nil {
		return nil, err
	}
	if actual := SHA256Hex(object.Data); actual != expectedHash {
		return nil, errors.New("hash mismatch")
	}
	return &EvidenceReceipt{Reference: ref, SHA256: expectedHash}, nil
}
