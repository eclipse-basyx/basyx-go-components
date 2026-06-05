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
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// Evidence catalog table constants identify the PostgreSQL receipt tables.
const (
	TableHistoryEvidenceManifests = "history_evidence_manifests"
	TableHistoryEvidenceArtifacts = "history_evidence_artifacts"
)

// EvidenceCatalogSnapshotReceipt pairs a stored snapshot artifact receipt with its history row.
type EvidenceCatalogSnapshotReceipt struct {
	Candidate SnapshotArtifactCandidate
	Receipt   EvidenceReceipt
}

// EvidenceCatalogEventReceipt pairs a stored history-event artifact receipt with its history row.
type EvidenceCatalogEventReceipt struct {
	Candidate EventArtifactCandidate
	Receipt   EvidenceReceipt
}

// EvidenceCatalogRecord contains the manifest and artifact receipts to persist in PostgreSQL.
type EvidenceCatalogRecord struct {
	Manifest         HistoryManifest
	ManifestReceipt  EvidenceReceipt
	EventReceipts    []EvidenceCatalogEventReceipt
	SnapshotReceipts []EvidenceCatalogSnapshotReceipt
}

// EventEvidenceRecord contains one synchronously stored history event artifact receipt.
type EventEvidenceRecord struct {
	HistoryTable string
	Identifier   string
	HistoryID    int64
	RowHash      string
	ContentHash  string
	Receipt      EvidenceReceipt
}

// RecordEvidenceCatalogTx records evidence object receipts in the PostgreSQL catalog.
//
// The catalog write is part of the caller's PostgreSQL transaction. It records
// the manifest receipt and all associated history-event and snapshot artifact
// receipts without mutating guarded history rows.
//
// Parameters:
//   - ctx: Transaction context for catalog inserts.
//   - tx: Open PostgreSQL transaction owned by the caller.
//   - record: Manifest and artifact receipts to persist.
//
// Returns:
//   - int64: Generated manifest catalog ID.
//   - error: Error when the transaction is nil, the manifest is invalid, or a
//     catalog insert fails.
func RecordEvidenceCatalogTx(ctx context.Context, tx *sql.Tx, record EvidenceCatalogRecord) (int64, error) {
	if tx == nil {
		return 0, common.NewErrBadRequest("HISTORY-EVIDENCE-CATALOG-NILTX transaction must not be nil")
	}
	if err := validateManifest(record.Manifest); err != nil {
		return 0, err
	}
	manifestID, err := insertEvidenceManifestCatalogRow(ctx, tx, record)
	if err != nil {
		return 0, err
	}
	if err = insertEvidenceArtifactCatalogRows(ctx, tx, manifestID, record); err != nil {
		return 0, err
	}
	return manifestID, nil
}

// RecordHistoryEventEvidenceArtifactTx records one WORM history event artifact receipt.
//
// This is used by the synchronous history append path after the object-store
// write succeeded but before the surrounding PostgreSQL transaction commits.
//
// Parameters:
//   - ctx: Transaction context for the receipt insert.
//   - tx: Open PostgreSQL transaction owned by the history append operation.
//   - record: History row identity and evidence object receipt.
//
// Returns:
//   - error: Error when required receipt fields are missing or the insert fails.
func RecordHistoryEventEvidenceArtifactTx(ctx context.Context, tx *sql.Tx, record EventEvidenceRecord) error {
	if tx == nil {
		return common.NewErrBadRequest("HISTORY-EVIDENCE-EVENTCATALOG-NILTX transaction must not be nil")
	}
	if record.HistoryID < 1 {
		return common.NewErrBadRequest("HISTORY-EVIDENCE-EVENTCATALOG-HISTORYID history_id must be positive")
	}
	if strings.TrimSpace(record.HistoryTable) == "" {
		return common.NewErrBadRequest("HISTORY-EVIDENCE-EVENTCATALOG-TABLE history table is required")
	}
	if strings.TrimSpace(record.Receipt.Reference.ObjectKey) == "" {
		return common.NewErrBadRequest("HISTORY-EVIDENCE-EVENTCATALOG-OBJECTKEY evidence object key is required")
	}
	query, args, err := goqu.Insert(TableHistoryEvidenceArtifacts).
		Rows(historyEventArtifactCatalogRow(record)).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-EVENTCATALOG-BUILD " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-EVENTCATALOG-INSERT " + err.Error())
	}
	return nil
}

func insertEvidenceManifestCatalogRow(ctx context.Context, tx *sql.Tx, record EvidenceCatalogRecord) (int64, error) {
	manifest := record.Manifest
	receipt := record.ManifestReceipt
	row := goqu.Record{
		"manifest_version":           manifest.ManifestVersion,
		"history_table":              manifest.HistoryTable,
		"identifier":                 nullableText(manifest.Identifier),
		"first_history_id":           manifest.FirstHistoryID,
		"last_history_id":            manifest.LastHistoryID,
		"first_row_hash":             manifest.FirstRowHash,
		"last_row_hash":              manifest.LastRowHash,
		"row_count":                  manifest.RowCount,
		"range_digest":               manifest.RangeDigest,
		"generated_at":               manifest.GeneratedAt.UTC(),
		"signature_state":            manifest.SignatureState,
		"signer_key_id":              nullableText(manifestSignerKeyID(manifest)),
		"signer_algorithm":           nullableText(manifestSignerAlgorithm(manifest)),
		"snapshot_reference_count":   len(manifest.SnapshotReferences),
		"provider":                   receipt.Reference.Provider,
		"bucket":                     nullableText(receipt.Reference.Bucket),
		"manifest_object_key":        receipt.Reference.ObjectKey,
		"manifest_object_version_id": nullableText(receipt.Reference.VersionID),
		"manifest_sha256":            receipt.SHA256,
		"retention_mode":             nullableText(receipt.RetentionMode),
		"retain_until":               nullableTime(receipt.RetainUntil),
		"legal_hold":                 receipt.LegalHold,
		"artifact_metadata":          jsonbMetadata(receipt.Metadata),
	}
	query, args, err := goqu.Insert(TableHistoryEvidenceManifests).
		Rows(row).
		Returning(goqu.C("manifest_id")).
		ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("HISTORY-EVIDENCE-CATALOG-BUILDMANIFEST " + err.Error())
	}
	var manifestID int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&manifestID); err != nil {
		return 0, common.NewInternalServerError("HISTORY-EVIDENCE-CATALOG-INSERTMANIFEST " + err.Error())
	}
	return manifestID, nil
}

func insertEvidenceArtifactCatalogRows(ctx context.Context, tx *sql.Tx, manifestID int64, record EvidenceCatalogRecord) error {
	rows := []goqu.Record{manifestArtifactCatalogRow(manifestID, record)}
	for _, eventReceipt := range record.EventReceipts {
		rows = append(rows, evidenceEventArtifactCatalogRow(eventReceipt))
	}
	for _, snapshotReceipt := range record.SnapshotReceipts {
		rows = append(rows, snapshotArtifactCatalogRow(manifestID, record.Manifest, snapshotReceipt))
	}
	query, args, err := goqu.Insert(TableHistoryEvidenceArtifacts).
		Rows(rows).
		OnConflict(goqu.DoNothing()).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-CATALOG-BUILDARTIFACTS " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-CATALOG-INSERTARTIFACTS " + err.Error())
	}
	return nil
}

func manifestArtifactCatalogRow(manifestID int64, record EvidenceCatalogRecord) goqu.Record {
	manifest := record.Manifest
	receipt := record.ManifestReceipt
	return baseArtifactCatalogRow(manifestID, EvidenceArtifactManifest, manifest.HistoryTable, manifest.Identifier, receipt, goqu.Record{
		"history_id":   nil,
		"row_hash":     nil,
		"content_hash": nil,
	})
}

func snapshotArtifactCatalogRow(manifestID int64, manifest HistoryManifest, snapshotReceipt EvidenceCatalogSnapshotReceipt) goqu.Record {
	candidate := snapshotReceipt.Candidate
	row := baseArtifactCatalogRow(manifestID, EvidenceArtifactSnapshot, manifest.HistoryTable, candidate.Identifier, snapshotReceipt.Receipt, goqu.Record{
		"history_id":   candidate.HistoryID,
		"row_hash":     nullableText(candidate.RowHash),
		"content_hash": nullableText(candidate.ContentHash),
	})
	return row
}

func evidenceEventArtifactCatalogRow(eventReceipt EvidenceCatalogEventReceipt) goqu.Record {
	candidate := eventReceipt.Candidate
	return historyEventArtifactCatalogRow(EventEvidenceRecord{
		HistoryTable: candidate.HistoryTable,
		Identifier:   candidate.Identifier,
		HistoryID:    candidate.HistoryID,
		RowHash:      candidate.RowHash,
		ContentHash:  candidate.ContentHash,
		Receipt:      eventReceipt.Receipt,
	})
}

func historyEventArtifactCatalogRow(record EventEvidenceRecord) goqu.Record {
	row := baseArtifactCatalogRow(0, EvidenceArtifactHistoryEvent, record.HistoryTable, record.Identifier, record.Receipt, goqu.Record{
		"history_id":   record.HistoryID,
		"row_hash":     nullableText(record.RowHash),
		"content_hash": nullableText(record.ContentHash),
	})
	row["manifest_id"] = nil
	return row
}

func baseArtifactCatalogRow(manifestID int64, artifactType string, table string, identifier string, receipt EvidenceReceipt, extra goqu.Record) goqu.Record {
	row := goqu.Record{
		"manifest_id":       manifestID,
		"artifact_type":     artifactType,
		"history_table":     table,
		"identifier":        nullableText(identifier),
		"provider":          receipt.Reference.Provider,
		"bucket":            nullableText(receipt.Reference.Bucket),
		"object_key":        receipt.Reference.ObjectKey,
		"object_version_id": nullableText(receipt.Reference.VersionID),
		"sha256":            receipt.SHA256,
		"size_bytes":        receipt.SizeBytes,
		"content_type":      receipt.ContentType,
		"retention_mode":    nullableText(receipt.RetentionMode),
		"retain_until":      nullableTime(receipt.RetainUntil),
		"legal_hold":        receipt.LegalHold,
		"artifact_metadata": jsonbMetadata(receipt.Metadata),
	}
	for key, value := range extra {
		row[key] = value
	}
	return row
}

func manifestSignerKeyID(manifest HistoryManifest) string {
	if manifest.Signer == nil {
		return ""
	}
	return manifest.Signer.KeyID
}

func manifestSignerAlgorithm(manifest HistoryManifest) string {
	if manifest.Signer == nil {
		return ""
	}
	return manifest.Signer.Algorithm
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func jsonbMetadata(metadata map[string]string) any {
	if metadata == nil {
		metadata = map[string]string{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return goqu.L("?::jsonb", "{}")
	}
	return goqu.L("?::jsonb", string(encoded))
}
