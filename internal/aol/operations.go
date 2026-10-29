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
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/conditional"
	"github.com/google/uuid"
)

// Operations enforces immutable plans, explicit approval and conditional compensation.
type Operations struct {
	Preflight func(context.Context, Draft) error
	Store     Store
	Validator Validator
	Dispatch  Dispatcher
	TTL       time.Duration
	PublicURL string
	Lease     time.Duration
}

// PlanCreate validates before persisting a proposal. It cannot authorize execution.
func (o Operations) PlanCreate(ctx context.Context, draft Draft) (*Plan, error) {
	identity := IdentityFromContext(ctx)
	if identity.Subject == "" {
		return nil, Failure("AOL-PLAN-IDENTITY", "writes require an authenticated user", "Sign in before planning a write.")
	}
	report := o.Validator.Validate(draft)
	if !report.Valid {
		raw, _ := json.Marshal(report)
		return nil, Failure("AOL-PLAN-VALIDATE", string(raw), "Correct all L1/L2 issues before creating a plan.")
	}
	if o.Preflight != nil {
		if err := o.Preflight(ctx, draft); err != nil {
			return nil, err
		}
	}
	normalizeReferences(&draft)
	hashes, err := o.templateHashes(draft)
	if err != nil {
		return nil, err
	}
	hash, err := proposalHash(draft, hashes)
	if err != nil {
		return nil, err
	}
	p := &Plan{ID: uuid.NewString(), Subject: identity.Subject, Issuer: identity.Issuer, SessionID: identity.SessionID, Hash: hash, TemplateHashes: hashes, Draft: draft, Status: "planned", Expires: time.Now().UTC().Add(o.TTL)}
	for _, sm := range draft.Submodels {
		p.Steps = append(p.Steps, Step{Kind: "submodels", ID: stringField(sm, "id"), Status: "planned"})
	}
	p.Steps = append(p.Steps, Step{Kind: "shells", ID: stringField(draft.Shell, "id"), Status: "planned"})
	if err = o.Store.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}
func normalizeReferences(d *Draft) {
	refs := []any{}
	for _, sm := range d.Submodels {
		refs = append(refs, map[string]any{"type": "ModelReference", "keys": []any{map[string]any{"type": "Submodel", "value": stringField(sm, "id")}}})
	}
	d.Shell["submodels"] = refs
}

// Owned returns state only to the original issuer and subject.
func (o Operations) Owned(ctx context.Context, id string) (*Plan, error) {
	p, err := o.Store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	i := IdentityFromContext(ctx)
	if i.Subject == "" || i.Subject != p.Subject || i.Issuer != p.Issuer {
		return nil, Failure("AOL-PLAN-NOTFOUND", "plan not found", "Use a plan owned by the current user.")
	}
	if (p.Status == "applying" || p.Status == "rolling_back") && o.Lease > 0 && time.Since(p.Updated) > o.Lease {
		p.Status = "recovery_required"
		p.Failure = "execution lease expired; inspect committed receipts before retrying rollback"
		if err = o.Store.Update(ctx, p, p.Version); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// Approve is invoked exclusively by the authenticated human confirmation HTTP flow.
func (o Operations) Approve(ctx context.Context, id, action string) (string, error) {
	p, err := o.Owned(ctx, id)
	if err != nil {
		return "", err
	}
	if !approvalState(p, action) {
		return "", Failure("AOL-CONFIRM-STATE", "plan cannot be approved in its current state", "Reload the changeset.")
	}
	if action == "apply" && time.Now().After(p.Expires) {
		return "", Failure("AOL-CONFIRM-EXPIRED", "plan expired", "Create and review a new plan.")
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	p.ApprovalHash = approvalHash(p, token, action)
	p.ApprovalAction = action
	p.ApprovalExpires = time.Now().Add(o.TTL)
	if action == "apply" && p.Expires.Before(p.ApprovalExpires) {
		p.ApprovalExpires = p.Expires
	}
	if err = o.Store.Update(ctx, p, p.Version); err != nil {
		return "", err
	}
	return token, nil
}
func approvalState(p *Plan, action string) bool {
	if action == "apply" {
		return p.Status == "planned"
	}
	if action != "rollback" {
		return false
	}
	return p.Status == "applied" || p.Status == "recovery_required" || p.Status == "conflict"
}
func randomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("AOL-CONFIRM-RANDOM: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
func approvalHash(p *Plan, token, action string) string {
	raw, _ := json.Marshal([]string{p.ID, p.Issuer, p.Subject, p.Hash, action, token})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (o Operations) consume(ctx context.Context, id, token, action string) (*Plan, error) {
	p, err := o.Owned(ctx, id)
	if err != nil {
		return nil, err
	}
	expected := approvalHash(p, token, action)
	if !approvalState(p, action) || p.ApprovalAction != action || time.Now().After(p.ApprovalExpires) || subtle.ConstantTimeCompare([]byte(expected), []byte(p.ApprovalHash)) != 1 {
		return nil, Failure("AOL-APPLY-CONFIRM", "confirmation is absent, expired, used or mismatched", "Review the plan and obtain a new human confirmation token.")
	}
	hash, err := proposalHash(p.Draft, p.TemplateHashes)
	if err != nil {
		return nil, err
	}
	if hash != p.Hash {
		return nil, Failure("AOL-APPLY-HASH", "stored proposal hash changed", "Create a new plan.")
	}
	p.ApprovalHash = ""
	p.ApprovalAction = ""
	p.Status = "applying"
	if action == "rollback" {
		p.Status = "rolling_back"
	}
	if err = o.Store.Update(ctx, p, p.Version); err != nil {
		return nil, err
	}
	return p, nil
}

// Apply consumes one approval, creates only absent resources, then verifies each write.
func (o Operations) Apply(ctx context.Context, id, token string) (*Plan, error) {
	ctx = common.WithWriterPostgresReads(ctx)
	p, err := o.consume(ctx, id, token, "apply")
	if err != nil {
		return nil, err
	}
	if err = o.checkTemplates(p); err != nil {
		return o.failed(ctx, p, err)
	}
	if report := o.Validator.Validate(p.Draft); !report.Valid {
		return o.failed(ctx, p, Failure("AOL-APPLY-REVALIDATE", "template validation changed", "Create a new plan."))
	}
	if o.Preflight != nil {
		if err = o.Preflight(ctx, p.Draft); err != nil {
			return o.failed(ctx, p, err)
		}
	}
	for _, step := range p.Steps {
		if err = o.createStep(ctx, p, step); err != nil {
			return o.failed(ctx, p, err)
		}
	}
	p.Status = "applied"
	if err = o.Store.Update(ctx, p, p.Version); err != nil {
		return nil, err
	}
	return o.hydrate(ctx, p)
}
func (o Operations) createStep(ctx context.Context, p *Plan, step Step) error {
	payload := stepPayload(p, step)
	ctx = conditional.WithRevisionObserver(ctx, o.receiptObserver(p, step))
	// Collection POST provides the repository's atomic duplicate-ID rejection.
	response, err := o.Dispatch.Do(ctx, "POST", "/"+step.Kind, payload, nil)
	if err != nil {
		return err
	}
	if response.Status != http.StatusCreated {
		return Failure("AOL-APPLY-CREATE", "repository did not acknowledge a new resource", "Inspect the changeset before retrying.")
	}
	receipts, err := o.Store.Receipts(ctx, p.ID)
	if err != nil {
		return err
	}
	expected := receiptETag(receipts, step)
	if expected == "" {
		return Failure("AOL-APPLY-RECEIPT", "resource has no durable revision receipt", "Inspect the changeset; automatic deletion is disabled.")
	}
	read, err := o.Dispatch.Do(ctx, "GET", resourcePath(step.Kind, step.ID), nil, nil)
	if err != nil {
		return err
	}
	if !conditionalTagMatches(expected, read.Header.Get("ETag")) {
		return Failure("AOL-APPLY-VERIFY", "resource changed during verification", "Inspect the conflicting resource.")
	}
	var actual map[string]any
	if err = json.Unmarshal(read.Body, &actual); err != nil {
		return err
	}
	if !containsDraft(actual, payload) {
		return Failure("AOL-APPLY-VERIFY", "stored content differs from the proposal", "Inspect the changeset.")
	}
	return nil
}
func stepPayload(p *Plan, step Step) map[string]any {
	if step.Kind != "submodels" {
		return p.Draft.Shell
	}
	for _, sm := range p.Draft.Submodels {
		if stringField(sm, "id") == step.ID {
			return sm
		}
	}
	return nil
}
func receiptETag(receipts []Step, step Step) string {
	for _, receipt := range receipts {
		if receipt.Kind == step.Kind && receipt.ID == step.ID {
			return receipt.ETag
		}
	}
	return ""
}
func containsDraft(actual, expected any) bool {
	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		return ok && containsMap(a, e)
	case []any:
		a, ok := actual.([]any)
		return ok && containsSlice(a, e)
	default:
		a, _ := json.Marshal(actual)
		b, _ := json.Marshal(expected)
		return string(a) == string(b)
	}
}
func containsMap(actual, expected map[string]any) bool {
	for key, value := range expected {
		if !containsDraft(actual[key], value) {
			return false
		}
	}
	return true
}
func containsSlice(actual, expected []any) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i, value := range expected {
		if !containsDraft(actual[i], value) {
			return false
		}
	}
	return true
}

func conditionalTagMatches(expected, actual string) bool {
	if expected == "" {
		return false
	}
	return actual == expected || (len(actual) > len(expected) && actual[:len(expected)-1] == expected[:len(expected)-1] && actual[len(expected)-1] == '-')
}
func (o Operations) receiptObserver(p *Plan, step Step) conditional.RevisionObserver {
	return func(ctx context.Context, tx *sql.Tx, revisions map[conditional.ResourceRef]int64) error {
		kind := conditional.KindSubmodel
		if step.Kind == "shells" {
			kind = conditional.KindAAS
		}
		revision, ok := revisions[conditional.Ref(kind, step.ID)]
		if !ok {
			return nil
		}
		audit, _ := json.Marshal(map[string]string{"agent.session_id": p.SessionID, "aol.plan_id": p.ID, "mcp.tool": "apply_plan", "subject": p.Subject, "issuer": p.Issuer})
		q, args, err := goqu.Dialect("postgres").Insert("aol_receipt").Rows(goqu.Record{"plan_id": p.ID, "kind": step.Kind, "identifier": step.ID, "revision": revision, "audit": string(audit)}).Prepared(true).ToSQL()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("AOL-RECEIPT-INSERT: %w", err)
		}
		return nil
	}
}
func (o Operations) failed(ctx context.Context, p *Plan, cause error) (*Plan, error) {
	// The derived context retains identity and has a bounded cleanup deadline even if the client disconnects.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	p.Failure = cause.Error()
	p.Status = "recovery_required"
	compensationErr := o.compensate(cleanup, p)
	if compensationErr == nil {
		p.Status = "compensated"
	} else {
		p.Failure += "; " + compensationErr.Error()
	}
	saveErr := o.Store.Update(cleanup, p, p.Version)
	if saveErr == nil {
		if inspected, inspectErr := o.Inspect(cleanup, p.ID); inspectErr == nil {
			p = inspected
		}
	}
	return p, errors.Join(cause, compensationErr, saveErr)
}

// Rollback validates every revision before deleting resources in reverse creation order.
func (o Operations) Rollback(ctx context.Context, id, token string) (*Plan, error) {
	ctx = common.WithWriterPostgresReads(ctx)
	p, err := o.consume(ctx, id, token, "rollback")
	if err != nil {
		return nil, err
	}
	err = o.compensate(ctx, p)
	p.Status = "rolled_back"
	if err != nil {
		p.Status = "conflict"
		p.Failure = err.Error()
	}
	saveErr := o.Store.Update(ctx, p, p.Version)
	if saveErr == nil {
		if inspected, inspectErr := o.Inspect(ctx, p.ID); inspectErr == nil {
			p = inspected
		}
	}
	return p, errors.Join(err, saveErr)
}
func (o Operations) compensate(ctx context.Context, p *Plan) error {
	receipts, err := o.Store.Receipts(ctx, p.ID)
	if err != nil {
		return err
	}
	for _, step := range receipts {
		response, readErr := o.Dispatch.Do(ctx, "GET", resourcePath(step.Kind, step.ID), nil, nil)
		if isNotFound(readErr) {
			continue
		}
		if readErr != nil {
			return readErr
		}
		if !conditionalTagMatches(step.ETag, response.Header.Get("ETag")) {
			return Failure("AOL-ROLLBACK-CONFLICT", "a created resource changed after creation", "Resolve the conflict manually; no further resources were deleted.")
		}
	}
	for i := len(receipts) - 1; i >= 0; i-- {
		step := receipts[i]
		_, err = o.Dispatch.Do(ctx, "DELETE", resourcePath(step.Kind, step.ID), nil, http.Header{"If-Match": {step.ETag}})
		if err != nil && !isNotFound(err) {
			return err
		}
	}
	return nil
}
func isNotFound(err error) bool {
	var failure *Error
	return errors.As(err, &failure) && failure.Status == 404
}
func (o Operations) hydrate(ctx context.Context, p *Plan) (*Plan, error) {
	steps, err := o.Store.Receipts(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Steps = steps
	return p, nil
}

func (o Operations) templateHashes(draft Draft) (map[string]string, error) {
	hashes := map[string]string{}
	for _, slug := range draft.Templates {
		hash, err := common.CanonicalJSONHash(o.Validator.Templates[slug])
		if err != nil {
			return nil, err
		}
		hashes[slug] = hash
	}
	return hashes, nil
}
func proposalHash(draft Draft, hashes map[string]string) (string, error) {
	if len(hashes) == 0 {
		return common.CanonicalJSONHash(draft)
	}
	return common.CanonicalJSONHash(map[string]any{"draft": draft, "templates": hashes})
}
func (o Operations) checkTemplates(p *Plan) error {
	hashes, err := o.templateHashes(p.Draft)
	if err != nil {
		return err
	}
	for slug, hash := range p.TemplateHashes {
		if hashes[slug] != hash {
			return Failure("AOL-APPLY-TEMPLATECHANGED", "a qualified template changed after planning", "Create and approve a new plan.")
		}
	}
	return nil
}

// Inspect merges the durable revision receipts into the user-visible execution journal.
func (o Operations) Inspect(ctx context.Context, id string) (*Plan, error) {
	p, err := o.Owned(ctx, id)
	if err != nil {
		return nil, err
	}
	receipts, err := o.Store.Receipts(ctx, id)
	if err != nil {
		return nil, err
	}
	for i, step := range p.Steps {
		if tag := receiptETag(receipts, step); tag != "" {
			p.Steps[i].ETag = tag
			p.Steps[i].Status = "created"
			if p.Status == "rolled_back" || p.Status == "compensated" {
				p.Steps[i].Status = "deleted"
			}
		}
	}
	return p, nil
}
