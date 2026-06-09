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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// Verification severity constants classify verifier findings.
const (
	VerificationSeverityError = "error"
	VerificationSeverityInfo  = "info"
)

// VerifyHistoryRangeOptions selects the history range and optional evidence object to verify.
//
// SkipEventArtifacts is intended for publisher preflight checks before existing
// PostgreSQL rows have been backfilled into WORM history_event artifacts. Normal
// verification should leave it false so missing or modified event artifacts are
// reported.
type VerifyHistoryRangeOptions struct {
	HistoryTable                string
	Identifier                  string
	FirstHistoryID              int64
	LastHistoryID               int64
	Manifest                    *HistoryManifest
	ManifestArtifactData        []byte
	ManifestArtifactContentType string
	ManifestVerifier            *ManifestJWSVerifier
	RequireSignedManifest       bool
	EvidenceStore               EvidenceStore
	ManifestArtifactRef         EvidenceReference
	ManifestArtifactHash        string
	SkipEventArtifacts          bool
}

// HistoryEvidenceVerificationReport summarizes hash-chain, manifest, and artifact verification.
//
//revive:disable-next-line:exported
type HistoryEvidenceVerificationReport struct {
	Valid          bool                  `json:"valid"`
	HistoryTable   string                `json:"history_table"`
	Identifier     string                `json:"identifier,omitempty"`
	FirstHistoryID int64                 `json:"first_history_id"`
	LastHistoryID  int64                 `json:"last_history_id"`
	FirstRowHash   string                `json:"first_row_hash,omitempty"`
	LastRowHash    string                `json:"last_row_hash,omitempty"`
	RowCount       int64                 `json:"row_count"`
	RangeDigest    string                `json:"range_digest,omitempty"`
	Findings       []VerificationFinding `json:"findings,omitempty"`
}

// VerificationFinding records one verifier observation.
type VerificationFinding struct {
	Severity   string `json:"severity"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Identifier string `json:"identifier,omitempty"`
	HistoryID  int64  `json:"history_id,omitempty"`
}

type manifestDBRow struct {
	HistoryID  int64
	Identifier string
	RowHash    string
}

type eventArtifactReceiptRow struct {
	ArtifactID  int64
	Identifier  string
	HistoryID   int64
	RowHash     string
	ContentHash string
	Receipt     EvidenceReceipt
}

// VerifyHistoryRange verifies PostgreSQL history rows against the hash chain and optional WORM evidence.
//
// The verifier reports missing, modified, reordered, or overwritten history
// records by checking row hashes, chain continuity, range manifests, per-row
// history_event receipts, and object-store artifact hashes when an EvidenceStore
// is configured.
//
// Parameters:
//   - ctx: Request context for PostgreSQL and optional evidence-store reads.
//   - db: Database handle connected to the BaSyx PostgreSQL database.
//   - options: History range plus optional manifest and evidence-store references.
//
// Returns:
//   - *HistoryEvidenceVerificationReport: Verification status and findings.
//   - error: Error when inputs are invalid or required rows cannot be loaded.
func VerifyHistoryRange(ctx context.Context, db *sql.DB, options VerifyHistoryRangeOptions) (*HistoryEvidenceVerificationReport, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("HISTORY-EVIDENCE-VERIFY-NILDB database handle must not be nil")
	}
	if err := validateVerifyHistoryRangeOptions(options); err != nil {
		return nil, err
	}
	rows, err := loadManifestRangeDBRows(ctx, db, options.HistoryTable, options.Identifier, options.FirstHistoryID, options.LastHistoryID)
	if err != nil {
		return nil, err
	}
	report := newVerificationReport(options, rows)
	if len(rows) == 0 {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EMPTYRANGE", "no history rows exist in the requested range", "", 0)
		return report, nil
	}
	manifest := verifyManifestEvidence(report, options)
	verifyRangeDigest(report, rows)
	verifyManifestRange(report, manifest)
	verifyChainsByIdentifier(ctx, db, report, options.HistoryTable, rows)
	if !options.SkipEventArtifacts {
		verifyHistoryEventArtifacts(ctx, db, options, report)
	}
	verifyManifestArtifact(ctx, options, report)
	report.Valid = len(report.Findings) == 0
	return report, nil
}

func verifyManifestEvidence(report *HistoryEvidenceVerificationReport, options VerifyHistoryRangeOptions) *HistoryManifest {
	if len(options.ManifestArtifactData) > 0 {
		manifest, _, err := DecodeAndVerifyManifestArtifact(
			options.ManifestArtifactData,
			options.ManifestArtifactContentType,
			options.ManifestVerifier,
			options.RequireSignedManifest,
		)
		if err != nil {
			report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTSIGNATURE", fmt.Sprintf("manifest signature verification failed: %v", err), "", 0)
			return nil
		}
		return &manifest
	}
	if options.Manifest == nil {
		if options.RequireSignedManifest {
			report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTMISSING", "signed manifest is required but no manifest was provided", "", 0)
		}
		return nil
	}
	if options.Manifest.SignatureState != SignatureStateSigned {
		if options.RequireSignedManifest {
			report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTUNSIGNED", "unsigned manifest is not accepted when signing is required", "", 0)
		}
		return options.Manifest
	}
	if options.ManifestVerifier != nil || options.RequireSignedManifest {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTSIGNATURE", "signed manifest verification requires raw manifest artifact bytes", "", 0)
	}
	return options.Manifest
}

// LoadManifestRangeRows returns ordered row-hash inputs for manifest generation.
//
// Parameters:
//   - ctx: Request context for reading history metadata.
//   - db: Database handle connected to the BaSyx PostgreSQL database.
//   - table: History table to read.
//   - identifier: Optional entity identifier scope. Empty means all identifiers.
//   - firstHistoryID: Inclusive lower history_id bound.
//   - lastHistoryID: Inclusive upper history_id bound.
//
// Returns:
//   - []ManifestRangeRow: Ordered history_id and row_hash pairs.
//   - error: Error when the table is unsupported or rows cannot be read.
func LoadManifestRangeRows(ctx context.Context, db *sql.DB, table string, identifier string, firstHistoryID int64, lastHistoryID int64) ([]ManifestRangeRow, error) {
	rows, err := loadManifestRangeDBRows(ctx, db, table, identifier, firstHistoryID, lastHistoryID)
	if err != nil {
		return nil, err
	}
	manifestRows := make([]ManifestRangeRow, 0, len(rows))
	for _, row := range rows {
		manifestRows = append(manifestRows, ManifestRangeRow{HistoryID: row.HistoryID, RowHash: row.RowHash})
	}
	return manifestRows, nil
}

func validateVerifyHistoryRangeOptions(options VerifyHistoryRangeOptions) error {
	if _, err := historyPayloadTable(options.HistoryTable); err != nil {
		return err
	}
	if options.FirstHistoryID < 1 {
		return common.NewErrBadRequest("HISTORY-EVIDENCE-VERIFY-FIRSTID first history_id must be positive")
	}
	if options.LastHistoryID < options.FirstHistoryID {
		return common.NewErrBadRequest("HISTORY-EVIDENCE-VERIFY-LASTID last history_id must be greater than or equal to first history_id")
	}
	return nil
}

func loadManifestRangeDBRows(ctx context.Context, queryer historyQueryer, table string, identifier string, firstHistoryID int64, lastHistoryID int64) ([]manifestDBRow, error) {
	if _, err := historyPayloadTable(table); err != nil {
		return nil, err
	}
	dataset := goqu.From(table).
		Select(goqu.C("history_id"), goqu.C("identifier"), goqu.C("row_hash")).
		Where(
			goqu.C("history_id").Gte(firstHistoryID),
			goqu.C("history_id").Lte(lastHistoryID),
		)
	if strings.TrimSpace(identifier) != "" {
		dataset = dataset.Where(goqu.C("identifier").Eq(strings.TrimSpace(identifier)))
	}
	query, args, err := dataset.Order(goqu.C("history_id").Asc()).ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-VERIFY-BUILDRANGE " + err.Error())
	}
	sqlRows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-VERIFY-READRANGE " + err.Error())
	}
	defer func() {
		_ = sqlRows.Close()
	}()
	return scanManifestDBRows(sqlRows)
}

func scanManifestDBRows(sqlRows *sql.Rows) ([]manifestDBRow, error) {
	rows := make([]manifestDBRow, 0)
	for sqlRows.Next() {
		var row manifestDBRow
		var rowHash sql.NullString
		if err := sqlRows.Scan(&row.HistoryID, &row.Identifier, &rowHash); err != nil {
			return nil, common.NewInternalServerError("HISTORY-EVIDENCE-VERIFY-SCANRANGE " + err.Error())
		}
		row.RowHash = strings.TrimSpace(nullStringValue(rowHash))
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-VERIFY-RANGEROWS " + err.Error())
	}
	return rows, nil
}

func newVerificationReport(options VerifyHistoryRangeOptions, rows []manifestDBRow) *HistoryEvidenceVerificationReport {
	report := &HistoryEvidenceVerificationReport{
		Valid:          true,
		HistoryTable:   options.HistoryTable,
		Identifier:     strings.TrimSpace(options.Identifier),
		FirstHistoryID: options.FirstHistoryID,
		LastHistoryID:  options.LastHistoryID,
		RowCount:       int64(len(rows)),
	}
	if len(rows) > 0 {
		report.FirstRowHash = rows[0].RowHash
		report.LastRowHash = rows[len(rows)-1].RowHash
	}
	return report
}

func verifyRangeDigest(report *HistoryEvidenceVerificationReport, rows []manifestDBRow) {
	manifestRows := make([]ManifestRangeRow, 0, len(rows))
	for _, row := range rows {
		if row.RowHash == "" {
			report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MISSINGROWHASH", "row_hash is missing", row.Identifier, row.HistoryID)
		}
		manifestRows = append(manifestRows, ManifestRangeRow{HistoryID: row.HistoryID, RowHash: row.RowHash})
	}
	rangeDigest, err := ComputeRangeDigest(manifestRows)
	if err != nil {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-RANGEDIGEST", err.Error(), "", 0)
		return
	}
	report.RangeDigest = rangeDigest
}

func verifyManifestRange(report *HistoryEvidenceVerificationReport, manifest *HistoryManifest) {
	if manifest == nil {
		return
	}
	if manifest.HistoryTable != report.HistoryTable {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTTABLE", "manifest history table does not match requested table", "", 0)
	}
	if strings.TrimSpace(manifest.Identifier) != strings.TrimSpace(report.Identifier) {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTIDENTIFIER", "manifest identifier does not match requested identifier", "", 0)
	}
	if manifest.FirstHistoryID != report.FirstHistoryID || manifest.LastHistoryID != report.LastHistoryID {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTRANGE", "manifest history_id range does not match requested range", "", 0)
	}
	if manifest.RowCount != report.RowCount {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTROWCOUNT", "manifest row count does not match PostgreSQL rows", "", 0)
	}
	if manifest.FirstRowHash != report.FirstRowHash || manifest.LastRowHash != report.LastRowHash {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTROWHASH", "manifest first or last row hash does not match PostgreSQL rows", "", 0)
	}
	if manifest.RangeDigest != report.RangeDigest {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTDIGEST", "manifest range digest does not match PostgreSQL rows", "", 0)
	}
}

func verifyChainsByIdentifier(ctx context.Context, queryer historyQueryer, report *HistoryEvidenceVerificationReport, table string, rows []manifestDBRow) {
	ranges := identifierRanges(rows)
	for identifier, historyRange := range ranges {
		if err := verifyIdentifierChain(ctx, queryer, table, identifier, historyRange.firstID, historyRange.lastID); err != nil {
			report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-CHAIN", err.Error(), identifier, historyRange.lastID)
		}
	}
}

type identifierHistoryRange struct {
	firstID int64
	lastID  int64
}

func identifierRanges(rows []manifestDBRow) map[string]identifierHistoryRange {
	ranges := make(map[string]identifierHistoryRange)
	for _, row := range rows {
		current, exists := ranges[row.Identifier]
		if !exists {
			ranges[row.Identifier] = identifierHistoryRange{firstID: row.HistoryID, lastID: row.HistoryID}
			continue
		}
		if row.HistoryID < current.firstID {
			current.firstID = row.HistoryID
		}
		if row.HistoryID > current.lastID {
			current.lastID = row.HistoryID
		}
		ranges[row.Identifier] = current
	}
	return ranges
}

func verifyIdentifierChain(ctx context.Context, queryer historyQueryer, table string, identifier string, firstHistoryID int64, lastHistoryID int64) error {
	payloadTable, err := historyPayloadTable(table)
	if err != nil {
		return err
	}
	checkpointID, err := nearestSnapshotHistoryID(ctx, queryer, table, identifier, firstHistoryID)
	if err != nil {
		return err
	}
	rows, err := loadVersionChain(ctx, queryer, table, payloadTable, identifier, checkpointID, lastHistoryID)
	if err != nil {
		return err
	}
	if _, err = restoreVersionChainRows(rows); err != nil {
		return err
	}
	return nil
}

func verifyManifestArtifact(ctx context.Context, options VerifyHistoryRangeOptions, report *HistoryEvidenceVerificationReport) {
	hasObjectKey := strings.TrimSpace(options.ManifestArtifactRef.ObjectKey) != ""
	hasHash := strings.TrimSpace(options.ManifestArtifactHash) != ""
	if !hasObjectKey && !hasHash {
		return
	}
	if options.EvidenceStore == nil || !hasObjectKey || !hasHash {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-MANIFESTARTIFACTCONFIG", "manifest artifact verification requires evidence store, object key, and SHA-256", "", 0)
		return
	}
	if _, err := options.EvidenceStore.VerifyArtifact(ctx, options.ManifestArtifactRef, options.ManifestArtifactHash); err != nil {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-ARTIFACT", fmt.Sprintf("manifest artifact verification failed: %v", err), "", 0)
	}
}

func verifyHistoryEventArtifacts(ctx context.Context, db *sql.DB, options VerifyHistoryRangeOptions, report *HistoryEvidenceVerificationReport) {
	candidates, err := LoadEventArtifactCandidates(ctx, db, options.HistoryTable, options.Identifier, options.FirstHistoryID, options.LastHistoryID)
	if err != nil {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EVENTLOAD", err.Error(), "", 0)
		return
	}
	receipts, err := loadEventArtifactReceiptRows(ctx, db, options.HistoryTable, options.Identifier, options.FirstHistoryID, options.LastHistoryID)
	if err != nil {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EVENTRECEIPTS", err.Error(), "", 0)
		return
	}
	receiptIndex := indexEventArtifactReceipts(receipts)
	for _, candidate := range candidates {
		verifyEventArtifactCandidate(ctx, options, report, candidate, receiptIndex[eventArtifactKey(candidate.Identifier, candidate.HistoryID, candidate.RowHash)])
	}
}

func verifyEventArtifactCandidate(ctx context.Context, options VerifyHistoryRangeOptions, report *HistoryEvidenceVerificationReport, candidate EventArtifactCandidate, receipts []*eventArtifactReceiptRow) {
	if len(receipts) == 0 {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EVENTMISSING", "history_event artifact receipt is missing", candidate.Identifier, candidate.HistoryID)
		return
	}
	if len(receipts) > 1 {
		message := "duplicate history_event artifact receipts exist for the same history row"
		if hasConflictingEventReceipts(receipts) {
			message = "conflicting history_event artifact receipts exist for the same history row"
		}
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EVENTDUPLICATE", message, candidate.Identifier, candidate.HistoryID)
	}
	expectedSHA := SHA256Hex(candidate.Artifact.Data)
	for _, receipt := range receipts {
		verifyEventArtifactReceipt(ctx, options, report, candidate, receipt, expectedSHA)
	}
}

func hasConflictingEventReceipts(receipts []*eventArtifactReceiptRow) bool {
	if len(receipts) < 2 {
		return false
	}
	first := receipts[0].Receipt
	firstRef := first.Reference
	for _, receipt := range receipts[1:] {
		current := receipt.Receipt
		if !strings.EqualFold(first.SHA256, current.SHA256) {
			return true
		}
		if firstRef.Provider != current.Reference.Provider ||
			firstRef.Bucket != current.Reference.Bucket ||
			firstRef.ObjectKey != current.Reference.ObjectKey ||
			firstRef.VersionID != current.Reference.VersionID {
			return true
		}
	}
	return false
}

func verifyEventArtifactReceipt(ctx context.Context, options VerifyHistoryRangeOptions, report *HistoryEvidenceVerificationReport, candidate EventArtifactCandidate, receipt *eventArtifactReceiptRow, expectedSHA string) {
	if !strings.EqualFold(receipt.Receipt.SHA256, expectedSHA) {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EVENTSHA", "history_event receipt SHA-256 does not match PostgreSQL row artifact", candidate.Identifier, candidate.HistoryID)
	}
	if receipt.ContentHash != "" && receipt.ContentHash != candidate.ContentHash {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EVENTCONTENTHASH", "history_event receipt content_hash does not match PostgreSQL row", candidate.Identifier, candidate.HistoryID)
	}
	if receipt.Receipt.RetentionMode == "" || receipt.Receipt.RetainUntil == nil {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EVENTRETENTION", "history_event receipt is missing WORM retention metadata", candidate.Identifier, candidate.HistoryID)
	}
	if options.EvidenceStore == nil {
		return
	}
	if _, err := options.EvidenceStore.VerifyArtifact(ctx, receipt.Receipt.Reference, receipt.Receipt.SHA256); err != nil {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EVENTOBJECT", fmt.Sprintf("history_event artifact verification failed: %v", err), candidate.Identifier, candidate.HistoryID)
	}
	verifyEventArtifactRetentionState(ctx, options.EvidenceStore, report, candidate, receipt)
}

func verifyEventArtifactRetentionState(ctx context.Context, store EvidenceStore, report *HistoryEvidenceVerificationReport, candidate EventArtifactCandidate, receipt *eventArtifactReceiptRow) {
	if receipt.Receipt.RetentionMode == "" || receipt.Receipt.RetainUntil == nil {
		return
	}
	retentionVerifier, ok := store.(EvidenceRetentionVerifier)
	if !ok {
		return
	}
	if err := retentionVerifier.VerifyArtifactRetention(ctx, receipt.Receipt.Reference, receipt.Receipt); err != nil {
		report.addFinding(VerificationSeverityError, "HISTORY-EVIDENCE-VERIFY-EVENTRETENTIONSTATE", fmt.Sprintf("history_event artifact retention verification failed: %v", err), candidate.Identifier, candidate.HistoryID)
	}
}

func loadEventArtifactReceiptRows(ctx context.Context, queryer historyQueryer, table string, identifier string, firstHistoryID int64, lastHistoryID int64) ([]eventArtifactReceiptRow, error) {
	dataset := goqu.From(TableHistoryEvidenceArtifacts).
		Select(
			goqu.C("artifact_id"),
			goqu.C("identifier"),
			goqu.C("history_id"),
			goqu.C("row_hash"),
			goqu.C("content_hash"),
			goqu.C("provider"),
			goqu.C("bucket"),
			goqu.C("object_key"),
			goqu.C("object_version_id"),
			goqu.C("sha256"),
			goqu.C("size_bytes"),
			goqu.C("content_type"),
			goqu.C("retention_mode"),
			goqu.C("retain_until"),
			goqu.C("legal_hold"),
		).
		Where(
			goqu.C("artifact_type").Eq(EvidenceArtifactHistoryEvent),
			goqu.C("history_table").Eq(table),
			goqu.C("history_id").Gte(firstHistoryID),
			goqu.C("history_id").Lte(lastHistoryID),
		)
	if strings.TrimSpace(identifier) != "" {
		dataset = dataset.Where(goqu.C("identifier").Eq(strings.TrimSpace(identifier)))
	}
	query, args, err := dataset.Order(goqu.C("history_id").Asc(), goqu.C("artifact_id").Asc()).ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-VERIFY-BUILDEVENTRECEIPTS " + err.Error())
	}
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-VERIFY-READEVENTRECEIPTS " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()
	return scanEventArtifactReceiptRows(rows)
}

func scanEventArtifactReceiptRows(rows *sql.Rows) ([]eventArtifactReceiptRow, error) {
	receipts := make([]eventArtifactReceiptRow, 0)
	for rows.Next() {
		receipt, err := scanEventArtifactReceiptRow(rows)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-VERIFY-EVENTRECEIPTROWS " + err.Error())
	}
	return receipts, nil
}

func scanEventArtifactReceiptRow(rows *sql.Rows) (eventArtifactReceiptRow, error) {
	var row eventArtifactReceiptRow
	var identifier sql.NullString
	var rowHash sql.NullString
	var contentHash sql.NullString
	var bucket sql.NullString
	var versionID sql.NullString
	var retentionMode sql.NullString
	var retainUntil sql.NullTime
	if err := rows.Scan(
		&row.ArtifactID,
		&identifier,
		&row.HistoryID,
		&rowHash,
		&contentHash,
		&row.Receipt.Reference.Provider,
		&bucket,
		&row.Receipt.Reference.ObjectKey,
		&versionID,
		&row.Receipt.SHA256,
		&row.Receipt.SizeBytes,
		&row.Receipt.ContentType,
		&retentionMode,
		&retainUntil,
		&row.Receipt.LegalHold,
	); err != nil {
		return eventArtifactReceiptRow{}, common.NewInternalServerError("HISTORY-EVIDENCE-VERIFY-SCANEVENTRECEIPT " + err.Error())
	}
	row.Identifier = nullStringValue(identifier)
	row.RowHash = nullStringValue(rowHash)
	row.ContentHash = nullStringValue(contentHash)
	row.Receipt.Reference.Bucket = nullStringValue(bucket)
	row.Receipt.Reference.VersionID = nullStringValue(versionID)
	row.Receipt.RetentionMode = nullStringValue(retentionMode)
	if retainUntil.Valid {
		retainUntilUTC := retainUntil.Time.UTC()
		row.Receipt.RetainUntil = &retainUntilUTC
	}
	return row, nil
}

func indexEventArtifactReceipts(receipts []eventArtifactReceiptRow) map[string][]*eventArtifactReceiptRow {
	index := make(map[string][]*eventArtifactReceiptRow, len(receipts))
	for i := range receipts {
		receipt := &receipts[i]
		key := eventArtifactKey(receipt.Identifier, receipt.HistoryID, receipt.RowHash)
		index[key] = append(index[key], receipt)
	}
	return index
}

func eventArtifactKey(identifier string, historyID int64, rowHash string) string {
	return strings.TrimSpace(identifier) + "\x00" + fmt.Sprintf("%d", historyID) + "\x00" + strings.TrimSpace(rowHash)
}

func (report *HistoryEvidenceVerificationReport) addFinding(severity string, code string, message string, identifier string, historyID int64) {
	report.Valid = false
	report.Findings = append(report.Findings, VerificationFinding{
		Severity:   severity,
		Code:       code,
		Message:    message,
		Identifier: identifier,
		HistoryID:  historyID,
	})
}
