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

// Package evidence provides provider-neutral immutable evidence artifacts and
// WORM-compatible object stores for history, audit, and policy consumers.
package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"
)

// Provider and artifact constants identify shared evidence formats.
const (
	ProviderNone = "none"
	ProviderS3   = "s3"

	ArtifactManifest     = "manifest"
	ArtifactSnapshot     = "snapshot"
	ArtifactHistoryEvent = "history_event"
	ArtifactABACPolicy   = "abac_policy_version"
	ArtifactBinary       = "binary_content"
	ArtifactBinaryRef    = "binary_reference"
)

// Store writes and verifies immutable evidence objects.
type Store interface {
	PutArtifact(ctx context.Context, artifact Artifact) (*Receipt, error)
	GetArtifact(ctx context.Context, ref Reference) (*Object, error)
	VerifyArtifact(ctx context.Context, ref Reference, expectedHash string) (*Receipt, error)
}

// StreamStore supports bounded-memory evidence uploads.
type StreamStore interface {
	PutArtifactReader(ctx context.Context, artifact Artifact, reader io.Reader, sizeBytes int64, digest string) (*Receipt, error)
}

// RetentionExtender extends protection of an existing retained version.
type RetentionExtender interface {
	ExtendArtifactRetention(ctx context.Context, ref Reference, current Receipt, artifact Artifact) (*Receipt, error)
}

// RetentionVerifier verifies server-enforced retention for an object version.
type RetentionVerifier interface {
	VerifyArtifactRetention(ctx context.Context, ref Reference, expected Receipt) error
}

// Artifact contains bytes and requested retention metadata.
type Artifact struct {
	ArtifactType  string
	ObjectKey     string
	ContentType   string
	Data          []byte
	Metadata      map[string]string
	RetentionMode string
	RetainUntil   time.Time
	LegalHold     bool
}

// Reference identifies a specific immutable object version.
type Reference struct {
	Provider  string `json:"provider"`
	Bucket    string `json:"bucket,omitempty"`
	ObjectKey string `json:"object_key"`
	VersionID string `json:"version_id,omitempty"`
}

// Receipt records verified storage metadata for an artifact.
type Receipt struct {
	Reference     Reference         `json:"reference"`
	SHA256        string            `json:"sha256"`
	SizeBytes     int64             `json:"size_bytes"`
	ContentType   string            `json:"content_type"`
	RetentionMode string            `json:"retention_mode,omitempty"`
	RetainUntil   *time.Time        `json:"retain_until,omitempty"`
	LegalHold     bool              `json:"legal_hold"`
	StoredAt      time.Time         `json:"stored_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Object contains downloaded evidence bytes and metadata.
type Object struct {
	Reference   Reference
	Data        []byte
	ContentType string
	Metadata    map[string]string
}

// SHA256Hex returns the lowercase SHA-256 digest of evidence bytes.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
