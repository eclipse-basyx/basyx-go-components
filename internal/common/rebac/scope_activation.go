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
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/doug-martin/goqu/v9"
)

// IntegrationState contains effective runtime settings, including forced defaults.
type IntegrationState struct {
	AASRegistry      bool `json:"aas_registry"`
	SubmodelRegistry bool `json:"submodel_registry"`
	Discovery        bool `json:"discovery"`
}

func (i IntegrationState) enabled(kind string) bool {
	switch kind {
	case "aas_registry":
		return i.AASRegistry
	case "submodel_registry":
		return i.SubmodelRegistry
	case "discovery":
		return i.Discovery
	default:
		return false
	}
}

// ScopeActivation retains a database session lock for this process configuration.
type ScopeActivation struct {
	conn *sql.Conn
	once sync.Once
}

// Close destroys the dedicated connection so session locks cannot enter the pool.
func (a *ScopeActivation) Close() {
	a.once.Do(func() { _ = a.conn.Raw(func(any) error { return driver.ErrBadConn }); _ = a.conn.Close() })
}

func scopeSessionLock(ctx context.Context, conn *sql.Conn, scope, purpose, function string) (bool, error) {
	statement, args, err := stateDialect.Select(goqu.Func(function, goqu.Func("hashtextextended", scope+":"+purpose, 0))).Prepared(true).ToSQL()
	if err != nil {
		return false, fmt.Errorf("REBAC-ACTIVATE-LOCKSQL: %w", err)
	}
	if function == "pg_advisory_lock" {
		_, err = conn.ExecContext(ctx, statement, args...)
		if err != nil {
			return false, fmt.Errorf("REBAC-ACTIVATE-LOCK: %w", err)
		}
		return true, nil
	}
	var result bool
	if err = conn.QueryRowContext(ctx, statement, args...).Scan(&result); err != nil {
		return false, fmt.Errorf("REBAC-ACTIVATE-LOCK: %w", err)
	}
	return result, nil
}

// ActivateIntegrations changes derived relationships only when no incompatible
// live participant holds the scope. State locks fence instances after connection loss.
func (s *StateStore) ActivateIntegrations(ctx context.Context, desired IntegrationState) (*ScopeActivation, error) {
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("REBAC-ACTIVATE-CONNECT: %w", err)
	}
	activation := &ScopeActivation{conn: conn}
	success := false
	defer func() {
		if !success {
			activation.Close()
		}
	}()
	locked, err := scopeSessionLock(ctx, conn, s.Scope, "startup", "pg_advisory_lock")
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrScopeConflict
	}
	if err = s.activateIntegrationState(ctx, conn, desired); err != nil {
		return nil, err
	}
	if _, err = scopeSessionLock(ctx, conn, s.Scope, "startup", "pg_advisory_unlock"); err != nil {
		return nil, err
	}
	success = true
	go func() { <-ctx.Done(); activation.Close() }()
	return activation, nil
}

func (s *StateStore) activateIntegrationState(ctx context.Context, conn *sql.Conn, desired IntegrationState) error {
	data, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("REBAC-ACTIVATE-ENCODE: %w", err)
	}
	expected := string(data)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("REBAC-ACTIVATE-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	statement, args, err := stateDialect.From("rebac_scope").Select("integration_state").Where(goqu.C("scope").Eq(s.Scope)).ForUpdate(goqu.Wait).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-ACTIVATE-READSQL: %w", err)
	}
	var previous string
	if err = tx.QueryRowContext(ctx, statement, args...).Scan(&previous); err != nil {
		return fmt.Errorf("REBAC-ACTIVATE-READ: %w", err)
	}
	changed := previous != expected
	if changed {
		if err = s.changeIntegrationState(ctx, conn, tx, desired, expected); err != nil {
			return err
		}
	}
	locked, err := scopeSessionLock(ctx, conn, s.Scope, "participants", "pg_try_advisory_lock_shared")
	if err != nil {
		return err
	}
	if !locked {
		return ErrScopeConflict
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("REBAC-ACTIVATE-COMMIT: %w", err)
	}
	if changed {
		if _, err = scopeSessionLock(ctx, conn, s.Scope, "participants", "pg_advisory_unlock"); err != nil {
			return err
		}
	}
	s.ExpectedIntegrationState = expected
	return nil
}

func (s *StateStore) reconcileIntegrationState(ctx context.Context, tx *sql.Tx, desired IntegrationState, expected string) error {
	writes, deletes, err := s.integrationStateTuples(ctx, tx, desired)
	if err != nil {
		return err
	}
	if _, err = s.QueueChanged(ctx, tx, writes, deletes); err != nil {
		return err
	}
	statement, args, err := stateDialect.Update("rebac_scope").Set(goqu.Record{"integration_state": expected}).Where(goqu.C("scope").Eq(s.Scope)).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-ACTIVATE-UPDATESQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, statement, args...); err != nil {
		return fmt.Errorf("REBAC-ACTIVATE-UPDATE: %w", err)
	}
	return nil
}

func (s *StateStore) integrationStateTuples(ctx context.Context, tx *sql.Tx, desired IntegrationState) ([]Tuple, []Tuple, error) {
	query := stateDialect.From(goqu.T("rebac_generated").As("g")).
		Join(goqu.T("rebac_resource").As("s"), goqu.On(goqu.I("s.resource_uuid").Eq(goqu.I("g.source_uuid")))).
		Join(goqu.T("rebac_resource").As("t"), goqu.On(goqu.I("t.resource_uuid").Eq(goqu.I("g.target_uuid")))).
		Select(goqu.I("s.kind"), goqu.I("s.resource_uuid"), goqu.I("t.kind"), goqu.I("t.resource_uuid"), goqu.I("g.integration")).
		Where(goqu.Ex{"g.scope": s.Scope, "s.scope": s.Scope, "t.scope": s.Scope, "s.deleted_at": nil, "t.deleted_at": nil})
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, nil, fmt.Errorf("REBAC-ACTIVATE-PROVENANCESQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("REBAC-ACTIVATE-PROVENANCE: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var writes, deletes []Tuple
	for rows.Next() {
		var sourceKind, sourceID, targetKind, targetID, integration string
		if err = rows.Scan(&sourceKind, &sourceID, &targetKind, &targetID, &integration); err != nil {
			return nil, nil, fmt.Errorf("REBAC-ACTIVATE-SCAN: %w", err)
		}
		source, sourceErr := ResourceObject(s.Scope, sourceKind, sourceID)
		if sourceErr != nil {
			return nil, nil, sourceErr
		}
		target, targetErr := ResourceObject(s.Scope, targetKind, targetID)
		if targetErr != nil {
			return nil, nil, targetErr
		}
		tuple := Tuple{User: source, Relation: "source", Object: target}
		if desired.enabled(integration) {
			writes = append(writes, tuple)
		} else {
			deletes = append(deletes, tuple)
		}
	}
	return writes, deletes, rows.Err()
}

func (s *StateStore) validateIntegrationState(ctx context.Context, tx *sql.Tx) error {
	statement, args, err := stateDialect.From("rebac_scope").Select("integration_state", "recovery_pending").Where(goqu.C("scope").Eq(s.Scope)).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-ACTIVATE-FENCESQL: %w", err)
	}
	var current string
	var recovering bool
	if err = tx.QueryRowContext(ctx, statement, args...).Scan(&current, &recovering); err != nil {
		return fmt.Errorf("REBAC-ACTIVATE-FENCE: %w", err)
	}
	if s.ExpectedIntegrationState != "" && current != s.ExpectedIntegrationState {
		return ErrScopeConflict
	}
	if recovering && !s.recoveryMode {
		return ErrPending
	}
	return nil
}

func (s *StateStore) changeIntegrationState(ctx context.Context, conn *sql.Conn, tx *sql.Tx, desired IntegrationState, expected string) error {
	locked, err := scopeSessionLock(ctx, conn, s.Scope, "participants", "pg_try_advisory_lock")
	if err != nil {
		return err
	}
	if !locked {
		return ErrScopeConflict
	}
	return s.reconcileIntegrationState(ctx, tx, desired, expected)
}
