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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const snapshotArtifactVersion = "basyx-history-snapshot-v1"

// SnapshotArtifactCandidate is a full-snapshot recovery checkpoint ready for evidence storage.
type SnapshotArtifactCandidate struct {
	Artifact    EvidenceArtifact
	HistoryID   int64
	Identifier  string
	RowHash     string
	ContentHash string
}

// SnapshotReference builds a manifest reference after the snapshot artifact was stored.
//
// Parameters:
//   - receipt: Object-store receipt returned after writing the snapshot artifact.
//
// Returns:
//   - SnapshotArtifactReference: Manifest-ready pointer to the recovery snapshot.
//   - error: Error when receipt is nil.
func (candidate SnapshotArtifactCandidate) SnapshotReference(receipt *EvidenceReceipt) (SnapshotArtifactReference, error) {
	if receipt == nil {
		return SnapshotArtifactReference{}, fmt.Errorf("HISTORY-EVIDENCE-SNAPSHOT-NILRECEIPT evidence receipt must not be nil")
	}
	return SnapshotArtifactReference{
		HistoryID:   candidate.HistoryID,
		RowHash:     candidate.RowHash,
		ContentHash: candidate.ContentHash,
		SHA256:      receipt.SHA256,
		Reference:   receipt.Reference,
	}, nil
}

// LoadSnapshotArtifactCandidates loads full snapshot rows that can be archived for bounded recovery.
//
// Parameters:
//   - ctx: Request context for reading history rows.
//   - db: Database handle connected to the BaSyx PostgreSQL database.
//   - table: History table to read, for example submodel_history.
//   - identifier: Optional entity identifier scope. Empty means all identifiers.
//   - firstHistoryID: Inclusive lower history_id bound.
//   - lastHistoryID: Inclusive upper history_id bound.
//
// Returns:
//   - []SnapshotArtifactCandidate: Canonical full-snapshot artifacts in history_id order.
//   - error: Error when the table is unsupported, snapshot hashes do not validate,
//     or rows cannot be read.
func LoadSnapshotArtifactCandidates(ctx context.Context, db *sql.DB, table string, identifier string, firstHistoryID int64, lastHistoryID int64) ([]SnapshotArtifactCandidate, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("HISTORY-EVIDENCE-SNAPSHOT-NILDB database handle must not be nil")
	}
	payloadTable, err := historyPayloadTable(table)
	if err != nil {
		return nil, err
	}
	query, args, err := snapshotArtifactQuery(table, payloadTable, identifier, firstHistoryID, lastHistoryID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-SNAPSHOT-EXECQUERY " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()
	return scanSnapshotArtifactCandidates(rows, table)
}

func snapshotArtifactQuery(table string, payloadTable string, identifier string, firstHistoryID int64, lastHistoryID int64) (string, []any, error) {
	historyAlias := goqu.T(table).As("history")
	payloadAlias := goqu.T(payloadTable).As("payload")
	dataset := goqu.From(historyAlias).
		InnerJoin(payloadAlias, goqu.On(historyAlias.Col("history_id").Eq(payloadAlias.Col("history_id")))).
		Select(
			historyAlias.Col("history_id"),
			historyAlias.Col("identifier"),
			historyAlias.Col("content_hash"),
			historyAlias.Col("payload_hash"),
			historyAlias.Col("row_hash"),
			goqu.L(`"payload"."snapshot"::text`),
		).
		Where(
			historyAlias.Col("history_id").Gte(firstHistoryID),
			historyAlias.Col("history_id").Lte(lastHistoryID),
			historyAlias.Col("payload_type").Eq(PayloadTypeSnapshot),
		)
	if strings.TrimSpace(identifier) != "" {
		dataset = dataset.Where(historyAlias.Col("identifier").Eq(strings.TrimSpace(identifier)))
	}
	return dataset.Order(historyAlias.Col("history_id").Asc()).ToSQL()
}

func scanSnapshotArtifactCandidates(rows *sql.Rows, table string) ([]SnapshotArtifactCandidate, error) {
	candidates := make([]SnapshotArtifactCandidate, 0)
	for rows.Next() {
		candidate, err := scanSnapshotArtifactCandidate(rows, table)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-SNAPSHOT-ROWS " + err.Error())
	}
	return candidates, nil
}

func scanSnapshotArtifactCandidate(rows *sql.Rows, table string) (SnapshotArtifactCandidate, error) {
	var historyID int64
	var identifier string
	var contentHash sql.NullString
	var payloadHash sql.NullString
	var rowHash sql.NullString
	var snapshotText sql.NullString
	if err := rows.Scan(&historyID, &identifier, &contentHash, &payloadHash, &rowHash, &snapshotText); err != nil {
		return SnapshotArtifactCandidate{}, common.NewInternalServerError("HISTORY-EVIDENCE-SNAPSHOT-SCAN " + err.Error())
	}
	if !snapshotText.Valid {
		return SnapshotArtifactCandidate{}, common.NewInternalServerError("HISTORY-EVIDENCE-SNAPSHOT-MISSING snapshot payload is missing")
	}
	var snapshot map[string]any
	if err := decodeJSONPreservingNumbers([]byte(snapshotText.String), &snapshot); err != nil {
		return SnapshotArtifactCandidate{}, common.NewInternalServerError("HISTORY-EVIDENCE-SNAPSHOT-DECODE " + err.Error())
	}
	if err := verifyCanonicalHash(snapshot, contentHash, "HISTORY-EVIDENCE-SNAPSHOT-CONTENTHASH"); err != nil {
		return SnapshotArtifactCandidate{}, err
	}
	if err := verifyCanonicalHash(snapshot, payloadHash, "HISTORY-EVIDENCE-SNAPSHOT-PAYLOADHASH"); err != nil {
		return SnapshotArtifactCandidate{}, err
	}
	data, err := CanonicalJSON(map[string]any{
		"artifact_version": snapshotArtifactVersion,
		"history_table":    table,
		"identifier":       identifier,
		"history_id":       historyID,
		"row_hash":         nullStringValue(rowHash),
		"content_hash":     nullStringValue(contentHash),
		"payload_hash":     nullStringValue(payloadHash),
		"snapshot":         snapshot,
	})
	if err != nil {
		return SnapshotArtifactCandidate{}, common.NewInternalServerError("HISTORY-EVIDENCE-SNAPSHOT-CANONICAL " + err.Error())
	}
	return SnapshotArtifactCandidate{
		Artifact: EvidenceArtifact{
			ArtifactType: EvidenceArtifactSnapshot,
			ObjectKey:    snapshotObjectKey(table, identifier, historyID, nullStringValue(rowHash)),
			ContentType:  manifestJSONContentType,
			Data:         data,
			Metadata:     snapshotMetadata(table, identifier, historyID, nullStringValue(rowHash)),
		},
		HistoryID:   historyID,
		Identifier:  identifier,
		RowHash:     nullStringValue(rowHash),
		ContentHash: nullStringValue(contentHash),
	}, nil
}

func snapshotObjectKey(table string, identifier string, historyID int64, rowHash string) string {
	return path.Join(
		"history-snapshots",
		url.PathEscape(table),
		url.PathEscape(identifier),
		fmt.Sprintf("%d-%s.json", historyID, strings.TrimSpace(rowHash)),
	)
}

func snapshotMetadata(table string, identifier string, historyID int64, rowHash string) map[string]string {
	return map[string]string{
		"artifact_type": EvidenceArtifactSnapshot,
		"history_table": table,
		"identifier":    identifier,
		"history_id":    fmt.Sprintf("%d", historyID),
		"row_hash":      strings.TrimSpace(rowHash),
	}
}
