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
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// WriteHistoryEvidenceOptions selects the history range to publish as WORM evidence.
type WriteHistoryEvidenceOptions struct {
	DB             *sql.DB
	Store          EvidenceStore
	HistoryTable   string
	Identifier     string
	FirstHistoryID int64
	LastHistoryID  int64
	GeneratedAt    time.Time
	Signer         *ManifestJWSSigner
}

// WriteHistoryEvidenceResult summarizes the evidence publication.
type WriteHistoryEvidenceResult struct {
	Manifest           HistoryManifest
	ManifestReceipt    EvidenceReceipt
	EventReceipts      []EvidenceCatalogEventReceipt
	SnapshotReceipts   []EvidenceCatalogSnapshotReceipt
	ManifestID         int64
	VerificationReport *HistoryEvidenceVerificationReport
}

// WriteHistoryEvidence verifies a history range, stores event/snapshot/manifest artifacts, and records receipts.
//
// The function first verifies PostgreSQL hashes, then backfills every row in the
// requested range as a history_event artifact, stores snapshot checkpoints and a
// manifest artifact, and finally records all receipts in PostgreSQL. It is used
// by tooling and does not change the HTTP history API.
//
// Parameters:
//   - ctx: Request context for database and evidence-store operations.
//   - options: History range, evidence store, database handle, timestamp, and signer.
//
// Returns:
//   - *WriteHistoryEvidenceResult: Stored manifest, artifact receipts, catalog ID,
//     and PostgreSQL verification report.
//   - error: Error when PostgreSQL verification fails, WORM writes fail, or
//     catalog persistence fails.
func WriteHistoryEvidence(ctx context.Context, options WriteHistoryEvidenceOptions) (*WriteHistoryEvidenceResult, error) {
	if err := validateWriteHistoryEvidenceOptions(options); err != nil {
		return nil, err
	}
	report, err := VerifyHistoryRange(ctx, options.DB, VerifyHistoryRangeOptions{
		HistoryTable:       options.HistoryTable,
		Identifier:         options.Identifier,
		FirstHistoryID:     options.FirstHistoryID,
		LastHistoryID:      options.LastHistoryID,
		SkipEventArtifacts: true,
	})
	if err != nil {
		return nil, err
	}
	if !report.Valid {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-WRITE-VERIFYFAILED PostgreSQL history range is not verifiable: %s", firstFindingMessage(report))
	}
	rows, err := LoadManifestRangeRows(ctx, options.DB, options.HistoryTable, options.Identifier, options.FirstHistoryID, options.LastHistoryID)
	if err != nil {
		return nil, err
	}
	eventReceipts, err := storeHistoryEventArtifacts(ctx, options)
	if err != nil {
		return nil, err
	}
	snapshotReceipts, snapshotRefs, err := storeSnapshotArtifacts(ctx, options)
	if err != nil {
		return nil, err
	}
	manifest, err := BuildHistoryManifest(HistoryManifestOptions{
		HistoryTable:       options.HistoryTable,
		Identifier:         options.Identifier,
		Rows:               rows,
		SnapshotReferences: snapshotRefs,
		GeneratedAt:        options.GeneratedAt,
	})
	if err != nil {
		return nil, err
	}
	manifestReceipt, manifest, err := storeManifestArtifact(ctx, options.Store, manifest, options.Signer)
	if err != nil {
		return nil, err
	}
	manifestID, err := recordEvidenceCatalog(ctx, options.DB, manifest, manifestReceipt, eventReceipts, snapshotReceipts)
	if err != nil {
		return nil, err
	}
	return &WriteHistoryEvidenceResult{
		Manifest:           manifest,
		ManifestReceipt:    *manifestReceipt,
		EventReceipts:      eventReceipts,
		SnapshotReceipts:   snapshotReceipts,
		ManifestID:         manifestID,
		VerificationReport: report,
	}, nil
}

func validateWriteHistoryEvidenceOptions(options WriteHistoryEvidenceOptions) error {
	if options.DB == nil {
		return common.NewErrBadRequest("HISTORY-EVIDENCE-WRITE-NILDB database handle must not be nil")
	}
	if options.Store == nil {
		return common.NewErrBadRequest("HISTORY-EVIDENCE-WRITE-NILSTORE evidence store must not be nil")
	}
	return validateVerifyHistoryRangeOptions(VerifyHistoryRangeOptions{
		HistoryTable:   options.HistoryTable,
		Identifier:     options.Identifier,
		FirstHistoryID: options.FirstHistoryID,
		LastHistoryID:  options.LastHistoryID,
	})
}

func storeHistoryEventArtifacts(ctx context.Context, options WriteHistoryEvidenceOptions) ([]EvidenceCatalogEventReceipt, error) {
	candidates, err := LoadEventArtifactCandidates(ctx, options.DB, options.HistoryTable, options.Identifier, options.FirstHistoryID, options.LastHistoryID)
	if err != nil {
		return nil, err
	}
	receipts := make([]EvidenceCatalogEventReceipt, 0, len(candidates))
	for _, candidate := range candidates {
		receipt, err := options.Store.PutArtifact(ctx, candidate.Artifact)
		if err != nil {
			return nil, fmt.Errorf("HISTORY-EVIDENCE-WRITE-PUTEVENT %w", err)
		}
		if receipt == nil {
			return nil, fmt.Errorf("HISTORY-EVIDENCE-WRITE-NILEVENTRECEIPT evidence store returned nil receipt")
		}
		receipts = append(receipts, EvidenceCatalogEventReceipt{Candidate: candidate, Receipt: *receipt})
	}
	return receipts, nil
}

func storeSnapshotArtifacts(ctx context.Context, options WriteHistoryEvidenceOptions) ([]EvidenceCatalogSnapshotReceipt, []SnapshotArtifactReference, error) {
	candidates, err := LoadSnapshotArtifactCandidates(ctx, options.DB, options.HistoryTable, options.Identifier, options.FirstHistoryID, options.LastHistoryID)
	if err != nil {
		return nil, nil, err
	}
	receipts := make([]EvidenceCatalogSnapshotReceipt, 0, len(candidates))
	refs := make([]SnapshotArtifactReference, 0, len(candidates))
	for _, candidate := range candidates {
		receipt, err := options.Store.PutArtifact(ctx, candidate.Artifact)
		if err != nil {
			return nil, nil, fmt.Errorf("HISTORY-EVIDENCE-WRITE-PUTSNAPSHOT %w", err)
		}
		ref, err := candidate.SnapshotReference(receipt)
		if err != nil {
			return nil, nil, err
		}
		receipts = append(receipts, EvidenceCatalogSnapshotReceipt{Candidate: candidate, Receipt: *receipt})
		refs = append(refs, ref)
	}
	return receipts, refs, nil
}

func storeManifestArtifact(ctx context.Context, store EvidenceStore, manifest HistoryManifest, signer *ManifestJWSSigner) (*EvidenceReceipt, HistoryManifest, error) {
	artifact, finalManifest, err := BuildManifestEvidenceArtifact(manifest, "", signer)
	if err != nil {
		return nil, HistoryManifest{}, err
	}
	receipt, err := store.PutArtifact(ctx, artifact)
	if err != nil {
		return nil, HistoryManifest{}, fmt.Errorf("HISTORY-EVIDENCE-WRITE-PUTMANIFEST %w", err)
	}
	return receipt, finalManifest, nil
}

func recordEvidenceCatalog(ctx context.Context, db *sql.DB, manifest HistoryManifest, manifestReceipt *EvidenceReceipt, eventReceipts []EvidenceCatalogEventReceipt, snapshotReceipts []EvidenceCatalogSnapshotReceipt) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, common.NewInternalServerError("HISTORY-EVIDENCE-WRITE-BEGINTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	manifestID, err := RecordEvidenceCatalogTx(ctx, tx, EvidenceCatalogRecord{
		Manifest:         manifest,
		ManifestReceipt:  *manifestReceipt,
		EventReceipts:    eventReceipts,
		SnapshotReceipts: snapshotReceipts,
	})
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, common.NewInternalServerError("HISTORY-EVIDENCE-WRITE-COMMIT " + err.Error())
	}
	committed = true
	return manifestID, nil
}

func firstFindingMessage(report *HistoryEvidenceVerificationReport) string {
	if report == nil || len(report.Findings) == 0 {
		return "unknown verifier failure"
	}
	finding := report.Findings[0]
	return strings.TrimSpace(finding.Code + " " + finding.Message)
}
