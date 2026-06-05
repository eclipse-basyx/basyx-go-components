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
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3EvidenceStoreConfig configures an S3-compatible WORM evidence backend.
//
// Bucket and region are required to create the client. RetentionMode and
// RetentionDays are required for PutArtifact calls because evidence writes must
// be protected by object-lock retention.
type S3EvidenceStoreConfig struct {
	Bucket          string
	Prefix          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	RetentionMode   string
	RetentionDays   int
}

// S3EvidenceStore stores evidence artifacts in AWS S3 or an S3-compatible backend.
type S3EvidenceStore struct {
	client *s3.Client
	cfg    S3EvidenceStoreConfig
	now    func() time.Time
}

// NewS3EvidenceStore creates an EvidenceStore backed by S3-compatible object storage.
//
// The store supports AWS S3 and endpoint-compatible backends such as MinIO. A
// bucket and region are always required. Retention settings may be omitted for
// read-only verifier usage, but writes fail unless object-lock retention is
// configured through RetentionMode and RetentionDays.
//
// Parameters:
//   - ctx: Startup context used while loading AWS SDK configuration.
//   - cfg: S3 bucket, endpoint, credential, prefix, and retention settings.
//
// Returns:
//   - *S3EvidenceStore: Initialized evidence store.
//   - error: Error when configuration is incomplete or AWS SDK setup fails.
func NewS3EvidenceStore(ctx context.Context, cfg S3EvidenceStoreConfig) (*S3EvidenceStore, error) {
	normalized, err := normalizeS3EvidenceStoreConfig(cfg)
	if err != nil {
		return nil, err
	}
	loadOptions := []func(*config.LoadOptions) error{config.WithRegion(normalized.Region)}
	if normalized.AccessKeyID != "" {
		loadOptions = append(loadOptions, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(normalized.AccessKeyID, normalized.SecretAccessKey, ""),
		))
	}
	if normalized.Endpoint != "" {
		loadOptions = append(loadOptions, config.WithBaseEndpoint(normalized.Endpoint))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-LOADCONFIG %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = normalized.UsePathStyle
	})
	return &S3EvidenceStore{client: client, cfg: normalized, now: func() time.Time { return time.Now().UTC() }}, nil
}

func normalizeS3EvidenceStoreConfig(cfg S3EvidenceStoreConfig) (S3EvidenceStoreConfig, error) {
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.Prefix = strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	cfg.Region = strings.TrimSpace(cfg.Region)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.SecretAccessKey = strings.TrimSpace(cfg.SecretAccessKey)
	cfg.RetentionMode = strings.ToLower(strings.TrimSpace(cfg.RetentionMode))
	if cfg.Bucket == "" {
		return S3EvidenceStoreConfig{}, fmt.Errorf("HISTORY-EVIDENCE-S3-BUCKET bucket is required")
	}
	if cfg.Region == "" {
		return S3EvidenceStoreConfig{}, fmt.Errorf("HISTORY-EVIDENCE-S3-REGION region is required")
	}
	if (cfg.AccessKeyID == "") != (cfg.SecretAccessKey == "") {
		return S3EvidenceStoreConfig{}, fmt.Errorf("HISTORY-EVIDENCE-S3-CREDENTIALS access key and secret access key must be configured together")
	}
	switch cfg.RetentionMode {
	case "", "governance", "compliance":
	default:
		return S3EvidenceStoreConfig{}, fmt.Errorf("HISTORY-EVIDENCE-S3-RETENTIONMODE unsupported retention mode %q", cfg.RetentionMode)
	}
	if cfg.RetentionDays < 0 {
		return S3EvidenceStoreConfig{}, fmt.Errorf("HISTORY-EVIDENCE-S3-RETENTIONDAYS retention days must not be negative")
	}
	return cfg, nil
}

// PutArtifact writes one immutable evidence artifact to S3.
//
// The object is written with object-lock retention metadata. Evidence writes
// fail closed when no retention mode and retain-until timestamp can be derived
// from the artifact or store configuration.
//
// Parameters:
//   - ctx: Request context for the S3 PutObject call.
//   - artifact: Canonical evidence bytes plus object key and metadata.
//
// Returns:
//   - *EvidenceReceipt: Object reference, version, hash, retention, and metadata.
//   - error: Error when the store is uninitialized, retention is missing, or S3
//     rejects the write.
func (store *S3EvidenceStore) PutArtifact(ctx context.Context, artifact EvidenceArtifact) (*EvidenceReceipt, error) {
	if store == nil || store.client == nil {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-NILSTORE evidence store is not initialized")
	}
	objectKey := store.objectKey(artifact.ObjectKey)
	if objectKey == "" {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-EMPTYKEY artifact object key is required")
	}
	receipt := store.receiptForArtifact(objectKey, artifact)
	if receipt.RetentionMode == "" || receipt.RetainUntil == nil {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-RETENTION retention mode and retain-until timestamp are required for evidence writes")
	}
	input := &s3.PutObjectInput{
		Bucket:      aws.String(store.cfg.Bucket),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(artifact.Data),
		ContentType: aws.String(artifact.ContentType),
		Metadata:    cleanS3Metadata(receipt.Metadata),
	}
	applyS3Retention(input, receipt)
	output, err := store.client.PutObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-PUTOBJECT %w", err)
	}
	if output.VersionId != nil {
		receipt.Reference.VersionID = strings.TrimSpace(*output.VersionId)
	}
	return receipt, nil
}

// GetArtifact reads an evidence artifact from S3.
//
// Parameters:
//   - ctx: Request context for the S3 GetObject call.
//   - ref: Provider object reference to read. VersionID is used when present.
//
// Returns:
//   - *EvidenceObject: Downloaded bytes, content type, metadata, and resolved reference.
//   - error: Error when the store is uninitialized, the reference is incomplete,
//     or S3 cannot return the object.
func (store *S3EvidenceStore) GetArtifact(ctx context.Context, ref EvidenceReference) (*EvidenceObject, error) {
	if store == nil || store.client == nil {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-NILSTORE evidence store is not initialized")
	}
	objectKey := strings.TrimSpace(ref.ObjectKey)
	if objectKey == "" {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-EMPTYREF artifact object key is required")
	}
	input := &s3.GetObjectInput{Bucket: aws.String(store.cfg.Bucket), Key: aws.String(objectKey)}
	if strings.TrimSpace(ref.VersionID) != "" {
		input.VersionId = aws.String(strings.TrimSpace(ref.VersionID))
	}
	output, err := store.client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-GETOBJECT %w", err)
	}
	defer func() {
		_ = output.Body.Close()
	}()
	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-READOBJECT %w", err)
	}
	versionID := strings.TrimSpace(ref.VersionID)
	if output.VersionId != nil && strings.TrimSpace(*output.VersionId) != "" {
		versionID = strings.TrimSpace(*output.VersionId)
	}
	contentType := ""
	if output.ContentType != nil {
		contentType = strings.TrimSpace(*output.ContentType)
	}
	return &EvidenceObject{
		Reference:   EvidenceReference{Provider: EvidenceProviderS3, Bucket: store.cfg.Bucket, ObjectKey: objectKey, VersionID: versionID},
		Data:        data,
		ContentType: contentType,
		Metadata:    copyStringMap(output.Metadata),
	}, nil
}

// VerifyArtifact checks that the stored object bytes match expectedHash.
//
// Parameters:
//   - ctx: Request context for reading the artifact.
//   - ref: Provider object reference to verify.
//   - expectedHash: Expected lowercase or uppercase hexadecimal SHA-256 digest.
//
// Returns:
//   - *EvidenceReceipt: Receipt reconstructed from the downloaded object.
//   - error: Error when the object cannot be read or its SHA-256 digest differs.
func (store *S3EvidenceStore) VerifyArtifact(ctx context.Context, ref EvidenceReference, expectedHash string) (*EvidenceReceipt, error) {
	object, err := store.GetArtifact(ctx, ref)
	if err != nil {
		return nil, err
	}
	actualHash := SHA256Hex(object.Data)
	if !strings.EqualFold(strings.TrimSpace(expectedHash), actualHash) {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-S3-HASHMISMATCH artifact hash mismatch")
	}
	return &EvidenceReceipt{
		Reference:   object.Reference,
		SHA256:      actualHash,
		SizeBytes:   int64(len(object.Data)),
		ContentType: object.ContentType,
		StoredAt:    store.now(),
		Metadata:    object.Metadata,
	}, nil
}

func (store *S3EvidenceStore) objectKey(rawKey string) string {
	key := strings.Trim(strings.TrimSpace(rawKey), "/")
	if key == "" {
		return ""
	}
	if store.cfg.Prefix == "" || strings.HasPrefix(key, store.cfg.Prefix+"/") {
		return key
	}
	return path.Join(store.cfg.Prefix, key)
}

func (store *S3EvidenceStore) receiptForArtifact(objectKey string, artifact EvidenceArtifact) *EvidenceReceipt {
	retentionMode, retainUntil := store.retention(artifact)
	metadata := copyStringMap(artifact.Metadata)
	metadata["sha256"] = SHA256Hex(artifact.Data)
	metadata["artifact_type"] = strings.TrimSpace(artifact.ArtifactType)
	return &EvidenceReceipt{
		Reference: EvidenceReference{
			Provider:  EvidenceProviderS3,
			Bucket:    store.cfg.Bucket,
			ObjectKey: objectKey,
		},
		SHA256:        metadata["sha256"],
		SizeBytes:     int64(len(artifact.Data)),
		ContentType:   strings.TrimSpace(artifact.ContentType),
		RetentionMode: retentionMode,
		RetainUntil:   retainUntil,
		LegalHold:     artifact.LegalHold,
		StoredAt:      store.now(),
		Metadata:      metadata,
	}
}

func (store *S3EvidenceStore) retention(artifact EvidenceArtifact) (string, *time.Time) {
	mode := strings.ToLower(strings.TrimSpace(artifact.RetentionMode))
	if mode == "" {
		mode = store.cfg.RetentionMode
	}
	if mode == "" {
		return "", nil
	}
	if !artifact.RetainUntil.IsZero() {
		retainUntil := artifact.RetainUntil.UTC()
		return mode, &retainUntil
	}
	if store.cfg.RetentionDays < 1 {
		return "", nil
	}
	retainUntil := store.now().AddDate(0, 0, store.cfg.RetentionDays).UTC()
	return mode, &retainUntil
}

func applyS3Retention(input *s3.PutObjectInput, receipt *EvidenceReceipt) {
	if receipt.LegalHold {
		input.ObjectLockLegalHoldStatus = types.ObjectLockLegalHoldStatusOn
	}
	if receipt.RetainUntil == nil || receipt.RetentionMode == "" {
		return
	}
	input.ObjectLockRetainUntilDate = receipt.RetainUntil
	switch strings.ToLower(receipt.RetentionMode) {
	case "compliance":
		input.ObjectLockMode = types.ObjectLockModeCompliance
	case "governance":
		input.ObjectLockMode = types.ObjectLockModeGovernance
	}
}

func cleanS3Metadata(metadata map[string]string) map[string]string {
	cleaned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cleanKey := strings.ToLower(strings.TrimSpace(key))
		if cleanKey == "" {
			continue
		}
		cleaned[cleanKey] = strings.TrimSpace(value)
	}
	return cleaned
}

func copyStringMap(values map[string]string) map[string]string {
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
