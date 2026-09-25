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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const (
	auditTable = "rebac_audit_event"
	// auditChainVersion versions the hashed content of audit events.
	auditChainVersion = "basyx-rebac-audit-v1"
	// auditArtifactType marks audit events archived as WORM evidence.
	auditArtifactType = "rebac_audit_event"
	systemActor       = "system"
	// auditSystemObject is the object of events concerning all ReBAC state.
	auditSystemObject = "rebac"
	defaultAuditPage  = 100
	maxAuditPage      = 1000
)

// Audit event types.
const (
	AuditGrantsChanged      = "grants_changed"
	AuditInvitationCreated  = "invitation_created"
	AuditInvitationRevoked  = "invitation_revoked"
	AuditInvitationRedeemed = "invitation_redeemed"
	AuditInheritanceChanged = "inheritance_changed"
	AuditReconciled         = "reconciled"
)

// AuditEvent is one entry of the append-only, hash-chained audit trail of
// access changes. Hash covers the event and the hash of its predecessor.
type AuditEvent struct {
	ID           int64           `json:"id"`
	OccurredAt   time.Time       `json:"occurredAt"`
	Type         string          `json:"type"`
	Actor        string          `json:"actor"`
	Object       string          `json:"object"`
	Details      json.RawMessage `json:"details"`
	PreviousHash string          `json:"previousHash,omitempty"`
	Hash         string          `json:"hash"`
	Evidence     json.RawMessage `json:"evidence,omitempty"`
}

// hashedContent returns the canonical content the event hash covers.
func (e AuditEvent) hashedContent() (map[string]any, error) {
	var details any
	if err := json.Unmarshal(e.Details, &details); err != nil {
		return nil, fmt.Errorf("REBAC-AUDIT-DETAILS: %w", err)
	}
	return map[string]any{
		"version": auditChainVersion, "id": e.ID, "occurredAt": e.OccurredAt.UTC().Format(time.RFC3339Nano),
		"type": e.Type, "actor": e.Actor, "object": e.Object, "details": details, "previousHash": e.PreviousHash,
	}, nil
}

func (e AuditEvent) computeHash() (string, error) {
	content, err := e.hashedContent()
	if err != nil {
		return "", err
	}
	return common.CanonicalJSONHash(content)
}

// evidenceDocument is the archived form of an event: its content and hash.
func (e AuditEvent) evidenceDocument() ([]byte, error) {
	content, err := e.hashedContent()
	if err != nil {
		return nil, err
	}
	content["hash"] = e.Hash
	return common.CanonicalJSON(content)
}

// audit appends an event to the audit trail in tx. Writers are serialized,
// so the chain has no forks. With history evidence enabled the event is
// archived in the WORM store before the transaction commits.
func (c *Coordinator) audit(ctx context.Context, tx *sql.Tx, eventType string, object string, details any) error {
	detailsJSON, err := common.CanonicalJSON(details)
	if err != nil {
		return fmt.Errorf("REBAC-AUDIT-CANONICAL: %w", err)
	}
	event := AuditEvent{Type: eventType, Actor: c.actor(ctx), Object: object, Details: detailsJSON, OccurredAt: time.Now().UTC().Truncate(time.Microsecond)}
	if err = lockAuditChain(ctx, tx); err != nil {
		return err
	}
	if event.ID, event.PreviousHash, err = nextAuditPosition(ctx, tx); err != nil {
		return err
	}
	if event.Hash, err = event.computeHash(); err != nil {
		return err
	}
	if event.Evidence, err = archiveAuditEvent(ctx, event); err != nil {
		return err
	}
	if err = insertAuditEvent(ctx, tx, event); err != nil {
		return err
	}
	c.countChange(ctx, eventType)
	return nil
}

func (c *Coordinator) actor(ctx context.Context) string {
	if principal, ok := PrincipalFromClaims(auth.ClaimsFromContext(ctx), c.groupClaim); ok && auth.IsAuthenticated(ctx) {
		return principal.UserKey()
	}
	return systemActor
}

func lockAuditChain(ctx context.Context, tx *sql.Tx) error {
	ds := dialect.Select(goqu.Func("pg_advisory_xact_lock", goqu.Func("hashtextextended", auditTable, int64(0))))
	_, err := execSelect(ctx, tx, "REBAC-AUDIT-LOCK", ds)
	return err
}

// nextAuditPosition reserves the event id and returns the current head hash.
func nextAuditPosition(ctx context.Context, tx *sql.Tx) (int64, string, error) {
	head := dialect.From(auditTable).Select(goqu.C("event_hash")).Order(goqu.C("id").Desc()).Limit(1)
	ds := dialect.Select(
		goqu.Func("nextval", goqu.Func("pg_get_serial_sequence", auditTable, "id")),
		goqu.Func("COALESCE", head, ""),
	).Prepared(true)
	var id int64
	var previous string
	_, err := queryRowDataset(ctx, tx, "REBAC-AUDIT-POSITION", ds, &id, &previous)
	return id, previous, err
}

func insertAuditEvent(ctx context.Context, tx *sql.Tx, event AuditEvent) error {
	record := goqu.Record{
		"id": event.ID, "occurred_at": event.OccurredAt, "event_type": event.Type, "actor_key": event.Actor,
		"object_key": event.Object, "details": string(event.Details), "event_hash": event.Hash,
	}
	if event.PreviousHash != "" {
		record["previous_hash"] = event.PreviousHash
	}
	if len(event.Evidence) > 0 {
		record["evidence_receipt"] = string(event.Evidence)
	}
	_, err := execDataset(ctx, tx, "REBAC-AUDIT-INSERT", dialect.Insert(auditTable).Rows(record).Prepared(true))
	return err
}

// archiveAuditEvent stores the event in the history evidence store and
// returns the receipt, or nil when evidence is disabled.
func archiveAuditEvent(ctx context.Context, event AuditEvent) (json.RawMessage, error) {
	cfg := history.ActiveConfig()
	if !cfg.EvidenceEnabled {
		return nil, nil
	}
	if cfg.EvidenceStore == nil {
		return nil, fmt.Errorf("REBAC-AUDIT-EVIDENCESTORE evidence store is not initialized")
	}
	document, err := event.evidenceDocument()
	if err != nil {
		return nil, err
	}
	writeCtx, cancel := context.WithTimeout(ctx, cfg.EvidenceWriteTimeout)
	defer cancel()
	receipt, err := cfg.EvidenceStore.PutArtifact(writeCtx, history.EvidenceArtifact{
		ArtifactType: auditArtifactType,
		ObjectKey:    auditObjectKey(event),
		ContentType:  "application/json",
		Data:         document,
		Metadata:     map[string]string{"artifact_type": auditArtifactType, "event_id": strconv.FormatInt(event.ID, 10), "event_hash": event.Hash},
	})
	if err != nil || receipt == nil {
		return nil, common.NewErrServiceUnavailable(fmt.Sprintf("REBAC-AUDIT-EVIDENCEPUT audit evidence could not be stored: %v", err))
	}
	receiptJSON, err := common.CanonicalJSON(receipt)
	if err != nil {
		return nil, fmt.Errorf("REBAC-AUDIT-RECEIPT: %w", err)
	}
	return receiptJSON, nil
}

func auditObjectKey(event AuditEvent) string {
	return path.Join("rebac-audit", fmt.Sprintf("%020d-%s.json", event.ID, event.Hash))
}

func execSelect(ctx context.Context, q Queryer, code string, ds *goqu.SelectDataset) (bool, error) {
	var ignored any
	return queryRowDataset(ctx, q, code, ds, &ignored)
}

// ListAuditEvents returns up to limit events with an id greater than afterID.
func ListAuditEvents(ctx context.Context, q Queryer, afterID int64, limit uint) ([]AuditEvent, error) {
	events := []AuditEvent{}
	ds := auditEventsQuery().Where(goqu.C("id").Gt(afterID)).Order(goqu.C("id").Asc()).Limit(limit)
	err := streamAuditEvents(ctx, q, "REBAC-LISTAUDIT", ds, func(event AuditEvent) error {
		events = append(events, event)
		return nil
	})
	return events, err
}

func auditEventsQuery() *goqu.SelectDataset {
	return dialect.From(auditTable).Select(
		goqu.C("id"), goqu.C("occurred_at"), goqu.C("event_type"), goqu.C("actor_key"), goqu.C("object_key"),
		goqu.L("details::text"), goqu.L("COALESCE(previous_hash, '')"), goqu.C("event_hash"),
		goqu.L("COALESCE(evidence_receipt::text, '')"),
	)
}

// streamAuditEvents passes the events selected by ds to visit in order.
func streamAuditEvents(ctx context.Context, q Queryer, code string, ds *goqu.SelectDataset, visit func(AuditEvent) error) error {
	rows, err := queryDataset(ctx, q, code, ds)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var event AuditEvent
		var details, evidence string
		if err = rows.Scan(&event.ID, &event.OccurredAt, &event.Type, &event.Actor, &event.Object,
			&details, &event.PreviousHash, &event.Hash, &evidence); err != nil {
			return fmt.Errorf("%s-SCAN: %w", code, err)
		}
		event.OccurredAt = event.OccurredAt.UTC()
		event.Details = json.RawMessage(details)
		if evidence != "" {
			event.Evidence = json.RawMessage(evidence)
		}
		if err = visit(event); err != nil {
			return err
		}
	}
	return rows.Err()
}

// AuditVerification reports the result of verifying the audit trail.
type AuditVerification struct {
	Valid            bool   `json:"valid"`
	Checked          int    `json:"checked"`
	HeadHash         string `json:"headHash,omitempty"`
	FirstInvalidID   int64  `json:"firstInvalidId,omitempty"`
	Reason           string `json:"reason,omitempty"`
	EvidenceVerified int    `json:"evidenceVerified"`
	EvidenceMissing  int    `json:"evidenceMissing"`
}

// VerifyAuditTrail recomputes the hash chain of every audit event. With a
// store it also verifies each archived event against the WORM object. A
// non-empty expectedHead, retained independently of the database, detects
// removed trailing events.
func VerifyAuditTrail(ctx context.Context, q Queryer, store history.EvidenceStore, expectedHead string) (AuditVerification, error) {
	report := AuditVerification{Valid: true}
	err := streamAuditEvents(ctx, q, "REBAC-VERIFYAUDIT", auditEventsQuery().Order(goqu.C("id").Asc()), func(event AuditEvent) error {
		if !report.Valid {
			return nil
		}
		reason, err := verifyAuditEvent(ctx, store, event, report.HeadHash, &report)
		if err != nil {
			return err
		}
		report.Checked++
		if reason != "" {
			report.Valid, report.FirstInvalidID, report.Reason = false, event.ID, reason
			return nil
		}
		report.HeadHash = event.Hash
		return nil
	})
	if err == nil && report.Valid && expectedHead != "" && expectedHead != report.HeadHash {
		report.Valid, report.Reason = false, "the head of the trail differs from the expected head hash"
	}
	return report, err
}

func verifyAuditEvent(ctx context.Context, store history.EvidenceStore, event AuditEvent, previous string, report *AuditVerification) (string, error) {
	if event.PreviousHash != previous {
		return "the event does not follow its predecessor", nil
	}
	recomputed, err := event.computeHash()
	if err != nil {
		return "", err
	}
	if recomputed != event.Hash {
		return "the event content does not match its hash", nil
	}
	if store == nil {
		return "", nil
	}
	if len(event.Evidence) == 0 {
		report.EvidenceMissing++
		return "", nil
	}
	if reason := verifyAuditEvidence(ctx, store, event); reason != "" {
		return reason, nil
	}
	report.EvidenceVerified++
	return "", nil
}

func verifyAuditEvidence(ctx context.Context, store history.EvidenceStore, event AuditEvent) string {
	var receipt history.EvidenceReceipt
	if err := json.Unmarshal(event.Evidence, &receipt); err != nil {
		return "the evidence receipt is malformed"
	}
	document, err := event.evidenceDocument()
	if err != nil {
		return "the event cannot be rendered as evidence"
	}
	expected := history.SHA256Hex(document)
	if receipt.SHA256 != expected {
		return "the evidence receipt does not match the event"
	}
	if _, err = store.VerifyArtifact(ctx, receipt.Reference, expected); err != nil {
		return "the archived evidence does not match the event"
	}
	return ""
}
