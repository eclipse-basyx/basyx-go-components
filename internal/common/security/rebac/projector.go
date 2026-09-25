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
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"time"

	"github.com/doug-martin/goqu/v9"
)

const (
	projectorBatchSize      = maxWriteTuples
	projectorIdleInterval   = 2 * time.Second
	projectorBusyInterval   = 100 * time.Millisecond
	projectorMaxBackoff     = 30 * time.Second
	projectorBaseBackoff    = 250 * time.Millisecond
	projectorPruneInterval  = time.Hour
	projectorPruneRetention = 7 * 24 * time.Hour
	projectorWaitInterval   = 25 * time.Millisecond
	projectorBurstWindow    = 2 * time.Second
	projectorRevocationWait = time.Second
)

// errProjectorBusy reports that another instance holds the projection lock.
var errProjectorBusy = errors.New("REBAC-PROJECTOR-BUSY projection lock held by another instance")

// Projector applies the outbox to OpenFGA in queue order. Only one instance
// per scope projects at a time, elected through a PostgreSQL advisory lock.
type Projector struct {
	db      *sql.DB
	client  Client
	scope   string
	lockKey int64
	notify  chan struct{}
}

// NewProjector creates the projector of a scope.
func NewProjector(db *sql.DB, client Client, scope string) *Projector {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte("basyx-rebac-projector:" + scope))
	return &Projector{
		db:      db,
		client:  client,
		scope:   scope,
		lockKey: int64(hash.Sum64() & math.MaxInt64), //nolint:gosec // masked to the positive int64 range
		notify:  make(chan struct{}, 1),
	}
}

// Notify wakes the background worker after new operations were committed.
func (p *Projector) Notify() {
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

// Run drains the outbox until ctx is cancelled. After a notification it polls
// quickly for a short while, because notifications are sent before the
// notifying transaction commits.
func (p *Projector) Run(ctx context.Context) {
	lastPrune := time.Now()
	burstUntil := time.Time{}
	for {
		drained, err := p.DrainOnce(ctx)
		if err != nil && !errors.Is(err, errProjectorBusy) && ctx.Err() == nil {
			slog.WarnContext(ctx, "ReBAC outbox projection failed", "error.code", "REBAC-PROJECTOR-DRAIN", "error", err)
		}
		if time.Since(lastPrune) > projectorPruneInterval {
			p.prune(ctx)
			lastPrune = time.Now()
		}
		interval := projectorIdleInterval
		if drained > 0 || time.Now().Before(burstUntil) {
			interval = projectorBusyInterval
		}
		select {
		case <-ctx.Done():
			return
		case <-p.notify:
			burstUntil = time.Now().Add(projectorBurstWindow)
		case <-time.After(interval):
		}
	}
}

func (p *Projector) prune(ctx context.Context) {
	if err := PruneAppliedOutbox(ctx, p.db, projectorPruneRetention); err != nil {
		slog.WarnContext(ctx, "ReBAC outbox prune failed", "error.code", "REBAC-PROJECTOR-PRUNE", "error", err)
	}
}

// WaitApplied drains until the operation is applied or ctx expires. It
// reports whether the operation reached OpenFGA.
func (p *Projector) WaitApplied(ctx context.Context, operationID string) (bool, error) {
	for {
		if _, err := p.DrainOnce(ctx); err != nil && !errors.Is(err, errProjectorBusy) {
			return false, err
		}
		_, applied, err := OperationApplied(ctx, p.db, operationID)
		if err != nil || applied {
			return applied, err
		}
		select {
		case <-ctx.Done():
			return false, nil
		case <-time.After(projectorWaitInterval):
		}
	}
}

// DrainRevocations drains the queue until no committed revocation is pending
// or the bounded wait ends. It reports whether all revocations are applied.
func (p *Projector) DrainRevocations(ctx context.Context) (bool, error) {
	waitCtx, cancel := context.WithTimeout(ctx, projectorRevocationWait)
	defer cancel()
	for {
		if _, err := p.DrainOnce(waitCtx); err != nil && !errors.Is(err, errProjectorBusy) {
			return false, nil
		}
		pending, err := RevocationPending(ctx, p.db, p.scope)
		if err != nil || !pending {
			return !pending, err
		}
		select {
		case <-waitCtx.Done():
			return false, nil
		case <-time.After(projectorWaitInterval):
		}
	}
}

// DrainOnce applies one batch of pending operations and returns how many
// operations were applied.
func (p *Projector) DrainOnce(ctx context.Context) (applied int, err error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("REBAC-PROJECTOR-BEGIN: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	locked, err := p.tryLock(ctx, tx)
	if err != nil || !locked {
		_ = tx.Rollback()
		if err == nil {
			return 0, errProjectorBusy
		}
		return 0, err
	}
	applied, err = p.applyBatch(ctx, tx)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("REBAC-PROJECTOR-COMMIT: %w", err)
	}
	return applied, nil
}

func (p *Projector) tryLock(ctx context.Context, tx *sql.Tx) (bool, error) {
	ds := dialect.Select(goqu.Func("pg_try_advisory_xact_lock", p.lockKey)).Prepared(true)
	var locked bool
	if _, err := queryRowDataset(ctx, tx, "REBAC-PROJECTOR-LOCK", ds, &locked); err != nil {
		return false, err
	}
	return locked, nil
}

func (p *Projector) applyBatch(ctx context.Context, tx *sql.Tx) (int, error) {
	pending, err := PendingOutbox(ctx, tx, p.scope, projectorBatchSize)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	if pending[0].NextAttemptAt.After(time.Now()) {
		return 0, nil
	}
	batch := conflictFreePrefix(pending)
	writes, deletes := splitOperations(batch)
	if writeErr := p.client.Write(ctx, writes, deletes); writeErr != nil {
		retryAt := time.Now().Add(projectorBackoff(pending[0].Attempts))
		if markErr := MarkOutboxFailed(ctx, tx, pending[0].Seq, writeErr, retryAt); markErr != nil {
			return 0, markErr
		}
		return 0, nil
	}
	seqs := make([]int64, len(batch))
	for index, row := range batch {
		seqs[index] = row.Seq
	}
	if err = MarkOutboxApplied(ctx, tx, seqs); err != nil {
		return 0, err
	}
	return len(batch), nil
}

// conflictFreePrefix returns the longest queue prefix that touches every
// tuple at most once, so one OpenFGA transaction never writes and deletes
// the same tuple.
func conflictFreePrefix(rows []OutboxRow) []OutboxRow {
	seen := make(map[Tuple]struct{}, len(rows))
	for index, row := range rows {
		if _, duplicate := seen[row.Operation.Tuple]; duplicate {
			return rows[:index]
		}
		seen[row.Operation.Tuple] = struct{}{}
	}
	return rows
}

func splitOperations(rows []OutboxRow) ([]Tuple, []Tuple) {
	var writes, deletes []Tuple
	for _, row := range rows {
		if row.Operation.Delete {
			deletes = append(deletes, row.Operation.Tuple)
			continue
		}
		writes = append(writes, row.Operation.Tuple)
	}
	return writes, deletes
}

func projectorBackoff(attempts int) time.Duration {
	backoff := projectorBaseBackoff
	for range min(attempts, 16) {
		backoff *= 2
		if backoff >= projectorMaxBackoff {
			return projectorMaxBackoff
		}
	}
	return backoff
}
