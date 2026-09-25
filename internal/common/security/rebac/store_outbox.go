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
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/google/uuid"
)

const (
	outboxTable     = "rebac_outbox"
	scopeStateTable = "rebac_scope_state"
)

// OutboxOperation is one tuple change to project to OpenFGA.
type OutboxOperation struct {
	Delete bool
	Tuple  Tuple
}

// OutboxRow is a queued, not yet applied operation.
type OutboxRow struct {
	Seq           int64
	Operation     OutboxOperation
	Attempts      int
	NextAttemptAt time.Time
}

// ScopeState is the projection progress of one scope.
type ScopeState struct {
	Scope             string     `json:"scope"`
	Backlog           int64      `json:"outboxBacklog"`
	RevocationPending bool       `json:"revocationPending"`
	LastEnabledAt     *time.Time `json:"lastEnabledAt,omitempty"`
	ReconciledAt      *time.Time `json:"reconciledAt,omitempty"`
}

// EnsureScopeState creates the state row of a scope.
func EnsureScopeState(ctx context.Context, q Queryer, scope string) error {
	ds := dialect.Insert(scopeStateTable).Rows(goqu.Record{"scope": scope}).OnConflict(goqu.DoNothing()).Prepared(true)
	_, err := execDataset(ctx, q, "REBAC-ENSURESCOPESTATE", ds)
	return err
}

// ReadScopeState returns the projection progress of a scope.
func ReadScopeState(ctx context.Context, q Queryer, scope string) (ScopeState, error) {
	ds := dialect.From(goqu.T(scopeStateTable)).Select(
		goqu.C("scope"), goqu.C("last_enabled_at"), goqu.C("reconciled_at"),
	).Where(goqu.C("scope").Eq(scope)).Prepared(true)
	state := ScopeState{}
	found, err := queryRowDataset(ctx, q, "REBAC-READSCOPESTATE", ds, &state.Scope, &state.LastEnabledAt, &state.ReconciledAt)
	if err != nil {
		return ScopeState{}, err
	}
	if !found {
		return ScopeState{}, fmt.Errorf("REBAC-READSCOPESTATE-MISSING scope %q is not initialized", scope)
	}
	if state.Backlog, err = OutboxBacklog(ctx, q, scope); err != nil {
		return ScopeState{}, err
	}
	state.RevocationPending, err = RevocationPending(ctx, q, scope)
	return state, err
}

// MarkScopeReconciled records a completed re-enable reconciliation.
func MarkScopeReconciled(ctx context.Context, q Queryer, scope string, enabledAt time.Time) error {
	ds := dialect.Update(scopeStateTable).Set(goqu.Record{
		"last_enabled_at": enabledAt,
		"reconciled_at":   goqu.L("clock_timestamp()"),
		"updated_at":      goqu.L("clock_timestamp()"),
	}).Where(goqu.C("scope").Eq(scope)).Prepared(true)
	_, err := execDataset(ctx, q, "REBAC-MARKSCOPERECONCILED", ds)
	return err
}

// OutboxMode describes how an enqueued change reaches OpenFGA.
type OutboxMode uint8

// Outbox modes.
const (
	// OutboxPending changes are applied by the projector after commit.
	OutboxPending OutboxMode = iota
	// OutboxRevocation changes remove access to live resources; until they
	// are applied, the barrier rejects ReBAC allows.
	OutboxRevocation
	// OutboxApplied changes were applied before commit and are only recorded.
	OutboxApplied
)

// EnqueueOutbox records tuple changes as one operation and returns its ID.
// Callers must hold the revision locks of the affected objects, which keeps
// the queue order of each tuple equal to the commit order.
func EnqueueOutbox(ctx context.Context, tx *sql.Tx, scope string, operations []OutboxOperation, mode OutboxMode) (string, error) {
	if len(operations) == 0 {
		return "", nil
	}
	operationID := uuid.NewString()
	rows := make([]any, 0, len(operations))
	for _, operation := range operations {
		record := goqu.Record{
			"scope": scope, "operation_id": goqu.L("?::uuid", operationID), "operation": operationName(operation),
			"tuple_object": operation.Tuple.Object, "tuple_relation": operation.Tuple.Relation,
			"tuple_user": operation.Tuple.User, "revokes": mode == OutboxRevocation && operation.Delete,
		}
		if mode == OutboxApplied {
			record["applied_at"] = goqu.L("clock_timestamp()")
		}
		rows = append(rows, record)
	}
	if _, err := execDataset(ctx, tx, "REBAC-ENQUEUEOUTBOX-INSERT", dialect.Insert(outboxTable).Rows(rows...).Prepared(true)); err != nil {
		return "", err
	}
	return operationID, nil
}

func operationName(operation OutboxOperation) string {
	if operation.Delete {
		return "delete"
	}
	return "write"
}

// RevocationPending reports whether a committed revocation has not reached
// OpenFGA yet. It reads a partial index that is empty in steady state.
func RevocationPending(ctx context.Context, q Queryer, scope string) (bool, error) {
	ds := dialect.From(goqu.T(outboxTable)).Select(goqu.L("1")).Where(
		goqu.C("scope").Eq(scope), goqu.C("applied_at").IsNull(), goqu.C("revokes").IsTrue(),
	).Limit(1).Prepared(true)
	var marker int
	return queryRowDataset(ctx, q, "REBAC-REVOCATIONPENDING", ds, &marker)
}

// OperationApplied reports whether an operation exists and is fully applied.
func OperationApplied(ctx context.Context, q Queryer, operationID string) (found bool, applied bool, err error) {
	if _, parseErr := uuid.Parse(operationID); parseErr != nil {
		return false, false, nil
	}
	ds := dialect.From(goqu.T(outboxTable)).Select(
		goqu.COUNT(goqu.Star()), goqu.L("COUNT(*) FILTER (WHERE applied_at IS NULL)"),
	).Where(goqu.C("operation_id").Eq(goqu.L("?::uuid", operationID))).Prepared(true)
	var total, pending int64
	if _, err = queryRowDataset(ctx, q, "REBAC-OPERATIONAPPLIED", ds, &total, &pending); err != nil {
		return false, false, err
	}
	return total > 0, total > 0 && pending == 0, nil
}

// HasPendingOperations reports whether unapplied operations exist for any of
// the objects. Direct application must wait for them to preserve order.
func HasPendingOperations(ctx context.Context, q Queryer, scope string, objects []string) (bool, error) {
	ds := dialect.From(goqu.T(outboxTable)).Select(goqu.L("1")).Where(
		goqu.C("scope").Eq(scope), goqu.C("applied_at").IsNull(), goqu.C("tuple_object").In(objects),
	).Limit(1).Prepared(true)
	var marker int
	return queryRowDataset(ctx, q, "REBAC-HASPENDINGOPS", ds, &marker)
}

// PendingOutbox returns the oldest unapplied operations in queue order.
func PendingOutbox(ctx context.Context, q Queryer, scope string, limit uint) ([]OutboxRow, error) {
	ds := dialect.From(goqu.T(outboxTable)).Select(
		goqu.C("seq"), goqu.C("operation"), goqu.C("tuple_object"), goqu.C("tuple_relation"), goqu.C("tuple_user"),
		goqu.C("attempts"), goqu.C("next_attempt_at"),
	).Where(goqu.C("scope").Eq(scope), goqu.C("applied_at").IsNull()).
		Order(goqu.C("seq").Asc()).Limit(limit)
	rows, err := queryDataset(ctx, q, "REBAC-PENDINGOUTBOX", ds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var pending []OutboxRow
	for rows.Next() {
		var row OutboxRow
		var operation string
		if err = rows.Scan(&row.Seq, &operation, &row.Operation.Tuple.Object, &row.Operation.Tuple.Relation,
			&row.Operation.Tuple.User, &row.Attempts, &row.NextAttemptAt); err != nil {
			return nil, fmt.Errorf("REBAC-PENDINGOUTBOX-SCAN: %w", err)
		}
		row.Operation.Delete = operation == "delete"
		pending = append(pending, row)
	}
	return pending, rows.Err()
}

// MarkOutboxApplied marks operations as projected.
func MarkOutboxApplied(ctx context.Context, q Queryer, seqs []int64) error {
	ds := dialect.Update(outboxTable).Set(goqu.Record{
		"applied_at": goqu.L("clock_timestamp()"), "last_error": nil,
	}).Where(goqu.C("seq").In(seqs)).Prepared(true)
	_, err := execDataset(ctx, q, "REBAC-MARKOUTBOXAPPLIED", ds)
	return err
}

// MarkOutboxFailed schedules the head of the queue for a retry.
func MarkOutboxFailed(ctx context.Context, q Queryer, seq int64, cause error, retryAt time.Time) error {
	message := cause.Error()
	if len(message) > 1024 {
		message = message[:1024]
	}
	ds := dialect.Update(outboxTable).Set(goqu.Record{
		"attempts": goqu.L("attempts + 1"), "last_error": message, "next_attempt_at": retryAt,
	}).Where(goqu.C("seq").Eq(seq)).Prepared(true)
	_, err := execDataset(ctx, q, "REBAC-MARKOUTBOXFAILED", ds)
	return err
}

// PruneAppliedOutbox deletes applied operations older than retention.
func PruneAppliedOutbox(ctx context.Context, q Queryer, retention time.Duration) error {
	ds := dialect.Delete(outboxTable).Where(
		goqu.C("applied_at").Lt(time.Now().Add(-retention)),
	).Prepared(true)
	_, err := execDataset(ctx, q, "REBAC-PRUNEOUTBOX", ds)
	return err
}

// OutboxBacklog counts unapplied operations of a scope.
func OutboxBacklog(ctx context.Context, q Queryer, scope string) (int64, error) {
	ds := dialect.From(goqu.T(outboxTable)).Select(goqu.COUNT(goqu.Star())).
		Where(goqu.C("scope").Eq(scope), goqu.C("applied_at").IsNull()).Prepared(true)
	var count int64
	_, err := queryRowDataset(ctx, q, "REBAC-OUTBOXBACKLOG", ds, &count)
	return count, err
}
