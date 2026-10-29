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

package aol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/conditional"
)

// Plan is a durable, immutable proposal with an optimistic execution journal.
type Plan struct {
	TemplateHashes  map[string]string `json:"template_hashes,omitempty"`
	ID              string            `json:"id"`
	Subject         string            `json:"subject"`
	Issuer          string            `json:"issuer"`
	SessionID       string            `json:"agent.session_id"`
	Hash            string            `json:"payload_hash"`
	Draft           Draft             `json:"draft"`
	Status          string            `json:"status"`
	Version         int64             `json:"version"`
	Expires         time.Time         `json:"expires"`
	Updated         time.Time         `json:"updated"`
	ApprovalHash    string            `json:"-"`
	ApprovalAction  string            `json:"-"`
	ApprovalExpires time.Time         `json:"-"`
	Steps           []Step            `json:"steps"`
	Failure         string            `json:"failure,omitempty"`
}

// Step records resources that this plan created, including committed revision receipts.
type Step struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	ETag   string `json:"etag,omitempty"`
	Status string `json:"status"`
}

// Store persists approval transitions and resource receipts across restarts.
type Store interface {
	Create(context.Context, *Plan) error
	Get(context.Context, string) (*Plan, error)
	Update(context.Context, *Plan, int64) error
	Receipts(context.Context, string) ([]Step, error)
}

// SQLStore uses GOQU for every database statement.
type SQLStore struct{ DB *sql.DB }
type storedPlan Plan

type planDocument struct {
	Plan            storedPlan `json:"plan"`
	ApprovalHash    string     `json:"approval_hash"`
	ApprovalAction  string     `json:"approval_action"`
	ApprovalExpires time.Time  `json:"approval_expires"`
}

func encodePlan(p *Plan) (string, error) {
	raw, err := json.Marshal(planDocument{Plan: storedPlan(*p), ApprovalHash: p.ApprovalHash, ApprovalAction: p.ApprovalAction, ApprovalExpires: p.ApprovalExpires})
	return string(raw), err
}

// Create stores a newly validated proposal.
func (s SQLStore) Create(ctx context.Context, p *Plan) error {
	next := *p
	next.Version = 1
	next.Updated = time.Now().UTC()
	document, err := encodePlan(&next)
	if err != nil {
		return err
	}
	query, args, err := goqu.Dialect("postgres").Insert("aol_plan").Rows(goqu.Record{"id": next.ID, "issuer": next.Issuer, "subject": next.Subject, "version": next.Version, "status": next.Status, "updated_at": next.Updated, "document": document}).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AOL-STORE-BUILD: %w", err)
	}
	if err = s.transition(ctx, &next, query, args); err != nil {
		return err
	}
	*p = next
	return nil
}

// Get loads state; authorization is enforced by the operation layer before returning it.
func (s SQLStore) Get(ctx context.Context, id string) (*Plan, error) {
	q, args, err := goqu.Dialect("postgres").From("aol_plan").Select("document").Where(goqu.Ex{"id": id}).Prepared(true).ToSQL()
	if err != nil {
		return nil, err
	}
	var raw []byte
	if err = s.DB.QueryRowContext(ctx, q, args...).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, Failure("AOL-PLAN-NOTFOUND", "plan not found", "Use a plan owned by the current user.")
		}
		return nil, fmt.Errorf("AOL-STORE-GET: %w", err)
	}
	var doc planDocument
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("AOL-STORE-DECODE: %w", err)
	}
	p := Plan(doc.Plan)
	p.ApprovalHash = doc.ApprovalHash
	p.ApprovalAction = doc.ApprovalAction
	p.ApprovalExpires = doc.ApprovalExpires
	return &p, nil
}

// Update uses compare-and-swap so concurrent calls cannot reuse an approval.
func (s SQLStore) Update(ctx context.Context, p *Plan, version int64) error {
	next := *p
	next.Version = version + 1
	next.Updated = time.Now().UTC()
	document, err := encodePlan(&next)
	if err != nil {
		return err
	}
	query, args, err := goqu.Dialect("postgres").Update("aol_plan").Set(goqu.Record{"version": next.Version, "status": next.Status, "updated_at": next.Updated, "document": document}).Where(goqu.Ex{"id": next.ID, "version": version}).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AOL-STORE-BUILD: %w", err)
	}
	if err = s.transition(ctx, &next, query, args); err != nil {
		return err
	}
	*p = next
	return nil
}
func (s SQLStore) transition(ctx context.Context, p *Plan, query string, args []any) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AOL-STORE-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("AOL-STORE-TRANSITION: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("AOL-STORE-COUNT: %w", err)
	}
	if count != 1 {
		return Failure("AOL-PLAN-CONFLICT", "plan changed concurrently", "Reload the changeset before retrying.")
	}
	if err = appendAuditEvent(ctx, tx, p); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("AOL-STORE-COMMIT: %w", err)
	}
	return nil
}
func appendAuditEvent(ctx context.Context, tx *sql.Tx, p *Plan) error {
	identity := IdentityFromContext(ctx)
	audit, err := json.Marshal(map[string]string{"agent.session_id": identity.SessionID, "aol.plan_id": p.ID, "mcp.tool": toolFromContext(ctx), "subject": identity.Subject, "issuer": identity.Issuer, "status": p.Status})
	if err != nil {
		return fmt.Errorf("AOL-AUDIT-ENCODE: %w", err)
	}
	query, args, err := goqu.Dialect("postgres").Insert("aol_event").Rows(goqu.Record{"plan_id": p.ID, "audit": string(audit)}).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AOL-AUDIT-BUILD: %w", err)
	}
	_, err = tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("AOL-AUDIT-INSERT: %w", err)
	}
	return nil
}

// Receipts loads resource revisions recorded in the same transaction as each mutation.
func (s SQLStore) Receipts(ctx context.Context, id string) ([]Step, error) {
	q, args, err := goqu.Dialect("postgres").From("aol_receipt").Select("kind", "identifier", "revision").Where(goqu.Ex{"plan_id": id}).Order(goqu.C("sequence").Asc()).Prepared(true).ToSQL()
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("AOL-STORE-RECEIPTS: %w", err)
	}
	defer func() { _ = rows.Close() }()
	steps := []Step{}
	for rows.Next() {
		var step Step
		var revision int64
		if err = rows.Scan(&step.Kind, &step.ID, &revision); err != nil {
			return nil, err
		}
		kind := conditional.KindSubmodel
		if step.Kind == "shells" {
			kind = conditional.KindAAS
		}
		step.ETag = conditional.ConcurrencyETag(conditional.Ref(kind, step.ID), revision)
		step.Status = "created"
		steps = append(steps, step)
	}
	return steps, rows.Err()
}
