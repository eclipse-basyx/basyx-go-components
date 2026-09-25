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

package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/evidence"
)

// ManifestSigner optionally signs the deterministic audit event envelope before storage.
// Implementations must return identical signatures for identical envelope bytes so a
// retry after an acknowledgement failure writes the same immutable artifact.
type ManifestSigner interface {
	SignManifest(ctx context.Context, envelope []byte) ([]byte, error)
}

// Archiver transfers queued audit records to immutable evidence storage.
type Archiver struct {
	DB     *sql.DB
	Store  evidence.Store
	Signer ManifestSigner
}

type archiveRecord struct {
	EventID, Stream, Actor, Resource, Outcome, CorrelationID, PreviousHash, ContentHash string
	Sequence                                                                            int64
	OccurredAt                                                                          time.Time
	Event                                                                               json.RawMessage
}

// ArchiveNext archives one pending delivery and returns true when work was completed.
func (archiver Archiver) ArchiveNext(ctx context.Context) (bool, error) {
	if archiver.DB == nil || archiver.Store == nil {
		return false, fmt.Errorf("AUDIT-ARCHIVE-CONFIG database and evidence store are required")
	}
	record, found, err := archiver.loadPending(ctx)
	if err != nil || !found {
		return found, err
	}
	artifact, err := archiver.artifact(ctx, record)
	if err != nil {
		return false, err
	}
	receipt, err := archiver.Store.PutArtifact(ctx, artifact)
	if err != nil {
		return false, fmt.Errorf("AUDIT-ARCHIVE-PUT: %w", err)
	}
	if err = verifyArchiveReceipt(ctx, archiver.Store, artifact, receipt); err != nil {
		return false, err
	}
	if err = archiver.acknowledge(ctx, record.EventID, *receipt); err != nil {
		return false, err
	}
	return true, nil
}

func (archiver Archiver) loadPending(ctx context.Context) (archiveRecord, bool, error) {
	query, args, err := dialect.From("audit_delivery").
		Join(goqu.T("audit_record"), goqu.On(goqu.I("audit_delivery.event_id").Eq(goqu.I("audit_record.event_id")))).
		Select("audit_record.event_id", "stream", "sequence", "occurred_at", "actor", "resource", "outcome", "correlation_id", "event", "previous_hash", "content_hash").
		Where(goqu.I("audit_delivery.archived_at").IsNull()).Order(goqu.I("audit_record.stream").Asc(), goqu.I("audit_record.sequence").Asc()).Limit(1).Prepared(true).ToSQL()
	if err != nil {
		return archiveRecord{}, false, fmt.Errorf("AUDIT-ARCHIVE-SELECTSQL: %w", err)
	}
	var record archiveRecord
	err = archiver.DB.QueryRowContext(ctx, query, args...).Scan(&record.EventID, &record.Stream, &record.Sequence, &record.OccurredAt, &record.Actor, &record.Resource, &record.Outcome, &record.CorrelationID, &record.Event, &record.PreviousHash, &record.ContentHash)
	if err == sql.ErrNoRows {
		return archiveRecord{}, false, nil
	}
	if err != nil {
		return archiveRecord{}, false, fmt.Errorf("AUDIT-ARCHIVE-SELECT: %w", err)
	}
	return record, true, nil
}

func (archiver Archiver) artifact(ctx context.Context, record archiveRecord) (evidence.Artifact, error) {
	envelope, err := json.Marshal(record)
	if err != nil {
		return evidence.Artifact{}, fmt.Errorf("AUDIT-ARCHIVE-ENVELOPE: %w", err)
	}
	if archiver.Signer != nil {
		signature, signErr := archiver.Signer.SignManifest(ctx, envelope)
		if signErr != nil {
			return evidence.Artifact{}, fmt.Errorf("AUDIT-ARCHIVE-SIGN: %w", signErr)
		}
		envelope, err = json.Marshal(struct {
			Event     json.RawMessage `json:"event"`
			Signature []byte          `json:"signature"`
		}{envelope, signature})
		if err != nil {
			return evidence.Artifact{}, fmt.Errorf("AUDIT-ARCHIVE-SIGNEDENVELOPE: %w", err)
		}
	}
	return evidence.Artifact{ArtifactType: evidence.ArtifactHistoryEvent, ObjectKey: path.Join("audit", strings.TrimSpace(record.Stream), fmt.Sprintf("%020d-%s.json", record.Sequence, record.EventID)), ContentType: "application/json", Data: envelope, Metadata: map[string]string{"event_id": record.EventID, "stream": record.Stream, "sequence": fmt.Sprint(record.Sequence), "content_hash": record.ContentHash}}, nil
}

func verifyArchiveReceipt(ctx context.Context, store evidence.Store, artifact evidence.Artifact, receipt *evidence.Receipt) error {
	if receipt == nil || strings.TrimSpace(receipt.Reference.VersionID) == "" || !strings.EqualFold(receipt.SHA256, evidence.SHA256Hex(artifact.Data)) {
		return fmt.Errorf("AUDIT-ARCHIVE-RECEIPT evidence receipt is incomplete or does not match the archived envelope")
	}
	verified, err := store.VerifyArtifact(ctx, receipt.Reference, receipt.SHA256)
	if err != nil || verified == nil || !strings.EqualFold(verified.SHA256, receipt.SHA256) {
		return fmt.Errorf("AUDIT-ARCHIVE-VERIFY: %w", err)
	}
	retentionStore, ok := store.(evidence.RetentionVerifier)
	if !ok {
		return fmt.Errorf("AUDIT-ARCHIVE-RETENTION evidence store must verify retention")
	}
	if err = retentionStore.VerifyArtifactRetention(ctx, receipt.Reference, *receipt); err != nil {
		return fmt.Errorf("AUDIT-ARCHIVE-RETENTION: %w", err)
	}
	return nil
}

func (archiver Archiver) acknowledge(ctx context.Context, eventID string, receipt evidence.Receipt) error {
	payload, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("AUDIT-ARCHIVE-RECEIPTJSON: %w", err)
	}
	query, args, err := dialect.Update("audit_delivery").Set(goqu.Record{"receipt": goqu.L("?::jsonb", string(payload)), "archived_at": time.Now().UTC(), "attempts": goqu.L("attempts + 1")}).Where(goqu.C("event_id").Eq(eventID), goqu.C("archived_at").IsNull()).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AUDIT-ARCHIVE-ACKSQL: %w", err)
	}
	result, err := archiver.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("AUDIT-ARCHIVE-ACK: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return fmt.Errorf("AUDIT-ARCHIVE-ACKCOUNT expected one pending delivery")
	}
	return nil
}
