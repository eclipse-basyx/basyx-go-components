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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// EvidenceRecoveryCatalogOptions selects a history range whose WORM receipts should be exported.
type EvidenceRecoveryCatalogOptions struct {
	HistoryTable   string
	Identifier     string
	FirstHistoryID int64
	LastHistoryID  int64
}

// EvidenceRecoveryCatalog is a lean JSON-serializable catalog of history_event receipts.
//
// EventArtifacts may include prerequisite rows before FirstHistoryID when a
// diff-backed requested range needs an earlier WORM snapshot checkpoint and
// intervening diffs for reconstruction.
type EvidenceRecoveryCatalog struct {
	HistoryTable   string                     `json:"history_table"`
	Identifier     string                     `json:"identifier,omitempty"`
	FirstHistoryID int64                      `json:"first_history_id"`
	LastHistoryID  int64                      `json:"last_history_id"`
	EventArtifacts []EvidenceRecoveryArtifact `json:"event_artifacts"`
}

// EvidenceRecoveryArtifact links one history row to its immutable history_event receipt.
type EvidenceRecoveryArtifact struct {
	Identifier  string          `json:"identifier,omitempty"`
	HistoryID   int64           `json:"history_id"`
	RowHash     string          `json:"row_hash,omitempty"`
	ContentHash string          `json:"content_hash,omitempty"`
	Receipt     EvidenceReceipt `json:"receipt"`
}

// HistoryRecoveryReport contains verified rows reconstructed from WORM evidence.
//
//revive:disable-next-line:exported -- ticket requires the public history.HistoryRecoveryReport API name.
type HistoryRecoveryReport struct {
	Valid          bool                  `json:"valid"`
	HistoryTable   string                `json:"history_table"`
	Identifier     string                `json:"identifier,omitempty"`
	FirstHistoryID int64                 `json:"first_history_id"`
	LastHistoryID  int64                 `json:"last_history_id"`
	RowCount       int64                 `json:"row_count"`
	RecoveredRows  []RecoveredHistoryRow `json:"recovered_rows,omitempty"`
	Findings       []VerificationFinding `json:"findings,omitempty"`
}

// RecoveredHistoryRow is one history row reconstructed from WORM evidence.
type RecoveredHistoryRow struct {
	HistoryID     int64             `json:"history_id"`
	Identifier    string            `json:"identifier"`
	ChangeType    string            `json:"change_type"`
	Deleted       bool              `json:"deleted"`
	OperationTime time.Time         `json:"operation_time"`
	PayloadType   string            `json:"payload_type"`
	Payload       any               `json:"payload"`
	EffectiveDiff []map[string]any  `json:"effective_diff,omitempty"`
	Snapshot      map[string]any    `json:"snapshot"`
	ContentHash   string            `json:"content_hash"`
	PayloadHash   string            `json:"payload_hash"`
	PreviousHash  string            `json:"previous_hash"`
	RowHash       string            `json:"row_hash"`
	Audit         AuditContext      `json:"audit"`
	Reference     EvidenceReference `json:"reference"`
}

type historyEventArtifactDocument struct {
	ArtifactVersion string                    `json:"artifact_version"`
	HashContract    string                    `json:"hash_contract"`
	HistoryTable    string                    `json:"history_table"`
	HistoryID       int64                     `json:"history_id"`
	Identifier      string                    `json:"identifier"`
	ChangeType      string                    `json:"change_type"`
	Deleted         bool                      `json:"deleted"`
	ValidFrom       string                    `json:"valid_from"`
	OperationTime   string                    `json:"operation_time"`
	Administration  historyEventAdminDocument `json:"administration"`
	PayloadType     string                    `json:"payload_type"`
	Payload         json.RawMessage           `json:"payload"`
	EffectiveDiff   []map[string]any          `json:"effective_diff"`
	ContentHash     string                    `json:"content_hash"`
	PayloadHash     string                    `json:"payload_hash"`
	PreviousHash    string                    `json:"previous_hash"`
	RowHash         string                    `json:"row_hash"`
	Audit           historyEventAuditDocument `json:"audit"`
}

type historyEventAdminDocument struct {
	CreatedAtText string `json:"created_at_text"`
	UpdatedAtText string `json:"updated_at_text"`
}

type historyEventAuditDocument struct {
	RequestID           string `json:"request_id"`
	CorrelationID       string `json:"correlation_id"`
	ActorSubject        string `json:"actor_subject"`
	ActorIssuer         string `json:"actor_issuer"`
	ClientID            string `json:"client_id"`
	AuthorizationResult string `json:"authorization_result"`
	PolicyID            string `json:"policy_id"`
	MatchedRuleID       string `json:"matched_rule_id"`
	SourceIP            string `json:"source_ip"`
	UserAgent           string `json:"user_agent"`
	Operation           string `json:"operation"`
	Endpoint            string `json:"endpoint"`
	HTTPMethod          string `json:"http_method"`
}

// LoadEvidenceRecoveryCatalog loads WORM history_event receipts needed to recover a range.
//
// The catalog is built from PostgreSQL metadata and can be exported as JSON for
// offline recovery. For diff-backed rows, the catalog includes the nearest WORM
// snapshot checkpoint and intervening rows needed to reconstruct the selected
// range.
//
// Parameters:
//   - ctx: Request context for PostgreSQL reads.
//   - db: Database handle connected to the BaSyx PostgreSQL database.
//   - options: History range and optional identifier scope.
//
// Returns:
//   - EvidenceRecoveryCatalog: Receipt catalog for recovery/export.
//   - error: Error when the range is invalid, PostgreSQL rows cannot be read,
//     or the history table is unsupported.
func LoadEvidenceRecoveryCatalog(ctx context.Context, db *sql.DB, options EvidenceRecoveryCatalogOptions) (EvidenceRecoveryCatalog, error) {
	if db == nil {
		return EvidenceRecoveryCatalog{}, common.NewErrBadRequest("HISTORY-RECOVERY-CATALOG-NILDB database handle must not be nil")
	}
	if err := validateVerifyHistoryRangeOptions(VerifyHistoryRangeOptions{
		HistoryTable:   options.HistoryTable,
		Identifier:     options.Identifier,
		FirstHistoryID: options.FirstHistoryID,
		LastHistoryID:  options.LastHistoryID,
	}); err != nil {
		return EvidenceRecoveryCatalog{}, err
	}
	rows, err := loadManifestRangeDBRows(ctx, db, options.HistoryTable, options.Identifier, options.FirstHistoryID, options.LastHistoryID)
	if err != nil {
		return EvidenceRecoveryCatalog{}, err
	}
	catalog := EvidenceRecoveryCatalog{
		HistoryTable:   options.HistoryTable,
		Identifier:     strings.TrimSpace(options.Identifier),
		FirstHistoryID: options.FirstHistoryID,
		LastHistoryID:  options.LastHistoryID,
	}
	for identifier, historyRange := range identifierRanges(rows) {
		checkpointID, checkpointErr := nearestSnapshotHistoryID(ctx, db, options.HistoryTable, identifier, historyRange.firstID)
		if checkpointErr != nil {
			return EvidenceRecoveryCatalog{}, checkpointErr
		}
		receipts, receiptErr := loadEventArtifactReceiptRows(ctx, db, options.HistoryTable, identifier, checkpointID, historyRange.lastID)
		if receiptErr != nil {
			return EvidenceRecoveryCatalog{}, receiptErr
		}
		catalog.EventArtifacts = append(catalog.EventArtifacts, recoveryArtifactsFromReceipts(receipts)...)
	}
	sortRecoveryArtifacts(catalog.EventArtifacts)
	return catalog, nil
}

// RecoverHistoryFromEvidence verifies and reconstructs history rows from WORM evidence.
//
// The function never writes to PostgreSQL. It fetches the cataloged
// history_event artifacts, validates object hashes, verifies row and payload
// hashes, replays diffs from WORM snapshots, and returns a JSON-serializable
// export for rows inside the catalog range.
//
// Parameters:
//   - ctx: Request context for evidence-store reads.
//   - store: Evidence store used to fetch immutable history_event artifacts.
//   - catalog: Recovery catalog loaded from PostgreSQL or an exported JSON file.
//
// Returns:
//   - *HistoryRecoveryReport: Recovered rows and verification findings.
//   - error: Error when inputs are structurally invalid.
func RecoverHistoryFromEvidence(ctx context.Context, store EvidenceStore, catalog EvidenceRecoveryCatalog) (*HistoryRecoveryReport, error) {
	if store == nil {
		return nil, common.NewErrBadRequest("HISTORY-RECOVERY-NILSTORE evidence store must not be nil")
	}
	if err := validateVerifyHistoryRangeOptions(VerifyHistoryRangeOptions{
		HistoryTable:   catalog.HistoryTable,
		Identifier:     catalog.Identifier,
		FirstHistoryID: catalog.FirstHistoryID,
		LastHistoryID:  catalog.LastHistoryID,
	}); err != nil {
		return nil, err
	}
	report := &HistoryRecoveryReport{
		Valid:          true,
		HistoryTable:   catalog.HistoryTable,
		Identifier:     strings.TrimSpace(catalog.Identifier),
		FirstHistoryID: catalog.FirstHistoryID,
		LastHistoryID:  catalog.LastHistoryID,
	}
	rows, references := loadRecoveryRows(ctx, store, catalog, report)
	for identifier, identifierRows := range groupStoredRowsByIdentifier(rows) {
		if err := appendRecoveredRows(report, identifier, identifierRows, references); err != nil {
			report.addFinding(VerificationSeverityError, "HISTORY-RECOVERY-REPLAY", err.Error(), identifier, 0)
		}
	}
	sortRecoveredRows(report.RecoveredRows)
	report.RowCount = int64(len(report.RecoveredRows))
	report.Valid = len(report.Findings) == 0
	return report, nil
}

func recoveryArtifactsFromReceipts(receipts []eventArtifactReceiptRow) []EvidenceRecoveryArtifact {
	artifacts := make([]EvidenceRecoveryArtifact, 0, len(receipts))
	for _, receipt := range receipts {
		artifacts = append(artifacts, EvidenceRecoveryArtifact{
			Identifier:  receipt.Identifier,
			HistoryID:   receipt.HistoryID,
			RowHash:     receipt.RowHash,
			ContentHash: receipt.ContentHash,
			Receipt:     receipt.Receipt,
		})
	}
	return artifacts
}

func loadRecoveryRows(ctx context.Context, store EvidenceStore, catalog EvidenceRecoveryCatalog, report *HistoryRecoveryReport) ([]storedHistoryRow, map[string]EvidenceReference) {
	artifacts := append([]EvidenceRecoveryArtifact(nil), catalog.EventArtifacts...)
	sortRecoveryArtifacts(artifacts)
	rows := make([]storedHistoryRow, 0, len(artifacts))
	references := make(map[string]EvidenceReference, len(artifacts))
	seen := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		key := eventArtifactKey(artifact.Identifier, artifact.HistoryID, artifact.RowHash)
		if _, exists := seen[key]; exists {
			report.addFinding(VerificationSeverityError, "HISTORY-RECOVERY-DUPLICATE", "duplicate history_event artifact receipt in recovery catalog", artifact.Identifier, artifact.HistoryID)
			continue
		}
		seen[key] = struct{}{}
		row, err := loadRecoveryRow(ctx, store, catalog.HistoryTable, artifact)
		if err != nil {
			report.addFinding(VerificationSeverityError, "HISTORY-RECOVERY-ARTIFACT", err.Error(), artifact.Identifier, artifact.HistoryID)
			continue
		}
		rows = append(rows, row)
		references[key] = artifact.Receipt.Reference
	}
	return rows, references
}

func loadRecoveryRow(ctx context.Context, store EvidenceStore, table string, artifact EvidenceRecoveryArtifact) (storedHistoryRow, error) {
	if strings.TrimSpace(artifact.Receipt.SHA256) == "" {
		return storedHistoryRow{}, fmt.Errorf("HISTORY-RECOVERY-MISSINGHASH receipt SHA-256 is missing")
	}
	object, err := store.GetArtifact(ctx, artifact.Receipt.Reference)
	if err != nil {
		return storedHistoryRow{}, fmt.Errorf("HISTORY-RECOVERY-GETARTIFACT %w", err)
	}
	if object == nil {
		return storedHistoryRow{}, fmt.Errorf("HISTORY-RECOVERY-NILARTIFACT evidence store returned nil artifact")
	}
	if actual := SHA256Hex(object.Data); !strings.EqualFold(actual, artifact.Receipt.SHA256) {
		return storedHistoryRow{}, fmt.Errorf("HISTORY-RECOVERY-ARTIFACTHASH history_event object SHA-256 does not match receipt")
	}
	doc, err := decodeHistoryEventArtifactDocument(object.Data)
	if err != nil {
		return storedHistoryRow{}, err
	}
	if err = validateRecoveryArtifactDocument(table, artifact, doc); err != nil {
		return storedHistoryRow{}, err
	}
	return recoveryStoredRowFromDocument(doc)
}

func decodeHistoryEventArtifactDocument(data []byte) (historyEventArtifactDocument, error) {
	var doc historyEventArtifactDocument
	if err := decodeJSONPreservingNumbers(data, &doc); err != nil {
		return historyEventArtifactDocument{}, fmt.Errorf("HISTORY-RECOVERY-DECODE %w", err)
	}
	return doc, nil
}

func validateRecoveryArtifactDocument(table string, artifact EvidenceRecoveryArtifact, doc historyEventArtifactDocument) error {
	if doc.ArtifactVersion != historyEventArtifactVersion {
		return fmt.Errorf("HISTORY-RECOVERY-VERSION unsupported history_event artifact version %q", doc.ArtifactVersion)
	}
	if doc.HashContract != historyRowHashContract {
		return fmt.Errorf("HISTORY-RECOVERY-HASHCONTRACT unsupported history_event hash contract %q", doc.HashContract)
	}
	if doc.HistoryTable != table {
		return fmt.Errorf("HISTORY-RECOVERY-TABLE artifact history table does not match catalog")
	}
	if doc.HistoryID != artifact.HistoryID {
		return fmt.Errorf("HISTORY-RECOVERY-HISTORYID artifact history_id does not match catalog")
	}
	if doc.Identifier != artifact.Identifier {
		return fmt.Errorf("HISTORY-RECOVERY-IDENTIFIER artifact identifier does not match catalog")
	}
	if strings.TrimSpace(artifact.RowHash) != "" && doc.RowHash != artifact.RowHash {
		return fmt.Errorf("HISTORY-RECOVERY-ROWHASH artifact row_hash does not match catalog")
	}
	if strings.TrimSpace(artifact.ContentHash) != "" && doc.ContentHash != artifact.ContentHash {
		return fmt.Errorf("HISTORY-RECOVERY-CONTENTHASH artifact content_hash does not match catalog")
	}
	if doc.PayloadType != PayloadTypeSnapshot && doc.PayloadType != PayloadTypeDiff {
		return fmt.Errorf("HISTORY-RECOVERY-PAYLOADTYPE unsupported payload type %q", doc.PayloadType)
	}
	if len(doc.Payload) == 0 {
		return fmt.Errorf("HISTORY-RECOVERY-PAYLOAD payload is missing")
	}
	if strings.TrimSpace(doc.RowHash) == "" {
		return fmt.Errorf("HISTORY-RECOVERY-EMPTYROWHASH row_hash is missing")
	}
	return nil
}

func recoveryStoredRowFromDocument(doc historyEventArtifactDocument) (storedHistoryRow, error) {
	operationTime, err := parseHistoryEventArtifactTime(doc.OperationTime, doc.ValidFrom)
	if err != nil {
		return storedHistoryRow{}, err
	}
	row := storedHistoryRow{
		EntityType:          doc.HistoryTable,
		HistoryID:           doc.HistoryID,
		Identifier:          doc.Identifier,
		ChangeType:          doc.ChangeType,
		PayloadType:         doc.PayloadType,
		Deleted:             doc.Deleted,
		CreatedAt:           sqlString(doc.Administration.CreatedAtText),
		UpdatedAt:           sqlString(doc.Administration.UpdatedAtText),
		OperationAt:         operationTime,
		ContentHash:         sqlString(doc.ContentHash),
		PayloadHash:         sqlString(doc.PayloadHash),
		PreviousHash:        sqlString(doc.PreviousHash),
		RowHash:             sqlString(doc.RowHash),
		RequestID:           sqlString(doc.Audit.RequestID),
		CorrelationID:       sqlString(doc.Audit.CorrelationID),
		ActorSubject:        sqlString(doc.Audit.ActorSubject),
		ActorIssuer:         sqlString(doc.Audit.ActorIssuer),
		ClientID:            sqlString(doc.Audit.ClientID),
		AuthorizationResult: sqlString(doc.Audit.AuthorizationResult),
		PolicyID:            sqlString(doc.Audit.PolicyID),
		MatchedRuleID:       sqlString(doc.Audit.MatchedRuleID),
		SourceIP:            sqlString(doc.Audit.SourceIP),
		UserAgent:           sqlString(doc.Audit.UserAgent),
		Operation:           sqlString(doc.Audit.Operation),
		Endpoint:            sqlString(doc.Audit.Endpoint),
		HTTPMethod:          sqlString(doc.Audit.HTTPMethod),
	}
	if doc.PayloadType == PayloadTypeSnapshot {
		row.Snapshot = sqlString(string(doc.Payload))
	} else {
		row.Diff = sqlString(string(doc.Payload))
	}
	return row, nil
}

func parseHistoryEventArtifactTime(operationTime string, validFrom string) (time.Time, error) {
	raw := strings.TrimSpace(operationTime)
	if raw == "" {
		raw = strings.TrimSpace(validFrom)
	}
	if raw == "" {
		return time.Time{}, fmt.Errorf("HISTORY-RECOVERY-TIME operation_time is missing")
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("HISTORY-RECOVERY-TIME %w", err)
	}
	return parsed.UTC(), nil
}

func appendRecoveredRows(report *HistoryRecoveryReport, identifier string, rows []storedHistoryRow, references map[string]EvidenceReference) error {
	sortStoredRows(rows)
	versions, err := restoreVersionChainRows(rows)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.HistoryID < report.FirstHistoryID || row.HistoryID > report.LastHistoryID {
			continue
		}
		version, exists := versions[row.HistoryID]
		if !exists {
			return fmt.Errorf("HISTORY-RECOVERY-MISSINGVERSION recovered chain does not contain history_id %d", row.HistoryID)
		}
		recovered, rowErr := recoveredHistoryRow(row, version.snapshot, references[eventArtifactKey(identifier, row.HistoryID, nullStringValue(row.RowHash))])
		if rowErr != nil {
			return rowErr
		}
		report.RecoveredRows = append(report.RecoveredRows, recovered)
	}
	return nil
}

func recoveredHistoryRow(row storedHistoryRow, snapshot map[string]any, ref EvidenceReference) (RecoveredHistoryRow, error) {
	payload, err := recoveryPayloadValue(row)
	if err != nil {
		return RecoveredHistoryRow{}, err
	}
	effectiveDiff, err := recoveryEffectiveDiff(row)
	if err != nil {
		return RecoveredHistoryRow{}, err
	}
	return RecoveredHistoryRow{
		HistoryID:     row.HistoryID,
		Identifier:    row.Identifier,
		ChangeType:    row.ChangeType,
		Deleted:       row.Deleted,
		OperationTime: row.OperationAt,
		PayloadType:   row.PayloadType,
		Payload:       payload,
		EffectiveDiff: effectiveDiff,
		Snapshot:      snapshot,
		ContentHash:   nullStringValue(row.ContentHash),
		PayloadHash:   nullStringValue(row.PayloadHash),
		PreviousHash:  nullStringValue(row.PreviousHash),
		RowHash:       nullStringValue(row.RowHash),
		Audit: AuditContext{
			ActorSubject:        nullStringValue(row.ActorSubject),
			ActorIssuer:         nullStringValue(row.ActorIssuer),
			ClientID:            nullStringValue(row.ClientID),
			AuthorizationResult: nullStringValue(row.AuthorizationResult),
			PolicyID:            nullStringValue(row.PolicyID),
			MatchedRuleID:       nullStringValue(row.MatchedRuleID),
			RequestID:           nullStringValue(row.RequestID),
			CorrelationID:       nullStringValue(row.CorrelationID),
			SourceIP:            nullStringValue(row.SourceIP),
			UserAgent:           nullStringValue(row.UserAgent),
			Operation:           nullStringValue(row.Operation),
			Endpoint:            nullStringValue(row.Endpoint),
			HTTPMethod:          nullStringValue(row.HTTPMethod),
		},
		Reference: ref,
	}, nil
}

func recoveryPayloadValue(row storedHistoryRow) (any, error) {
	raw := row.Snapshot
	if row.PayloadType == PayloadTypeDiff {
		raw = row.Diff
	}
	if !raw.Valid {
		return nil, fmt.Errorf("HISTORY-RECOVERY-PAYLOADVALUE payload is missing")
	}
	var payload any
	if err := decodeJSONPreservingNumbers([]byte(raw.String), &payload); err != nil {
		return nil, fmt.Errorf("HISTORY-RECOVERY-PAYLOADVALUE %w", err)
	}
	return payload, nil
}

func recoveryEffectiveDiff(row storedHistoryRow) ([]map[string]any, error) {
	if row.PayloadType == PayloadTypeDiff {
		var patch []map[string]any
		if err := decodeJSONPreservingNumbers([]byte(row.Diff.String), &patch); err != nil {
			return nil, fmt.Errorf("HISTORY-RECOVERY-EFFECTIVEDIFF %w", err)
		}
		return patch, nil
	}
	return nil, nil
}

func groupStoredRowsByIdentifier(rows []storedHistoryRow) map[string][]storedHistoryRow {
	grouped := make(map[string][]storedHistoryRow)
	for _, row := range rows {
		grouped[row.Identifier] = append(grouped[row.Identifier], row)
	}
	return grouped
}

func sortStoredRows(rows []storedHistoryRow) {
	sort.Slice(rows, func(left int, right int) bool {
		return rows[left].HistoryID < rows[right].HistoryID
	})
}

func sortRecoveryArtifacts(artifacts []EvidenceRecoveryArtifact) {
	sort.Slice(artifacts, func(left int, right int) bool {
		if artifacts[left].Identifier != artifacts[right].Identifier {
			return artifacts[left].Identifier < artifacts[right].Identifier
		}
		if artifacts[left].HistoryID != artifacts[right].HistoryID {
			return artifacts[left].HistoryID < artifacts[right].HistoryID
		}
		return artifacts[left].RowHash < artifacts[right].RowHash
	})
}

func sortRecoveredRows(rows []RecoveredHistoryRow) {
	sort.Slice(rows, func(left int, right int) bool {
		if rows[left].HistoryID != rows[right].HistoryID {
			return rows[left].HistoryID < rows[right].HistoryID
		}
		return rows[left].Identifier < rows[right].Identifier
	})
}

func sqlString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: strings.TrimSpace(value) != ""}
}

func (report *HistoryRecoveryReport) addFinding(severity string, code string, message string, identifier string, historyID int64) {
	report.Valid = false
	report.Findings = append(report.Findings, VerificationFinding{
		Severity:   severity,
		Code:       code,
		Message:    message,
		Identifier: identifier,
		HistoryID:  historyID,
	})
}
