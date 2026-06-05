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
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"

	commonjws "github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	jose "gopkg.in/go-jose/go-jose.v2"
)

const (
	manifestJSONContentType = "application/json"
	manifestJWSContentType  = "application/jose"
	manifestSigningAlgRS256 = "RS256"
)

// ManifestJWSSigner signs canonical manifests with a configured RSA key.
type ManifestJWSSigner struct {
	PrivateKey *rsa.PrivateKey
	KeyID      string
}

// NewManifestJWSSignerFromKeyFile loads a JWS signer for evidence manifests.
//
// Parameters:
//   - privateKeyPath: Path to a PEM encoded RSA private key.
//   - keyID: Optional key identifier to embed in manifest signer metadata.
//
// Returns:
//   - *ManifestJWSSigner: Signer configured for RS256 compact JWS manifests.
//   - error: Error when the key path is empty or the key cannot be loaded.
func NewManifestJWSSignerFromKeyFile(privateKeyPath string, keyID string) (*ManifestJWSSigner, error) {
	privateKeyPath = strings.TrimSpace(privateKeyPath)
	if privateKeyPath == "" {
		return nil, fmt.Errorf("HISTORY-MANIFESTSIGNER-EMPTYPATH private key path is required")
	}
	privateKey, err := commonjws.LoadPrivateKey(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("HISTORY-MANIFESTSIGNER-LOADKEY %w", err)
	}
	return &ManifestJWSSigner{PrivateKey: privateKey, KeyID: strings.TrimSpace(keyID)}, nil
}

// BuildManifestEvidenceArtifact returns the signed or unsigned manifest object-store artifact.
//
// When signer is nil, the artifact contains unsigned canonical JSON and the
// returned manifest is marked unsigned. When signer is present, the artifact
// contains compact JWS over canonical manifest JSON and the returned manifest
// includes signer metadata.
//
// Parameters:
//   - manifest: Manifest range metadata to serialize.
//   - objectPrefix: Optional object key prefix for the evidence store.
//   - signer: Optional JWS signer.
//
// Returns:
//   - EvidenceArtifact: Manifest artifact ready for WORM storage.
//   - HistoryManifest: Manifest with final signature state and signer metadata.
//   - error: Error when canonical serialization or signing fails.
func BuildManifestEvidenceArtifact(manifest HistoryManifest, objectPrefix string, signer *ManifestJWSSigner) (EvidenceArtifact, HistoryManifest, error) {
	if signer == nil {
		return buildUnsignedManifestArtifact(manifest, objectPrefix)
	}
	return buildSignedManifestArtifact(manifest, objectPrefix, signer)
}

func buildUnsignedManifestArtifact(manifest HistoryManifest, objectPrefix string) (EvidenceArtifact, HistoryManifest, error) {
	manifest.SignatureState = SignatureStateUnsigned
	manifest.Signer = nil
	data, err := CanonicalManifestJSON(manifest)
	if err != nil {
		return EvidenceArtifact{}, HistoryManifest{}, err
	}
	return EvidenceArtifact{
		ArtifactType: EvidenceArtifactManifest,
		ObjectKey:    manifestObjectKey(objectPrefix, manifest, ".json"),
		ContentType:  manifestJSONContentType,
		Data:         data,
		Metadata:     manifestMetadata(manifest),
	}, manifest, nil
}

func buildSignedManifestArtifact(manifest HistoryManifest, objectPrefix string, signer *ManifestJWSSigner) (EvidenceArtifact, HistoryManifest, error) {
	if signer.PrivateKey == nil {
		return EvidenceArtifact{}, HistoryManifest{}, fmt.Errorf("HISTORY-MANIFESTSIGNER-NILKEY private key must not be nil")
	}
	manifest.SignatureState = SignatureStateSigned
	manifest.Signer = &ManifestSignerInfo{KeyID: signer.KeyID, Algorithm: manifestSigningAlgRS256}
	payload, err := CanonicalManifestJSON(manifest)
	if err != nil {
		return EvidenceArtifact{}, HistoryManifest{}, err
	}
	signed, err := commonjws.SignPayload(signer.PrivateKey, payload)
	if err != nil {
		return EvidenceArtifact{}, HistoryManifest{}, fmt.Errorf("HISTORY-MANIFESTSIGNER-SIGN %w", err)
	}
	return EvidenceArtifact{
		ArtifactType: EvidenceArtifactManifest,
		ObjectKey:    manifestObjectKey(objectPrefix, manifest, ".jws"),
		ContentType:  manifestJWSContentType,
		Data:         []byte(signed),
		Metadata:     manifestMetadata(manifest),
	}, manifest, nil
}

func manifestObjectKey(objectPrefix string, manifest HistoryManifest, extension string) string {
	prefix := strings.Trim(strings.TrimSpace(objectPrefix), "/")
	identifier := strings.TrimSpace(manifest.Identifier)
	if identifier == "" {
		identifier = "_all"
	}
	key := path.Join(
		"history-manifests",
		url.PathEscape(manifest.HistoryTable),
		url.PathEscape(identifier),
		fmt.Sprintf("%d-%d-%s%s", manifest.FirstHistoryID, manifest.LastHistoryID, manifest.RangeDigest, extension),
	)
	if prefix == "" {
		return key
	}
	return path.Join(prefix, key)
}

func manifestMetadata(manifest HistoryManifest) map[string]string {
	return map[string]string{
		"artifact_type":    EvidenceArtifactManifest,
		"history_table":    manifest.HistoryTable,
		"identifier":       manifest.Identifier,
		"first_history_id": fmt.Sprintf("%d", manifest.FirstHistoryID),
		"last_history_id":  fmt.Sprintf("%d", manifest.LastHistoryID),
		"range_digest":     manifest.RangeDigest,
		"signature_state":  manifest.SignatureState,
	}
}

// DecodeManifestArtifact extracts a manifest from unsigned JSON or compact JWS evidence bytes.
//
// JWS payloads are decoded without cryptographic signature verification. Callers
// that require signer trust validation must verify the JWS signature with their
// configured public key before accepting the manifest as signed evidence.
//
// Parameters:
//   - data: Stored manifest artifact bytes.
//   - contentType: Artifact content type returned by the evidence store.
//
// Returns:
//   - HistoryManifest: Decoded and structurally validated manifest.
//   - bool: True when the artifact was encoded as compact JWS.
//   - error: Error when the artifact cannot be decoded or manifest validation fails.
func DecodeManifestArtifact(data []byte, contentType string) (HistoryManifest, bool, error) {
	payload := data
	signed := isJWSManifestArtifact(data, contentType)
	if signed {
		jws, err := jose.ParseSigned(strings.TrimSpace(string(data)))
		if err != nil {
			return HistoryManifest{}, false, fmt.Errorf("HISTORY-MANIFEST-DECODE-PARSEJWS %w", err)
		}
		payload = jws.UnsafePayloadWithoutVerification()
	}
	var manifest HistoryManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return HistoryManifest{}, signed, fmt.Errorf("HISTORY-MANIFEST-DECODE-JSON %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return HistoryManifest{}, signed, err
	}
	return manifest, signed, nil
}

func isJWSManifestArtifact(data []byte, contentType string) bool {
	normalizedContentType := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(normalizedContentType, "jose") {
		return true
	}
	if strings.Contains(normalizedContentType, "json") {
		return false
	}
	return isCompactJWS(data)
}

func isCompactJWS(data []byte) bool {
	parts := strings.Split(strings.TrimSpace(string(data)), ".")
	if len(parts) != 3 || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
		return false
	}
	protectedHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var header map[string]any
	if err = json.Unmarshal(protectedHeader, &header); err != nil {
		return false
	}
	_, hasAlgorithm := header["alg"].(string)
	return hasAlgorithm
}
