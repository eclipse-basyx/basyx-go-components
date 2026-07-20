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
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
)

const (
	// TableBinaryEvidenceReceipt catalogs immutable canonical binary objects.
	TableBinaryEvidenceReceipt = "binary_evidence_receipt"
	// TableBinaryReferenceEvidence catalogs owner-scoped binary reference artifacts.
	TableBinaryReferenceEvidence = "binary_reference_evidence_artifacts"
	binaryReferenceVersion       = "basyx-binary-reference-v1"
)

type binaryReferenceExpectationContextKey struct{}

// BinaryReferenceExpectation commits the uploaded binary identity into the
// mutation-event hash before its separate reference artifact is written.
type BinaryReferenceExpectation struct {
	ModelPath       string            `json:"model_path"`
	SHA256          string            `json:"sha256"`
	SizeBytes       int64             `json:"size_bytes"`
	FileName        string            `json:"file_name"`
	ContentType     string            `json:"content_type"`
	BinaryReference EvidenceReference `json:"binary_reference"`
}

// NewBinaryReferenceExpectation builds the binary descriptor committed by a mutation event.
func NewBinaryReferenceExpectation(content binarycontent.Content, modelPath string, fileName string, contentType string, receipt *EvidenceReceipt) (BinaryReferenceExpectation, error) {
	if receipt == nil {
		return BinaryReferenceExpectation{}, nil
	}
	expectation := BinaryReferenceExpectation{
		ModelPath: strings.TrimSpace(modelPath), SHA256: strings.ToLower(content.SHA256),
		SizeBytes: content.SizeBytes, FileName: strings.TrimSpace(fileName),
		ContentType: strings.TrimSpace(contentType), BinaryReference: receipt.Reference,
	}
	if !validBinaryReferenceExpectation(expectation) {
		return BinaryReferenceExpectation{}, common.NewInternalServerError("HISTORY-EVIDENCE-BINARYREF-EXPECTATION uploaded binary descriptor is incomplete")
	}
	return expectation, nil
}

// WithBinaryReferenceExpected marks a mutation that must commit matching binary-reference evidence.
func WithBinaryReferenceExpected(ctx context.Context, expectation BinaryReferenceExpectation) context.Context {
	if expectation.ModelPath == "" {
		return ctx
	}
	expected := append([]BinaryReferenceExpectation(nil), binaryReferencesExpected(ctx)...)
	expected = append(expected, expectation)
	return context.WithValue(ctx, binaryReferenceExpectationContextKey{}, expected)
}

func binaryReferencesExpected(ctx context.Context) []BinaryReferenceExpectation {
	if ctx == nil {
		return nil
	}
	values, _ := ctx.Value(binaryReferenceExpectationContextKey{}).([]BinaryReferenceExpectation)
	return values
}

func validBinaryReferenceExpectation(expectation BinaryReferenceExpectation) bool {
	return strings.HasPrefix(expectation.ModelPath, "/aasx/files/") &&
		validSHA256(expectation.SHA256) && expectation.SizeBytes >= 0 &&
		strings.TrimSpace(expectation.FileName) != "" && strings.TrimSpace(expectation.ContentType) != "" &&
		expectation.BinaryReference.Provider == EvidenceProviderS3 &&
		strings.TrimSpace(expectation.BinaryReference.ObjectKey) != "" && strings.TrimSpace(expectation.BinaryReference.VersionID) != ""
}

// EnsureBinaryEvidenceTx writes canonical binary bytes once and reuses their
// immutable receipt across authorized logical references.
func EnsureBinaryEvidenceTx(ctx context.Context, tx *sql.Tx, content binarycontent.Content, contentType string) (*EvidenceReceipt, error) {
	cfg := ActiveConfig()
	if !cfg.EvidenceEnabled {
		return nil, nil
	}
	if len(content.SHA256) != 64 {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-BINARY-DIGEST canonical binary digest is invalid")
	}
	if cfg.EvidenceStore == nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-BINARY-NILSTORE evidence store is not initialized")
	}
	receipt, err := loadBinaryEvidenceReceiptTx(ctx, tx, content.SHA256, content.SizeBytes)
	artifact := EvidenceArtifact{
		ArtifactType: EvidenceArtifactBinary,
		ObjectKey:    path.Join("binary-content", content.SHA256[:2], content.SHA256),
		ContentType:  strings.TrimSpace(contentType),
		Metadata: map[string]string{
			"artifact_type": EvidenceArtifactBinary, "sha256": content.SHA256,
			"size_bytes": fmt.Sprintf("%d", content.SizeBytes),
		},
	}
	if artifact.ContentType == "" {
		artifact.ContentType = "application/octet-stream"
	}
	if err == nil {
		if extender, ok := cfg.EvidenceStore.(EvidenceRetentionExtender); ok {
			extendCtx := ctx
			cancelExtend := func() {}
			if cfg.EvidenceWriteTimeout > 0 {
				extendCtx, cancelExtend = context.WithTimeout(ctx, cfg.EvidenceWriteTimeout)
			}
			extended, extendErr := extender.ExtendArtifactRetention(extendCtx, receipt.Reference, receipt, artifact)
			cancelExtend()
			if extendErr != nil {
				log.Printf("HISTORY-EVIDENCE-BINARY-EXTEND immutable binary retention extension failed: %v", extendErr)
				return nil, binaryEvidenceUnavailableError()
			}
			if extended != nil {
				receipt = *extended
				if updateErr := upsertBinaryEvidenceReceiptTx(ctx, tx, content.ID, receipt); updateErr != nil {
					return nil, updateErr
				}
			}
		}
		if validationErr := validateCommittedEvidenceReceipt(receipt, content.SHA256, content.SizeBytes, time.Now()); validationErr != nil {
			log.Printf("HISTORY-EVIDENCE-BINARY-RECEIPT reused immutable binary receipt is invalid: %v", validationErr)
			return nil, binaryEvidenceUnavailableError()
		}
		return &receipt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	writeCtx := ctx
	cancel := func() {}
	if cfg.EvidenceWriteTimeout > 0 {
		writeCtx, cancel = context.WithTimeout(ctx, cfg.EvidenceWriteTimeout)
	}
	defer cancel()
	var storedReceipt *EvidenceReceipt
	err = binarycontent.StreamTx(writeCtx, tx, content, func(reader io.Reader) error {
		var putErr error
		if streamingStore, ok := cfg.EvidenceStore.(EvidenceStreamStore); ok {
			storedReceipt, putErr = streamingStore.PutArtifactReader(writeCtx, artifact, reader, content.SizeBytes, content.SHA256)
			return putErr
		}
		data, readErr := io.ReadAll(reader)
		if readErr != nil {
			return readErr
		}
		artifact.Data = data
		storedReceipt, putErr = cfg.EvidenceStore.PutArtifact(writeCtx, artifact)
		return putErr
	})
	if err != nil {
		log.Printf("HISTORY-EVIDENCE-BINARY-PUT immutable binary write failed: %v", err)
		return nil, binaryEvidenceUnavailableError()
	}
	if storedReceipt == nil {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-BINARY-NILRECEIPT evidence store returned nil receipt")
	}
	if !strings.EqualFold(storedReceipt.SHA256, content.SHA256) || storedReceipt.SizeBytes != content.SizeBytes {
		return nil, common.NewInternalServerError("HISTORY-EVIDENCE-BINARY-RECEIPT immutable binary receipt does not match canonical content")
	}
	if validationErr := validateCommittedEvidenceReceipt(*storedReceipt, content.SHA256, content.SizeBytes, time.Now()); validationErr != nil {
		log.Printf("HISTORY-EVIDENCE-BINARY-RECEIPT immutable binary receipt is invalid: %v", validationErr)
		return nil, binaryEvidenceUnavailableError()
	}
	if err = upsertBinaryEvidenceReceiptTx(ctx, tx, content.ID, *storedReceipt); err != nil {
		return nil, err
	}
	return storedReceipt, nil
}

// RecordBinaryReferenceEvidenceTx binds per-upload metadata to the latest
// mutation event for the owning entity.
func RecordBinaryReferenceEvidenceTx(ctx context.Context, tx *sql.Tx, entityType string, identifier string, expectation BinaryReferenceExpectation) error {
	if expectation.ModelPath == "" || !ActiveConfig().EvidenceEnabled {
		return nil
	}
	mutationID, sequence, eventHash, err := latestMutationEvidenceIdentityTx(ctx, tx, entityType, identifier)
	if err != nil {
		return err
	}
	document := map[string]any{
		"artifact_version": binaryReferenceVersion,
		"entity_type":      entityType, "identifier": identifier,
		"event_sequence": sequence, "event_hash": eventHash,
		"model_path": expectation.ModelPath, "file_name": expectation.FileName,
		"content_type": expectation.ContentType, "sha256": expectation.SHA256,
		"size_bytes": expectation.SizeBytes, "binary_reference": expectation.BinaryReference,
	}
	data, err := CanonicalJSON(document)
	if err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-BINARYREF-CANONICAL " + err.Error())
	}
	artifact := EvidenceArtifact{
		ArtifactType: EvidenceArtifactBinaryRef,
		ObjectKey:    path.Join("binary-references", url.PathEscape(entityType), url.PathEscape(identifier), fmt.Sprintf("%d-%s.json", sequence, eventHash)),
		ContentType:  manifestJSONContentType, Data: data,
		Metadata: map[string]string{
			"artifact_type": EvidenceArtifactBinaryRef, "entity_type": entityType,
			"identifier": identifier, "event_hash": eventHash, "binary_sha256": expectation.SHA256,
		},
	}
	cfg := ActiveConfig()
	writeCtx := ctx
	cancel := func() {}
	if cfg.EvidenceWriteTimeout > 0 {
		writeCtx, cancel = context.WithTimeout(ctx, cfg.EvidenceWriteTimeout)
	}
	defer cancel()
	receipt, err := cfg.EvidenceStore.PutArtifact(writeCtx, artifact)
	if err != nil {
		log.Printf("HISTORY-EVIDENCE-BINARYREF-PUT evidence store write failed: %v", err)
		return common.NewErrServiceUnavailable("HISTORY-EVIDENCE-BINARYREF-STORE binary reference evidence could not be stored")
	}
	if receipt == nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-BINARYREF-NILRECEIPT evidence store returned nil receipt")
	}
	if validationErr := validateCommittedEvidenceReceipt(*receipt, SHA256Hex(data), int64(len(data)), time.Now()); validationErr != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-BINARYREF-RECEIPT " + validationErr.Error())
	}
	return insertBinaryReferenceReceiptTx(ctx, tx, mutationID, expectation.ModelPath, *receipt)
}

func binaryEvidenceUnavailableError() error {
	return common.NewErrServiceUnavailable("HISTORY-EVIDENCE-BINARY-STORE immutable binary evidence could not be stored")
}

func loadBinaryEvidenceReceiptTx(ctx context.Context, tx *sql.Tx, digest string, sizeBytes int64) (EvidenceReceipt, error) {
	query, args, err := goqu.From(TableBinaryEvidenceReceipt).
		Select("provider", "bucket", "object_key", "object_version_id", "sha256", "size_bytes", "content_type", "retention_mode", "retain_until", "legal_hold", "artifact_metadata", "db_created_at").
		Where(goqu.Ex{"sha256": digest, "size_bytes": sizeBytes}).ToSQL()
	if err != nil {
		return EvidenceReceipt{}, common.NewInternalServerError("HISTORY-EVIDENCE-BINARY-BUILDLOAD " + err.Error())
	}
	var receipt EvidenceReceipt
	var bucket, version, retention sql.NullString
	var retainUntil sql.NullTime
	var metadata []byte
	if err = tx.QueryRowContext(ctx, query, args...).Scan(
		&receipt.Reference.Provider, &bucket, &receipt.Reference.ObjectKey, &version,
		&receipt.SHA256, &receipt.SizeBytes, &receipt.ContentType, &retention, &retainUntil,
		&receipt.LegalHold, &metadata, &receipt.StoredAt,
	); err != nil {
		return EvidenceReceipt{}, err
	}
	receipt.Reference.Bucket = bucket.String
	receipt.Reference.VersionID = version.String
	receipt.RetentionMode = retention.String
	if retainUntil.Valid {
		receipt.RetainUntil = &retainUntil.Time
	}
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &receipt.Metadata)
	}
	return receipt, nil
}

func upsertBinaryEvidenceReceiptTx(ctx context.Context, tx *sql.Tx, contentID int64, receipt EvidenceReceipt) error {
	record := goqu.Record{
		"binary_content_id": contentID, "provider": receipt.Reference.Provider,
		"bucket": nullableText(receipt.Reference.Bucket), "object_key": receipt.Reference.ObjectKey,
		"object_version_id": nullableText(receipt.Reference.VersionID), "sha256": receipt.SHA256,
		"size_bytes": receipt.SizeBytes, "content_type": receipt.ContentType,
		"retention_mode": nullableText(receipt.RetentionMode), "retain_until": nullableTime(receipt.RetainUntil),
		"legal_hold": receipt.LegalHold, "artifact_metadata": jsonbMetadata(receipt.Metadata), "db_updated_at": time.Now().UTC(),
	}
	query, args, err := goqu.Insert(TableBinaryEvidenceReceipt).Rows(record).
		OnConflict(goqu.DoUpdate("sha256,size_bytes", record)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-BINARY-BUILDUPSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-BINARY-UPSERT " + err.Error())
	}
	return nil
}

func latestMutationEvidenceIdentityTx(ctx context.Context, tx *sql.Tx, entityType string, identifier string) (int64, int64, string, error) {
	query, args, err := goqu.From(TableMutationEvidenceEvents).
		Select("artifact_id", "event_sequence", "event_hash").
		Where(goqu.Ex{"entity_type": entityType, "identifier_digest": mutationIdentifierDigest(identifier), "identifier": identifier}).
		Order(goqu.C("event_sequence").Desc()).Limit(1).ToSQL()
	if err != nil {
		return 0, 0, "", common.NewInternalServerError("HISTORY-EVIDENCE-BINARYREF-BUILDMUTATION " + err.Error())
	}
	var artifactID, sequence int64
	var eventHash string
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&artifactID, &sequence, &eventHash); err != nil {
		return 0, 0, "", common.NewInternalServerError("HISTORY-EVIDENCE-BINARYREF-MUTATION " + err.Error())
	}
	return artifactID, sequence, eventHash, nil
}

func insertBinaryReferenceReceiptTx(ctx context.Context, tx *sql.Tx, mutationID int64, modelPath string, receipt EvidenceReceipt) error {
	record := goqu.Record{
		"mutation_artifact_id": mutationID, "model_path": modelPath,
		"provider": receipt.Reference.Provider, "bucket": nullableText(receipt.Reference.Bucket),
		"object_key": receipt.Reference.ObjectKey, "object_version_id": nullableText(receipt.Reference.VersionID),
		"sha256": receipt.SHA256, "size_bytes": receipt.SizeBytes, "content_type": receipt.ContentType,
		"retention_mode": nullableText(receipt.RetentionMode), "retain_until": nullableTime(receipt.RetainUntil),
		"legal_hold": receipt.LegalHold, "artifact_metadata": jsonbMetadata(receipt.Metadata),
	}
	query, args, err := goqu.Insert(TableBinaryReferenceEvidence).Rows(record).ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-BINARYREF-BUILDINSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("HISTORY-EVIDENCE-BINARYREF-INSERT " + err.Error())
	}
	return nil
}
