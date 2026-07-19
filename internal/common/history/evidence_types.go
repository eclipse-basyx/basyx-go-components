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
	"io"
	"time"
)

// Evidence and integrity constants define supported providers, artifact types, and signature states.
const (
	EvidenceProviderNone = "none"
	EvidenceProviderS3   = "s3"

	IntegrityAnchorProviderNone = "none"

	EvidenceArtifactManifest     = "manifest"
	EvidenceArtifactSnapshot     = "snapshot"
	EvidenceArtifactHistoryEvent = "history_event"
	EvidenceArtifactABACPolicy   = "abac_policy_version"
	EvidenceArtifactBinary       = "binary_content"
	EvidenceArtifactBinaryRef    = "binary_reference"

	SignatureStateUnsigned = "unsigned"
	SignatureStateSigned   = "signed"

	HistoryManifestVersion = "basyx-history-manifest-v1"
	historyRangeContract   = "basyx-history-range-v1"
)

// EvidenceStore writes immutable evidence artifacts and can later re-read them for verification.
type EvidenceStore interface {
	PutArtifact(ctx context.Context, artifact EvidenceArtifact) (*EvidenceReceipt, error)
	GetArtifact(ctx context.Context, ref EvidenceReference) (*EvidenceObject, error)
	VerifyArtifact(ctx context.Context, ref EvidenceReference, expectedHash string) (*EvidenceReceipt, error)
}

// EvidenceStreamStore writes large immutable objects without materializing
// their complete payload in process memory.
type EvidenceStreamStore interface {
	PutArtifactReader(ctx context.Context, artifact EvidenceArtifact, reader io.Reader, sizeBytes int64, sha256 string) (*EvidenceReceipt, error)
}

// EvidenceRetentionExtender lengthens retention on a reused immutable object.
type EvidenceRetentionExtender interface {
	ExtendArtifactRetention(ctx context.Context, ref EvidenceReference, current EvidenceReceipt, artifact EvidenceArtifact) (*EvidenceReceipt, error)
}

// EvidenceRetentionVerifier verifies WORM retention state from the evidence backend.
//
// Implementations use provider-specific APIs to compare the stored object version
// against the PostgreSQL receipt. The S3 implementation checks Object Lock
// retention and legal hold state for the referenced version.
type EvidenceRetentionVerifier interface {
	VerifyArtifactRetention(ctx context.Context, ref EvidenceReference, expected EvidenceReceipt) error
}

// EvidenceArtifact is a byte artifact destined for WORM-compatible object storage.
type EvidenceArtifact struct {
	ArtifactType  string
	ObjectKey     string
	ContentType   string
	Data          []byte
	Metadata      map[string]string
	RetentionMode string
	RetainUntil   time.Time
	LegalHold     bool
}

// EvidenceReference identifies a stored evidence object.
type EvidenceReference struct {
	Provider  string `json:"provider"`
	Bucket    string `json:"bucket,omitempty"`
	ObjectKey string `json:"object_key"`
	VersionID string `json:"version_id,omitempty"`
}

// EvidenceReceipt records immutable object-store metadata returned after a write or verification.
type EvidenceReceipt struct {
	Reference     EvidenceReference `json:"reference"`
	SHA256        string            `json:"sha256"`
	SizeBytes     int64             `json:"size_bytes"`
	ContentType   string            `json:"content_type"`
	RetentionMode string            `json:"retention_mode,omitempty"`
	RetainUntil   *time.Time        `json:"retain_until,omitempty"`
	LegalHold     bool              `json:"legal_hold"`
	StoredAt      time.Time         `json:"stored_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// EvidenceObject is the downloaded representation of a stored evidence artifact.
type EvidenceObject struct {
	Reference   EvidenceReference
	Data        []byte
	ContentType string
	Metadata    map[string]string
}

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
