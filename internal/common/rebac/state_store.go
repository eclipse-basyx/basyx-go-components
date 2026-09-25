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
	"errors"
	"fmt"
	"strings"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/google/uuid"
)

var stateDialect = goqu.Dialect("postgres")

// ErrPending indicates that desired authorization state is not fully projected.
var ErrPending = errors.New("REBAC-STATE-PENDING authorization projection is pending")

// ErrScopeConflict rejects inconsistent configuration among scope participants.
var ErrScopeConflict = errors.New("REBAC-STATE-CONFIG scope configuration differs")

// StateStore keeps desired authorization state in the resource writer database.
type StateStore struct {
	DB                       *sql.DB
	Scope                    string
	ExpectedIntegrationState string
	recoveryMode             bool
	RecoveryAudit            func(context.Context, *sql.Tx, string, map[string]string) error
	Projected                func(context.Context, *sql.Tx, ProjectionChange) error
}

// ScopeState tracks desired and fully applied authorization revisions.
type ScopeState struct{ Desired, Applied int64 }

// StoredResource identifies one persistent resource generation.
type StoredResource struct{ UUID, Kind, Identifier string }

// ProjectionChange contains a durable revision of relationship changes.
type ProjectionChange struct {
	Revision        int64
	Writes, Deletes []Tuple
}

func executeState(ctx context.Context, tx *sql.Tx, query *goqu.InsertDataset) error {
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-STATE-BUILDSQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, statement, args...); err != nil {
		return fmt.Errorf("REBAC-STATE-EXECUTE: %w", err)
	}
	return nil
}

// Initialize creates or checks the configuration identity of a shared scope.
func (s *StateStore) Initialize(ctx context.Context, hash string) error {
	if s.DB == nil || s.Scope == "" || hash == "" {
		return errors.New("REBAC-STATE-INIT invalid scope configuration")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("REBAC-STATE-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	query := stateDialect.Insert("rebac_scope").Rows(goqu.Record{"scope": s.Scope, "configuration_hash": hash}).OnConflict(goqu.DoNothing())
	if err = executeState(ctx, tx, query); err != nil {
		return err
	}
	statement, args, err := stateDialect.From("rebac_scope").Select("configuration_hash").Where(goqu.C("scope").Eq(s.Scope)).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-STATE-INITSQL: %w", err)
	}
	var current string
	if err = tx.QueryRowContext(ctx, statement, args...).Scan(&current); err != nil {
		return fmt.Errorf("REBAC-STATE-INITHASH: %w", err)
	}
	if current != hash {
		return ErrScopeConflict
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("REBAC-STATE-INITCOMMIT: %w", err)
	}
	return nil
}

// Lock retains the scope barrier until the caller commits or rolls back tx.
func (s *StateStore) Lock(ctx context.Context, tx *sql.Tx, exclusive bool) (ScopeState, error) {
	query := stateDialect.From("rebac_scope").Select("desired_revision", "applied_revision").Where(goqu.C("scope").Eq(s.Scope))
	if exclusive {
		query = query.ForUpdate(goqu.Wait)
	} else {
		query = query.ForShare(goqu.Wait)
	}
	statement, args, err := query.Prepared(true).ToSQL()
	var state ScopeState
	if err != nil {
		return state, fmt.Errorf("REBAC-STATE-LOCKSQL: %w", err)
	}
	if err = tx.QueryRowContext(ctx, statement, args...).Scan(&state.Desired, &state.Applied); err != nil {
		return state, fmt.Errorf("REBAC-STATE-LOCK: %w", err)
	}
	if s.ExpectedIntegrationState != "" || s.recoveryMode {
		if err = s.validateIntegrationState(ctx, tx); err != nil {
			return state, err
		}
	}
	return state, nil
}

// WithApplied holds a shared barrier while using a fully applied projection.
func (s *StateStore) WithApplied(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("REBAC-STATE-READBEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := s.Lock(ctx, tx, false)
	if err != nil {
		return err
	}
	if state.Desired != state.Applied {
		return ErrPending
	}
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("REBAC-STATE-READCOMMIT: %w", err)
	}
	return nil
}

// CreateResource must be called in the transaction that creates the resource.
func (s *StateStore) CreateResource(ctx context.Context, tx *sql.Tx, kind, identifier string, parent *string) (StoredResource, error) {
	resource := StoredResource{UUID: uuid.NewString(), Kind: kind, Identifier: identifier}
	object, err := ResourceObject(s.Scope, kind, resource.UUID)
	if err != nil {
		return resource, err
	}
	_, objectKey, found := strings.Cut(object, ":")
	if !found || objectKey == "" {
		return resource, fmt.Errorf("REBAC-STATE-OBJECTKEY malformed resource object")
	}
	query := stateDialect.Insert("rebac_resource").Rows(goqu.Record{"resource_uuid": resource.UUID, "scope": s.Scope, "kind": kind, "identifier": identifier, "object_key": objectKey, "parent_uuid": parent})
	return resource, executeState(ctx, tx, query)
}

// Resource looks up the current generation of a resource.
func (s *StateStore) Resource(ctx context.Context, tx *sql.Tx, kind, identifier string) (StoredResource, error) {
	var resource StoredResource
	statement, args, err := stateDialect.From("rebac_resource").Select("resource_uuid", "kind", "identifier").Where(goqu.Ex{"scope": s.Scope, "kind": kind, "identifier": identifier, "deleted_at": nil}).Prepared(true).ToSQL()
	if err != nil {
		return resource, fmt.Errorf("REBAC-STATE-RESOURCEQUERY: %w", err)
	}
	if err = tx.QueryRowContext(ctx, statement, args...).Scan(&resource.UUID, &resource.Kind, &resource.Identifier); err != nil {
		return resource, fmt.Errorf("REBAC-STATE-RESOURCE: %w", err)
	}
	return resource, nil
}

// Enqueue serializes desired relationship changes with resource mutations.
func (s *StateStore) Enqueue(ctx context.Context, tx *sql.Tx, writes, deletes []Tuple) (int64, error) {
	state, err := s.Lock(ctx, tx, true)
	if err != nil {
		return 0, err
	}
	if state.Desired != state.Applied {
		return s.mergeTransactionChange(ctx, tx, state.Desired, writes, deletes)
	}
	if err = s.applyDesired(ctx, tx, writes, deletes); err != nil {
		return 0, err
	}
	revision := state.Desired + 1
	writeJSON, err := json.Marshal(writes)
	if err != nil {
		return 0, fmt.Errorf("REBAC-STATE-WRITESJSON: %w", err)
	}
	deleteJSON, err := json.Marshal(deletes)
	if err != nil {
		return 0, fmt.Errorf("REBAC-STATE-DELETESJSON: %w", err)
	}
	query := stateDialect.Insert("rebac_outbox").Rows(goqu.Record{"scope": s.Scope, "revision": revision, "writes": string(writeJSON), "deletes": string(deleteJSON)})
	if err = executeState(ctx, tx, query); err != nil {
		return 0, err
	}
	statement, args, err := stateDialect.Update("rebac_scope").Set(goqu.Record{"desired_revision": revision}).Where(goqu.C("scope").Eq(s.Scope)).Prepared(true).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("REBAC-STATE-REVISIONSQL: %w", err)
	}
	if _, err = tx.ExecContext(ctx, statement, args...); err != nil {
		return 0, fmt.Errorf("REBAC-STATE-REVISION: %w", err)
	}
	return revision, nil
}

func (s *StateStore) applyDesired(ctx context.Context, tx *sql.Tx, writes, deletes []Tuple) error {
	for _, tuple := range deletes {
		statement, args, err := stateDialect.Delete("rebac_relationship").Where(goqu.Ex{"scope": s.Scope, "subject": tuple.User, "relation": tuple.Relation, "object": tuple.Object}).Prepared(true).ToSQL()
		if err != nil {
			return fmt.Errorf("REBAC-STATE-DELETESQL: %w", err)
		}
		if _, err = tx.ExecContext(ctx, statement, args...); err != nil {
			return fmt.Errorf("REBAC-STATE-DELETE: %w", err)
		}
	}
	for _, tuple := range writes {
		query := stateDialect.Insert("rebac_relationship").Rows(goqu.Record{"scope": s.Scope, "subject": tuple.User, "relation": tuple.Relation, "object": tuple.Object}).OnConflict(goqu.DoNothing())
		if err := executeState(ctx, tx, query); err != nil {
			return err
		}
	}
	return nil
}
