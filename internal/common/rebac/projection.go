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
	"time"

	"github.com/doug-martin/goqu/v9"
)

// TupleWriter applies idempotent relationship changes to the projection.
type TupleWriter interface {
	Write(context.Context, []Tuple, []Tuple) error
}

// ProjectNext keeps the scope locked until a complete revision is acknowledged.
// Retrying after an uncertain write requires an idempotent TupleWriter.
func (s *StateStore) ProjectNext(ctx context.Context, writer TupleWriter) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("REBAC-PROJECT-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := s.Lock(ctx, tx, true)
	if err != nil {
		return false, err
	}
	if state.Desired == state.Applied {
		return false, nil
	}
	change, err := s.pendingChange(ctx, tx, state.Applied+1)
	if err != nil {
		return false, err
	}
	if err = writer.Write(ctx, change.Writes, change.Deletes); err != nil {
		return false, fmt.Errorf("REBAC-PROJECT-WRITE: %w", err)
	}
	if err = s.acknowledge(ctx, tx, change.Revision); err != nil {
		return false, err
	}
	if s.Projected != nil {
		if err = s.Projected(ctx, tx, change); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("REBAC-PROJECT-COMMIT: %w", err)
	}
	return true, nil
}

func (s *StateStore) pendingChange(ctx context.Context, tx *sql.Tx, revision int64) (ProjectionChange, error) {
	change := ProjectionChange{Revision: revision}
	statement, args, err := stateDialect.From("rebac_outbox").Select("writes", "deletes").Where(goqu.Ex{"scope": s.Scope, "revision": revision}).Prepared(true).ToSQL()
	if err != nil {
		return change, fmt.Errorf("REBAC-PROJECT-SELECTSQL: %w", err)
	}
	var writes, deletes []byte
	if err = tx.QueryRowContext(ctx, statement, args...).Scan(&writes, &deletes); err != nil {
		return change, fmt.Errorf("REBAC-PROJECT-SELECT: %w", err)
	}
	if err = json.Unmarshal(writes, &change.Writes); err != nil {
		return change, fmt.Errorf("REBAC-PROJECT-DECODEWRITES: %w", err)
	}
	if err = json.Unmarshal(deletes, &change.Deletes); err != nil {
		return change, fmt.Errorf("REBAC-PROJECT-DECODEDELETES: %w", err)
	}
	return change, nil
}

func (s *StateStore) acknowledge(ctx context.Context, tx *sql.Tx, revision int64) error {
	statement, args, err := stateDialect.Update("rebac_scope").Set(goqu.Record{"applied_revision": revision}).Where(goqu.Ex{"scope": s.Scope, "desired_revision": revision}).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-PROJECT-ACKSQL: %w", err)
	}
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("REBAC-PROJECT-ACK: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return fmt.Errorf("REBAC-PROJECT-ACKCOUNT expected one scope row")
	}
	return nil
}

// RunProjection stops when the owning service context is cancelled.
func (s *StateStore) RunProjection(ctx context.Context, writer TupleWriter, interval time.Duration, report func(error)) {
	if interval <= 0 {
		report(fmt.Errorf("REBAC-PROJECT-INTERVAL must be positive"))
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.ProjectNext(ctx, writer); err != nil {
				report(err)
			}
		}
	}
}
