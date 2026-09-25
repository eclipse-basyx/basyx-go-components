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

package rebac

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/doug-martin/goqu/v9"
)

func (s *StateStore) mergeTransactionChange(ctx context.Context, tx *sql.Tx, revision int64, writes, deletes []Tuple) (int64, error) {
	query, args, err := stateDialect.From("rebac_outbox").Select(goqu.L("transaction_id = txid_current()")).Where(goqu.Ex{"scope": s.Scope, "revision": revision}).Prepared(true).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("REBAC-STATE-TXSQL: %w", err)
	}
	var owned bool
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&owned); err != nil {
		return 0, fmt.Errorf("REBAC-STATE-TXOWNER: %w", err)
	}
	if !owned {
		return 0, ErrPending
	}
	previous, err := s.pendingChange(ctx, tx, revision)
	if err != nil {
		return 0, err
	}
	if err = s.applyDesired(ctx, tx, writes, deletes); err != nil {
		return 0, err
	}
	previous.Writes, previous.Deletes = mergeTupleChanges(previous.Writes, previous.Deletes, writes, deletes)
	w, err := json.Marshal(previous.Writes)
	if err != nil {
		return 0, err
	}
	d, err := json.Marshal(previous.Deletes)
	if err != nil {
		return 0, err
	}
	query, args, err = stateDialect.Update("rebac_outbox").Set(goqu.Record{"writes": string(w), "deletes": string(d)}).Where(goqu.Ex{"scope": s.Scope, "revision": revision}).Prepared(true).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("REBAC-STATE-MERGESQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return 0, fmt.Errorf("REBAC-STATE-MERGE: %w", err)
	}
	return revision, nil
}

func mergeTupleChanges(oldWrites, oldDeletes, writes, deletes []Tuple) ([]Tuple, []Tuple) {
	// Keep the final desired operation; idempotent delivery tolerates absent deletes.
	w := map[Tuple]bool{}
	d := map[Tuple]bool{}
	for _, t := range oldWrites {
		w[t] = true
	}
	for _, t := range oldDeletes {
		d[t] = true
	}
	for _, t := range deletes {
		delete(w, t)
		d[t] = true
	}
	for _, t := range writes {
		delete(d, t)
		w[t] = true
	}
	return sortedTuples(w), sortedTuples(d)
}

func sortedTuples(set map[Tuple]bool) []Tuple {
	result := make([]Tuple, 0, len(set))
	for item := range set {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Object != b.Object {
			return a.Object < b.Object
		}
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		return a.User < b.User
	})
	return result
}
