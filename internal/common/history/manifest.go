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
	"fmt"
	"strings"
	"time"
)

// HistoryManifestOptions contains the ordered row range used to build a manifest.
//
//revive:disable-next-line:exported
type HistoryManifestOptions struct {
	HistoryTable       string
	Identifier         string
	Rows               []ManifestRangeRow
	SnapshotReferences []SnapshotArtifactReference
	GeneratedAt        time.Time
	SignatureState     string
	Signer             *ManifestSignerInfo
}

// BuildHistoryManifest creates a canonical manifest for an ordered hash-chain range.
//
// Parameters:
//   - options: History table, ordered row hashes, optional snapshot references,
//     generation timestamp, and signer metadata.
//
// Returns:
//   - HistoryManifest: Deterministic range manifest ready for signing or storage.
//   - error: Error when the range is empty, unordered, or missing required hashes.
func BuildHistoryManifest(options HistoryManifestOptions) (HistoryManifest, error) {
	rows, err := validateManifestRows(options.Rows)
	if err != nil {
		return HistoryManifest{}, err
	}
	historyTable := strings.TrimSpace(options.HistoryTable)
	if historyTable == "" {
		return HistoryManifest{}, fmt.Errorf("HISTORY-MANIFEST-EMPTYTABLE history table is required")
	}
	rangeDigest, err := ComputeRangeDigest(rows)
	if err != nil {
		return HistoryManifest{}, err
	}
	generatedAt := options.GeneratedAt.UTC()
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	signatureState := normalizeSignatureState(options.SignatureState)
	return HistoryManifest{
		ManifestVersion:    HistoryManifestVersion,
		HistoryTable:       historyTable,
		Identifier:         strings.TrimSpace(options.Identifier),
		FirstHistoryID:     rows[0].HistoryID,
		LastHistoryID:      rows[len(rows)-1].HistoryID,
		FirstRowHash:       rows[0].RowHash,
		LastRowHash:        rows[len(rows)-1].RowHash,
		RowCount:           int64(len(rows)),
		RangeDigest:        rangeDigest,
		GeneratedAt:        generatedAt,
		SignatureState:     signatureState,
		Signer:             options.Signer,
		SnapshotReferences: cloneSnapshotReferences(options.SnapshotReferences),
	}, nil
}

// ComputeRangeDigest hashes an ordered list of row hashes and their sequence metadata.
//
// Parameters:
//   - rows: Strictly increasing history_id and row_hash pairs.
//
// Returns:
//   - string: Canonical SHA-256 range digest.
//   - error: Error when rows are empty, unordered, or contain empty row hashes.
func ComputeRangeDigest(rows []ManifestRangeRow) (string, error) {
	orderedRows, err := validateManifestRows(rows)
	if err != nil {
		return "", err
	}
	return CanonicalJSONHash(map[string]any{
		"hashContract": historyRangeContract,
		"rows":         orderedRows,
	})
}

// CanonicalManifestJSON returns the canonical JSON artifact bytes for a manifest.
//
// Parameters:
//   - manifest: Valid history manifest.
//
// Returns:
//   - []byte: Canonical JSON representation used for hashing and signing.
//   - error: Error when manifest validation or canonical serialization fails.
func CanonicalManifestJSON(manifest HistoryManifest) ([]byte, error) {
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	return CanonicalJSON(manifest)
}

func validateManifest(manifest HistoryManifest) error {
	if strings.TrimSpace(manifest.ManifestVersion) != HistoryManifestVersion {
		return fmt.Errorf("HISTORY-MANIFEST-VERSION unsupported manifest version %q", manifest.ManifestVersion)
	}
	if strings.TrimSpace(manifest.HistoryTable) == "" {
		return fmt.Errorf("HISTORY-MANIFEST-EMPTYTABLE history table is required")
	}
	if manifest.RowCount < 1 {
		return fmt.Errorf("HISTORY-MANIFEST-ROWCOUNT row count must be positive")
	}
	if manifest.FirstHistoryID < 1 || manifest.LastHistoryID < manifest.FirstHistoryID {
		return fmt.Errorf("HISTORY-MANIFEST-RANGE invalid history_id range")
	}
	if strings.TrimSpace(manifest.FirstRowHash) == "" || strings.TrimSpace(manifest.LastRowHash) == "" {
		return fmt.Errorf("HISTORY-MANIFEST-ROWHASH first and last row hashes are required")
	}
	if strings.TrimSpace(manifest.RangeDigest) == "" {
		return fmt.Errorf("HISTORY-MANIFEST-RANGEDIGEST range digest is required")
	}
	if normalizeSignatureState(manifest.SignatureState) != manifest.SignatureState {
		return fmt.Errorf("HISTORY-MANIFEST-SIGNATURESTATE unsupported signature state %q", manifest.SignatureState)
	}
	return nil
}

func validateManifestRows(rows []ManifestRangeRow) ([]ManifestRangeRow, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("HISTORY-MANIFEST-EMPTYRANGE at least one history row is required")
	}
	cloned := make([]ManifestRangeRow, 0, len(rows))
	previousID := int64(0)
	for _, row := range rows {
		if row.HistoryID <= previousID {
			return nil, fmt.Errorf("HISTORY-MANIFEST-ORDER history rows must be strictly increasing by history_id")
		}
		if strings.TrimSpace(row.RowHash) == "" {
			return nil, fmt.Errorf("HISTORY-MANIFEST-EMPTYROWHASH row hash is required")
		}
		cloned = append(cloned, ManifestRangeRow{HistoryID: row.HistoryID, RowHash: strings.TrimSpace(row.RowHash)})
		previousID = row.HistoryID
	}
	return cloned, nil
}

func normalizeSignatureState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case SignatureStateSigned:
		return SignatureStateSigned
	default:
		return SignatureStateUnsigned
	}
}

func cloneSnapshotReferences(refs []SnapshotArtifactReference) []SnapshotArtifactReference {
	if len(refs) == 0 {
		return nil
	}
	cloned := make([]SnapshotArtifactReference, len(refs))
	copy(cloned, refs)
	return cloned
}
