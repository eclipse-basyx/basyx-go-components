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
	"fmt"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/evidence"
)

// Evidence and integrity constants define supported providers, artifact types, and signature states.
const (
	EvidenceProviderNone = evidence.ProviderNone
	EvidenceProviderS3   = evidence.ProviderS3

	IntegrityAnchorProviderNone = "none"

	EvidenceArtifactManifest     = evidence.ArtifactManifest
	EvidenceArtifactSnapshot     = evidence.ArtifactSnapshot
	EvidenceArtifactHistoryEvent = evidence.ArtifactHistoryEvent
	EvidenceArtifactABACPolicy   = evidence.ArtifactABACPolicy
	EvidenceArtifactBinary       = evidence.ArtifactBinary
	EvidenceArtifactBinaryRef    = evidence.ArtifactBinaryRef

	SignatureStateUnsigned = "unsigned"
	SignatureStateSigned   = "signed"

	HistoryManifestVersion = "basyx-history-manifest-v1"
	historyRangeContract   = "basyx-history-range-v1"
)

// EvidenceStore retains the history API for shared evidence storage.
type EvidenceStore = evidence.Store

// EvidenceStreamStore retains the history streaming evidence API.
type EvidenceStreamStore = evidence.StreamStore

// EvidenceRetentionExtender retains the history retention extension API.
type EvidenceRetentionExtender = evidence.RetentionExtender

// EvidenceRetentionVerifier retains the history retention verification API.
type EvidenceRetentionVerifier = evidence.RetentionVerifier

// EvidenceArtifact is the shared immutable evidence artifact.
type EvidenceArtifact = evidence.Artifact

// EvidenceReference is the shared version-specific evidence reference.
type EvidenceReference = evidence.Reference

// EvidenceReceipt is the shared verified storage receipt.
type EvidenceReceipt = evidence.Receipt

// EvidenceObject is the shared downloaded evidence object.
type EvidenceObject = evidence.Object

// IntegrityAnchor is reserved for optional external ledgers or timestamping services.
type IntegrityAnchor interface {
	AnchorIntegrity(ctx context.Context, request IntegrityAnchorRequest) (*IntegrityAnchorReceipt, error)
}

// IntegrityAnchorRequest is the deterministic digest submitted to future anchor backends.
type IntegrityAnchorRequest struct {
	Provider       string
	HistoryTable   string
	Identifier     string
	RangeDigest    string
	ManifestHash   string
	ManifestRef    EvidenceReference
	GeneratedAt    time.Time
	AdditionalData map[string]string
}

// IntegrityAnchorReceipt captures future ledger/timestamping provider metadata.
type IntegrityAnchorReceipt struct {
	Provider   string
	AnchorID   string
	AnchorTime time.Time
	Proof      map[string]string
}

// NoopIntegrityAnchor is the default anchor implementation when no ledger backend is configured.
type NoopIntegrityAnchor struct{}

// BuildIntegrityAnchorRequest creates the deterministic input for optional anchor backends.
//
// The request is anchor-ready but does not contact a ledger or timestamping
// service. Providers can use the manifest range digest and object SHA-256 to
// prove that a signed WORM manifest existed for the selected history range.
//
// Parameters:
//   - manifest: Verified history range manifest.
//   - manifestReceipt: Object-store receipt for the manifest artifact.
//
// Returns:
//   - IntegrityAnchorRequest: Provider-neutral anchor request.
//   - error: Error when the manifest is invalid or the manifest receipt hash is missing.
func BuildIntegrityAnchorRequest(manifest HistoryManifest, manifestReceipt EvidenceReceipt) (IntegrityAnchorRequest, error) {
	if err := validateManifest(manifest); err != nil {
		return IntegrityAnchorRequest{}, err
	}
	if manifestReceipt.SHA256 == "" {
		return IntegrityAnchorRequest{}, fmt.Errorf("HISTORY-ANCHOR-MANIFESTHASH manifest receipt SHA-256 is required")
	}
	return IntegrityAnchorRequest{
		Provider:     IntegrityAnchorProviderNone,
		HistoryTable: manifest.HistoryTable,
		Identifier:   manifest.Identifier,
		RangeDigest:  manifest.RangeDigest,
		ManifestHash: manifestReceipt.SHA256,
		ManifestRef:  manifestReceipt.Reference,
		GeneratedAt:  manifest.GeneratedAt,
		AdditionalData: map[string]string{
			"manifest_version": manifest.ManifestVersion,
			"signature_state":  manifest.SignatureState,
		},
	}, nil
}

// AnchorIntegrity intentionally performs no external write and returns no receipt.
//
// Parameters:
//   - ctx: Unused context accepted to satisfy IntegrityAnchor.
//   - request: Unused anchor request accepted to satisfy IntegrityAnchor.
//
// Returns:
//   - *IntegrityAnchorReceipt: Always nil.
//   - error: Always nil.
func (NoopIntegrityAnchor) AnchorIntegrity(_ context.Context, _ IntegrityAnchorRequest) (*IntegrityAnchorReceipt, error) {
	return nil, nil
}

// HistoryManifest covers a deterministic, ordered range of history rows.
//
//revive:disable-next-line:exported
type HistoryManifest struct {
	ManifestVersion    string                      `json:"manifest_version"`
	HistoryTable       string                      `json:"history_table"`
	Identifier         string                      `json:"identifier,omitempty"`
	FirstHistoryID     int64                       `json:"first_history_id"`
	LastHistoryID      int64                       `json:"last_history_id"`
	FirstRowHash       string                      `json:"first_row_hash"`
	LastRowHash        string                      `json:"last_row_hash"`
	RowCount           int64                       `json:"row_count"`
	RangeDigest        string                      `json:"range_digest"`
	GeneratedAt        time.Time                   `json:"generated_at"`
	SignatureState     string                      `json:"signature_state"`
	Signer             *ManifestSignerInfo         `json:"signer,omitempty"`
	SnapshotReferences []SnapshotArtifactReference `json:"snapshot_references,omitempty"`
}

// ManifestSignerInfo identifies the key material used for a signed manifest artifact.
type ManifestSignerInfo struct {
	KeyID     string `json:"key_id,omitempty"`
	Algorithm string `json:"algorithm"`
}

// ManifestRangeRow is the ordered hash-chain row input for a manifest range digest.
type ManifestRangeRow struct {
	HistoryID int64  `json:"history_id"`
	RowHash   string `json:"row_hash"`
}

// SnapshotArtifactReference links a recovery checkpoint snapshot artifact to a manifest.
type SnapshotArtifactReference struct {
	HistoryID   int64             `json:"history_id"`
	RowHash     string            `json:"row_hash"`
	ContentHash string            `json:"content_hash,omitempty"`
	SHA256      string            `json:"sha256"`
	Reference   EvidenceReference `json:"reference"`
}
