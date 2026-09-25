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
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"go.opentelemetry.io/otel"
)

// ProjectionReader reads the actual relationship projection without authorizing it.
type ProjectionReader interface {
	TupleWriter
	ReadTuples(context.Context, string) ([]Tuple, string, error)
	ReadChanges(context.Context, string, int) (ChangesPage, error)
}

// Reconcile restores the committed desired graph, including removal of unexpected
// tuples on retired generations. The durable marker survives crashes and retries.
func (s *StateStore) Reconcile(ctx context.Context, client ProjectionReader) error {
	ctx, span := otel.Tracer("github.com/eclipse-basyx/basyx-go-components/rebac").Start(ctx, "rebac.reconcile")
	defer span.End()
	recovery := *s
	recovery.recoveryMode = true
	if err := recovery.markRecovery(ctx); err != nil {
		return err
	}
	err := recovery.repairProjection(ctx, client)
	if err != nil {
		recovery.recordRecoveryFailure(ctx)
	}
	return err
}

func (s *StateStore) scopeUpdate(ctx context.Context, tx *sql.Tx, values goqu.Record) error {
	statement, args, err := stateDialect.Update("rebac_scope").Set(values).Where(goqu.C("scope").Eq(s.Scope)).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-RECOVERY-UPDATESQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, statement, args...); err != nil {
		return fmt.Errorf("REBAC-RECOVERY-UPDATE: %w", err)
	}
	return nil
}

func (s *StateStore) recoveryAudit(ctx context.Context, tx *sql.Tx, phase string, details map[string]string) error {
	if s.RecoveryAudit == nil {
		return nil
	}
	return s.RecoveryAudit(ctx, tx, phase, details)
}

func (s *StateStore) markRecovery(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("REBAC-RECOVERY-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := s.Lock(ctx, tx, true)
	if err != nil {
		return err
	}
	if err = s.scopeUpdate(ctx, tx, goqu.Record{"recovery_pending": true}); err != nil {
		return err
	}
	if err = s.recoveryAudit(ctx, tx, "intent", map[string]string{"revision": fmt.Sprint(state.Desired)}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *StateStore) repairProjection(ctx context.Context, client ProjectionReader) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("REBAC-RECOVERY-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := s.Lock(ctx, tx, true)
	if err != nil {
		return err
	}
	desired, known, err := s.recoveryCatalog(ctx, tx)
	if err != nil {
		return err
	}
	actual, unknown, err := scopedProjection(ctx, client, known)
	if err != nil {
		return err
	}
	writes, deletes := tupleDifference(desired, actual), tupleDifference(actual, desired)
	if err = writeRepairBatches(ctx, client, writes, deletes); err != nil {
		return err
	}
	cursor, err := s.recoveryCursor(ctx, tx, client)
	if err != nil {
		return err
	}
	verified, _, err := scopedProjection(ctx, client, known)
	if err != nil {
		return err
	}
	if len(tupleDifference(desired, verified))+len(tupleDifference(verified, desired)) != 0 {
		return fmt.Errorf("REBAC-RECOVERY-VERIFY projection differs after repair")
	}
	details := map[string]string{"revision": fmt.Sprint(state.Desired), "writes": fmt.Sprint(len(writes)), "deletes": fmt.Sprint(len(deletes)), "unattributed_tuples": fmt.Sprint(unknown)}
	if err = s.recoveryAudit(ctx, tx, "applied", details); err != nil {
		return err
	}
	if err = s.scopeUpdate(ctx, tx, goqu.Record{"recovery_pending": false, "applied_revision": state.Desired, "changes_cursor": cursor}); err != nil {
		return err
	}
	return tx.Commit()
}

func writeRepairBatches(ctx context.Context, writer TupleWriter, writes, deletes []Tuple) error {
	for start := 0; start < len(deletes); start += 50 {
		if err := writer.Write(ctx, nil, deletes[start:min(start+50, len(deletes))]); err != nil {
			return fmt.Errorf("REBAC-RECOVERY-DELETE: %w", err)
		}
	}
	for start := 0; start < len(writes); start += 50 {
		if err := writer.Write(ctx, writes[start:min(start+50, len(writes))], nil); err != nil {
			return fmt.Errorf("REBAC-RECOVERY-WRITE: %w", err)
		}
	}
	return nil
}

func scopedProjection(ctx context.Context, client ProjectionReader, known map[string]bool) ([]Tuple, int, error) {
	var result []Tuple
	cursor := ""
	unknown := 0
	for {
		tuples, next, err := client.ReadTuples(ctx, cursor)
		if err != nil {
			return nil, 0, err
		}
		for _, tuple := range tuples {
			if known[tuple.Object] {
				result = append(result, tuple)
			} else {
				unknown++
			}
		}
		if next == "" {
			return result, unknown, nil
		}
		if next == cursor {
			return nil, 0, fmt.Errorf("REBAC-RECOVERY-CURSOR repeated tuple cursor")
		}
		cursor = next
	}
}

func (s *StateStore) recordRecoveryFailure(ctx context.Context) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()
	state, err := s.Lock(ctx, tx, true)
	if err != nil {
		return
	}
	if err = s.recoveryAudit(ctx, tx, "failed", map[string]string{"revision": fmt.Sprint(state.Desired)}); err == nil {
		_ = tx.Commit()
	}
}

func (s *StateStore) recoveryCatalog(ctx context.Context, tx *sql.Tx) ([]Tuple, map[string]bool, error) {
	desired, err := s.desiredRecoveryTuples(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	statement, args, err := stateDialect.From("rebac_resource").Select("kind", "resource_uuid").Where(goqu.C("scope").Eq(s.Scope)).Prepared(true).ToSQL()
	if err != nil {
		return nil, nil, fmt.Errorf("REBAC-RECOVERY-RESOURCESQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("REBAC-RECOVERY-RESOURCES: %w", err)
	}
	defer func() { _ = rows.Close() }()
	known := map[string]bool{}
	for rows.Next() {
		var kind, id string
		if err = rows.Scan(&kind, &id); err != nil {
			return nil, nil, err
		}
		object, objectErr := ResourceObject(s.Scope, kind, id)
		if objectErr != nil {
			return nil, nil, objectErr
		}
		known[object] = true
	}
	for _, tuple := range desired {
		known[tuple.Object] = true
	}
	return desired, known, rows.Err()
}

func (s *StateStore) desiredRecoveryTuples(ctx context.Context, tx *sql.Tx) ([]Tuple, error) {
	statement, args, err := stateDialect.From("rebac_relationship").Select("subject", "relation", "object").Where(goqu.C("scope").Eq(s.Scope)).Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-RECOVERY-DESIREDSQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-RECOVERY-DESIRED: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result []Tuple
	for rows.Next() {
		var tuple Tuple
		if err = rows.Scan(&tuple.User, &tuple.Relation, &tuple.Object); err != nil {
			return nil, err
		}
		result = append(result, tuple)
	}
	return result, rows.Err()
}

// MonitorProjection detects unexpected changes and periodically verifies restored stores.
func (s *StateStore) MonitorProjection(ctx context.Context, client ProjectionReader, report func(error)) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastFull := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			drift, err := s.detectProjectionChanges(ctx, client)
			if err == nil && (drift || time.Since(lastFull) >= time.Minute) {
				err = s.Reconcile(ctx, client)
				if err == nil {
					lastFull = time.Now()
				}
			}
			if err != nil {
				report(err)
			}
		}
	}
}

func (s *StateStore) detectProjectionChanges(ctx context.Context, client ProjectionReader) (bool, error) {
	recovery := *s
	recovery.recoveryMode = true
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("REBAC-RECOVERY-MONITORBEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := recovery.Lock(ctx, tx, true)
	if err != nil {
		return false, err
	}
	cursor, pending, err := recovery.recoveryStatus(ctx, tx)
	if err != nil {
		return false, err
	}
	if pending {
		return true, nil
	}
	if state.Desired != state.Applied {
		return false, nil
	}
	page, err := client.ReadChanges(ctx, cursor, 100)
	if err != nil {
		return s.failedChangeFeed(ctx, tx, state.Desired)
	}
	if len(page.Changes) == 0 {
		if err = s.scopeUpdate(ctx, tx, goqu.Record{"changes_cursor": page.ContinuationToken}); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	desired, known, err := s.recoveryCatalog(ctx, tx)
	if err != nil {
		return false, err
	}
	drift := changesDiffer(page.Changes, desired, known)
	if drift {
		if err = s.recoveryAudit(ctx, tx, "unexpected_change", map[string]string{"changes": fmt.Sprint(len(page.Changes)), "revision": fmt.Sprint(state.Desired)}); err != nil {
			return false, err
		}
	}
	if err = s.scopeUpdate(ctx, tx, goqu.Record{"changes_cursor": page.ContinuationToken, "recovery_pending": drift}); err != nil {
		return false, err
	}
	return drift, tx.Commit()
}

func changesDiffer(changes []Change, desired []Tuple, known map[string]bool) bool {
	present := map[Tuple]bool{}
	for _, tuple := range desired {
		present[tuple] = true
	}
	for _, change := range changes {
		if !known[change.Tuple.Object] {
			continue
		}
		if change.Operation == "TUPLE_OPERATION_WRITE" && !present[change.Tuple] {
			return true
		}
		if change.Operation == "TUPLE_OPERATION_DELETE" && present[change.Tuple] {
			return true
		}
	}
	return false
}

func (s *StateStore) recoveryStatus(ctx context.Context, tx *sql.Tx) (string, bool, error) {
	statement, args, err := stateDialect.From("rebac_scope").Select("changes_cursor", "recovery_pending").Where(goqu.C("scope").Eq(s.Scope)).Prepared(true).ToSQL()
	if err != nil {
		return "", false, fmt.Errorf("REBAC-RECOVERY-STATUSSQL: %w", err)
	}
	var cursor string
	var pending bool
	if err = tx.QueryRowContext(ctx, statement, args...).Scan(&cursor, &pending); err != nil {
		return "", false, fmt.Errorf("REBAC-RECOVERY-STATUS: %w", err)
	}
	return cursor, pending, nil
}

func (s *StateStore) recoveryCursor(ctx context.Context, tx *sql.Tx, client ProjectionReader) (string, error) {
	cursor, _, err := s.recoveryStatus(ctx, tx)
	if err != nil {
		return "", err
	}
	reset := false
	for {
		page, readErr := client.ReadChanges(ctx, cursor, 100)
		if readErr != nil {
			if cursor != "" && !reset {
				cursor = ""
				reset = true
				continue
			}
			return "", readErr
		}
		if page.ContinuationToken == "" {
			return "", nil
		}
		if len(page.Changes) == 0 || page.ContinuationToken == cursor {
			return page.ContinuationToken, nil
		}
		cursor = page.ContinuationToken
	}
}

func (s *StateStore) failedChangeFeed(ctx context.Context, tx *sql.Tx, revision int64) (bool, error) {
	if err := s.scopeUpdate(ctx, tx, goqu.Record{"changes_cursor": "", "recovery_pending": true}); err != nil {
		return false, err
	}
	if err := s.recoveryAudit(ctx, tx, "change_feed_unavailable", map[string]string{"revision": fmt.Sprint(revision)}); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
