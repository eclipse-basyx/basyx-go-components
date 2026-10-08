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

package conditional

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"sync"
	"weak"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// Operation describes how a write changes a resource.
type Operation int

// Write operations.
const (
	// OpCreate creates a resource that did not exist before.
	OpCreate Operation = iota + 1
	// OpUpdate changes an existing resource, including replacements.
	OpUpdate
	// OpNoOp writes an existing resource without changing it.
	OpNoOp
	// OpDelete deletes an existing resource.
	OpDelete
)

var (
	errMultipleTransactions = errors.New("COMMON-CONDREQ-MULTITX a conditional request changed its target in more than one transaction")
	errNotEvaluated         = errors.New("COMMON-CONDREQ-NOTEVALUATED a conditional request changed resources without evaluating its target precondition")
)

type touchState struct {
	existedBefore bool
	existsAfter   bool
	changed       bool
}

func newTouchState(op Operation) *touchState {
	state := &touchState{existedBefore: op != OpCreate}
	state.apply(op)
	return state
}

// needsRevision reports whether the resource needs a new revision. Deleted
// resources keep their last revision as a tombstone: revisions are never
// reused, so a recreated resource still gets a revision no client has seen.
func (t *touchState) needsRevision() bool {
	return t.existsAfter && t.changed
}

func (t *touchState) apply(op Operation) {
	switch op {
	case OpDelete:
		t.existsAfter = false
		t.changed = true
	case OpNoOp:
		t.existsAfter = true
	default:
		t.existsAfter = true
		t.changed = true
	}
}

type compositeTouch struct {
	ref    ResourceRef
	before []ResourceRef
	after  []ResourceRef
	op     Operation
}

// pendingTx collects the revision changes of one transaction until commit.
type pendingTx struct {
	ctx       context.Context
	state     *State
	touches   map[ResourceRef]*touchState
	composite *compositeTouch
}

// registry holds the pending changes per transaction. Keys are weak, so
// entries of transactions that are rolled back without Discard disappear
// with the transaction.
var registry = struct {
	sync.Mutex
	pending map[weak.Pointer[sql.Tx]]*pendingTx
}{pending: map[weak.Pointer[sql.Tx]]*pendingTx{}}

// Touch records that the transaction changes a resource. The revision change
// is written at commit (see FlushTx), after all other locks of the
// transaction and in a fixed order.
func Touch(ctx context.Context, tx *sql.Tx, ref ResourceRef, op Operation) error {
	if tx == nil || ref.Identifier == "" {
		return nil
	}
	pendingFor(ctx, tx).touch(ref, op)
	return nil
}

// TouchComposite records a write of a composite resource whose revision is
// derived from its members, such as a DPP. before are the members the
// precondition is evaluated against, after the members once the write is
// done. The members' own changes are recorded with Touch.
func TouchComposite(ctx context.Context, tx *sql.Tx, ref ResourceRef, before []ResourceRef, after []ResourceRef, op Operation) {
	if tx == nil {
		return
	}
	pending := pendingFor(ctx, tx)
	registry.Lock()
	defer registry.Unlock()
	pending.composite = &compositeTouch{
		ref:    ref,
		before: append([]ResourceRef(nil), before...),
		after:  append([]ResourceRef(nil), after...),
		op:     op,
	}
}

func pendingFor(ctx context.Context, tx *sql.Tx) *pendingTx {
	key := weak.Make(tx)
	registry.Lock()
	defer registry.Unlock()
	pending, ok := registry.pending[key]
	if !ok {
		pending = &pendingTx{ctx: ctx, state: stateFromContext(ctx), touches: map[ResourceRef]*touchState{}}
		registry.pending[key] = pending
		runtime.AddCleanup(tx, forget, key)
	}
	return pending
}

func forget(key weak.Pointer[sql.Tx]) {
	registry.Lock()
	defer registry.Unlock()
	delete(registry.pending, key)
}

func (p *pendingTx) touch(ref ResourceRef, op Operation) {
	registry.Lock()
	defer registry.Unlock()
	if existing, ok := p.touches[ref]; ok {
		existing.apply(op)
		return
	}
	p.touches[ref] = newTouchState(op)
}

// Discard drops the pending revision changes of a transaction that is rolled
// back.
func Discard(tx *sql.Tx) {
	if tx != nil {
		forget(weak.Make(tx))
	}
}

func takePending(tx *sql.Tx) *pendingTx {
	key := weak.Make(tx)
	registry.Lock()
	defer registry.Unlock()
	pending := registry.pending[key]
	delete(registry.pending, key)
	return pending
}

// Flushed is the outcome of FlushTx. Call Committed after the transaction
// committed.
type Flushed struct {
	state     *State
	evaluated bool
	writeETag string
}

// Committed records the outcome in the request state.
func (f *Flushed) Committed() {
	if f == nil || f.state == nil {
		return
	}
	f.state.recordCommit(f.evaluated, f.writeETag)
}

// FlushTx writes the pending revision changes of tx and evaluates the
// request's target precondition. It must run immediately before commit; on
// error the transaction must be rolled back.
func FlushTx(tx *sql.Tx) (*Flushed, error) {
	pending := takePending(tx)
	if pending == nil {
		return nil, nil
	}
	return pending.flush(tx)
}

func (p *pendingTx) flush(tx *sql.Tx) (*Flushed, error) {
	evaluate := p.state.evaluatesWrites()
	if evaluate {
		if err := p.checkEvaluable(); err != nil {
			return nil, err
		}
	}
	locked := p.lockedRefs(evaluate)
	previous, revisions, err := p.writePlain(tx, locked)
	if err != nil {
		return nil, err
	}
	if evaluate {
		if err = p.evaluate(previous); err != nil {
			p.state.setFailure(err)
			return nil, err
		}
	}
	if err = p.writeLocked(tx, locked, revisions); err != nil {
		return nil, err
	}
	return &Flushed{state: p.state, evaluated: evaluate, writeETag: p.writeETag(previous, revisions)}, nil
}

func (p *pendingTx) touchesTarget() bool {
	target := *p.state.target
	if p.composite != nil && p.composite.ref == target {
		return true
	}
	_, ok := p.touches[target]
	return ok
}

func (p *pendingTx) checkEvaluable() error {
	if len(p.touches) == 0 && p.composite == nil {
		return nil
	}
	if !p.touchesTarget() {
		return errNotEvaluated
	}
	if p.state.isEvaluated() {
		return errMultipleTransactions
	}
	return nil
}

// lockedRefs returns the resources whose previous revision is needed.
func (p *pendingTx) lockedRefs(evaluate bool) map[ResourceRef]bool {
	locked := map[ResourceRef]bool{}
	if !evaluate {
		return locked
	}
	if p.composite != nil {
		for _, member := range append(append([]ResourceRef(nil), p.composite.before...), p.composite.after...) {
			locked[member] = true
		}
		return locked
	}
	locked[*p.state.target] = true
	return locked
}

// writePlain walks all resources in sorted order. It locks the resources
// whose previous revision is needed and writes the new revisions of all
// other changed resources.
func (p *pendingTx) writePlain(tx *sql.Tx, locked map[ResourceRef]bool) (map[ResourceRef]int64, map[ResourceRef]int64, error) {
	previous := map[ResourceRef]int64{}
	revisions := map[ResourceRef]int64{}
	var upserts []ResourceRef
	for _, ref := range p.plan(locked) {
		if !locked[ref] {
			if p.touches[ref].needsRevision() {
				upserts = append(upserts, ref)
			}
			continue
		}
		if err := upsertRevisions(p.ctx, tx, upserts, revisions); err != nil {
			return nil, nil, err
		}
		upserts = nil
		revision, err := lockRevision(p.ctx, tx, ref)
		if err != nil {
			return nil, nil, err
		}
		previous[ref] = revision
	}
	if err := upsertRevisions(p.ctx, tx, upserts, revisions); err != nil {
		return nil, nil, err
	}
	return previous, revisions, nil
}

func (p *pendingTx) plan(locked map[ResourceRef]bool) []ResourceRef {
	refs := make([]ResourceRef, 0, len(p.touches)+len(locked))
	for ref := range p.touches {
		refs = append(refs, ref)
	}
	for ref := range locked {
		if _, touched := p.touches[ref]; !touched {
			refs = append(refs, ref)
		}
	}
	return sortedRefs(refs)
}

// writeLocked writes the revisions of resources that were locked.
func (p *pendingTx) writeLocked(tx *sql.Tx, locked map[ResourceRef]bool, revisions map[ResourceRef]int64) error {
	for _, ref := range sortedLocked(locked) {
		touch, touched := p.touches[ref]
		if !touched || !touch.needsRevision() {
			continue
		}
		revision, err := bumpRevision(p.ctx, tx, ref)
		if err != nil {
			return err
		}
		revisions[ref] = revision
	}
	return nil
}

func sortedLocked(locked map[ResourceRef]bool) []ResourceRef {
	refs := make([]ResourceRef, 0, len(locked))
	for ref := range locked {
		refs = append(refs, ref)
	}
	return sortedRefs(refs)
}

func (p *pendingTx) evaluate(previous map[ResourceRef]int64) error {
	if p.composite != nil {
		target := newWriteTarget(p.composite.op != OpCreate, compositeValidator(p.composite.ref, p.composite.before, previous), nil)
		return evaluateWrite(p.state.conds, target, p.state.requireIfMatch)
	}
	ref := *p.state.target
	revision := previous[ref]
	target := newWriteTarget(p.touches[ref].existedBefore, concurrencyValidator(ref, revision), p.state.addressedExistence())
	return evaluateWrite(p.state.conds, target, p.state.requireIfMatch)
}

// writeETag returns the entity tag of the request target after the write.
func (p *pendingTx) writeETag(previous map[ResourceRef]int64, revisions map[ResourceRef]int64) string {
	if p.state == nil {
		return ""
	}
	current := func(ref ResourceRef) (int64, bool) {
		if revision, ok := revisions[ref]; ok {
			return revision, true
		}
		revision, ok := previous[ref]
		return revision, ok
	}
	if p.compositeReturnsETag() {
		return p.compositeWriteETag(current)
	}
	if p.state.target != nil {
		return p.targetWriteETag(*p.state.target, current)
	}
	return p.createdWriteETag(revisions)
}

// compositeReturnsETag reports whether the response of a composite write
// carries the composite's entity tag: for writes of the target and for
// creates through a collection.
func (p *pendingTx) compositeReturnsETag() bool {
	if p.composite == nil || p.composite.op == OpDelete {
		return false
	}
	if p.state.isTarget(p.composite.ref) {
		return true
	}
	return p.state.mode == ModeCollection && p.composite.op == OpCreate && p.state.kind == p.composite.ref.Kind
}

func (p *pendingTx) compositeWriteETag(current func(ResourceRef) (int64, bool)) string {
	merged := map[ResourceRef]int64{}
	for _, member := range p.composite.after {
		revision, ok := current(member)
		if !ok {
			return ""
		}
		merged[member] = revision
	}
	return quote(compositeValidator(p.composite.ref, p.composite.after, merged))
}

func (p *pendingTx) targetWriteETag(target ResourceRef, current func(ResourceRef) (int64, bool)) string {
	touch, touched := p.touches[target]
	if !touched || !touch.existsAfter {
		return ""
	}
	revision, ok := current(target)
	if !ok {
		return ""
	}
	return ConcurrencyETag(target, revision)
}

// createdWriteETag returns the tag of the one resource a collection POST created.
func (p *pendingTx) createdWriteETag(revisions map[ResourceRef]int64) string {
	if p.state.mode != ModeCollection || p.state.kind == "" {
		return ""
	}
	created := ""
	for ref, touch := range p.touches {
		if ref.Kind != p.state.kind || touch.existedBefore || !touch.existsAfter {
			continue
		}
		if created != "" {
			return ""
		}
		created = ConcurrencyETag(ref, revisions[ref])
	}
	return created
}

// ObserveReadTx records the revision of the request target as the first
// statement of a read transaction, so the entity tag belongs to the same
// snapshot as the representation.
func ObserveReadTx(ctx context.Context, tx *sql.Tx) error {
	state := stateFromContext(ctx)
	if !state.observesReads() || state.target.Kind == KindDPP {
		return nil
	}
	revision, err := readRevision(ctx, tx, *state.target)
	if err != nil {
		return err
	}
	state.observe(revision)
	return nil
}

// ObserveComposite records the validator of a composite request target, such
// as a DPP, from the revisions of its members.
func ObserveComposite(ctx context.Context, q Queryer, ref ResourceRef, members []ResourceRef) error {
	state := stateFromContext(ctx)
	if !state.observesReads() || !state.isTarget(ref) {
		return nil
	}
	revisions, err := readRevisions(ctx, q, members)
	if err != nil {
		return err
	}
	state.observeComposite(compositeValidator(ref, members, revisions))
	return nil
}

// VerifyTarget evaluates If-Match and If-None-Match of an action that does
// not change the target, such as an operation invocation, against the
// committed revision. It guarantees the revision only at the time of the
// call.
func VerifyTarget(ctx context.Context, q Queryer) error {
	state := stateFromContext(ctx)
	if state == nil || state.mode != ModeVerifyOnly || state.target == nil || !state.conds.present() {
		return nil
	}
	revision, err := readRevision(ctx, q, *state.target)
	if err != nil {
		return err
	}
	target := newWriteTarget(true, concurrencyValidator(*state.target, revision), nil)
	if err = evaluateWrite(state.conds, target, false); err != nil {
		state.setFailure(err)
		return err
	}
	state.recordCommit(true, "")
	return nil
}

// PreCheck evaluates the target precondition of an existing resource without
// a lock, so stale writes fail before doing work. The authoritative
// evaluation happens at commit.
func PreCheck(ctx context.Context, q Queryer, ref ResourceRef) error {
	state := stateFromContext(ctx)
	if !state.evaluatesWrites() || !state.isTarget(ref) {
		return nil
	}
	revision, err := readRevision(ctx, q, ref)
	if err != nil {
		return err
	}
	target := newWriteTarget(true, concurrencyValidator(ref, revision), state.addressedExistence())
	if err = evaluateWrite(state.conds, target, state.requireIfMatch); err != nil {
		state.setFailure(err)
		return err
	}
	return nil
}

// RevisionExpression selects the committed revision of a resource as a
// column of another statement, so a single-statement read returns the
// representation and its revision from one snapshot.
func RevisionExpression(ref ResourceRef) exp.Expression {
	revision := dialect.From(revisionTable).Select(goqu.C(columnRevision)).Where(refCondition(ref))
	return goqu.COALESCE(revision, 0)
}

// ObserveRevision records the revision of a resource read in the same
// statement as its representation. It only applies to the request target.
func ObserveRevision(ctx context.Context, ref ResourceRef, revision int64) {
	state := stateFromContext(ctx)
	if state.observesReads() && state.isTarget(ref) {
		state.observe(revision)
	}
}

// RevisionUpsertDataset builds the statement that assigns new revisions to
// created or changed resources, for writes that run as one statement batch
// instead of a database/sql transaction. It returns kind, identifier and
// revision of each resource.
func RevisionUpsertDataset(refs ...ResourceRef) *goqu.InsertDataset {
	rows := make([]any, 0, len(refs))
	for _, ref := range sortedRefs(refs) {
		rows = append(rows, goqu.Record{columnKind: string(ref.Kind), columnIdentifier: ref.Identifier, columnRevision: nextRevision()})
	}
	return dialect.Insert(revisionTable).Rows(rows...).
		OnConflict(goqu.DoUpdate(columnKind+", "+columnIdentifier, goqu.Record{columnRevision: nextRevision()})).
		Returning(goqu.C(columnKind), goqu.C(columnIdentifier), goqu.C(columnRevision))
}

// RecordCreated records the revision of a resource that a collection POST
// created outside FlushTx, so the response carries its entity tag.
func RecordCreated(ctx context.Context, ref ResourceRef, revision int64) {
	state := stateFromContext(ctx)
	if state == nil || state.mode != ModeCollection || state.kind != ref.Kind {
		return
	}
	state.recordCommit(false, ConcurrencyETag(ref, revision))
}
