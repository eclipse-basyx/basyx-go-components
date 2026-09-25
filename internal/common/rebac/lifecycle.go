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
	"time"

	"github.com/doug-martin/goqu/v9"
)

// LifecycleActor carries trusted identity and the revision checked for a mutation.
type LifecycleActor struct {
	User     string
	Revision int64
}
type lifecycleContextKey struct{}

// ContextWithLifecycleActor propagates a validated initiating identity to transactional writers.
func ContextWithLifecycleActor(ctx context.Context, actor LifecycleActor) context.Context {
	return context.WithValue(ctx, lifecycleContextKey{}, actor)
}

// LifecycleActorFromContext retrieves validated mutation state, including in asynchronous workers.
func LifecycleActorFromContext(ctx context.Context) (LifecycleActor, bool) {
	actor, ok := ctx.Value(lifecycleContextKey{}).(LifecycleActor)
	return actor, ok
}

// ElementIdentifier preserves boundaries between a Submodel ID and an element path.
func ElementIdentifier(submodelID, path string) string {
	data, _ := json.Marshal([]string{submodelID, path})
	return string(data)
}

// EnsureResource preserves an existing generation and assigns ownership only for a new resource.
func (s *StateStore) EnsureResource(ctx context.Context, tx *sql.Tx, kind, id, parent, creator string) (StoredResource, []Tuple, error) {
	resource, err := s.Resource(ctx, tx, kind, id)
	if err == nil {
		return resource, nil, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return resource, nil, err
	}
	var parentID *string
	if parent != "" {
		parentID = &parent
	}
	resource, err = s.CreateResource(ctx, tx, kind, id, parentID)
	if err != nil {
		return resource, nil, err
	}
	object, err := ResourceObject(s.Scope, kind, resource.UUID)
	if err != nil {
		return resource, nil, err
	}
	tuples := []Tuple{}
	if creator != "" {
		tuples = append(tuples, Tuple{User: creator, Relation: "owner", Object: object})
	}
	return resource, tuples, nil
}

// RetireResource revokes all direct and incoming links without reusing its identity.
func (s *StateStore) RetireResource(ctx context.Context, tx *sql.Tx, resource StoredResource) ([]Tuple, error) {
	object, err := ResourceObject(s.Scope, resource.Kind, resource.UUID)
	if err != nil {
		return nil, err
	}
	tuples, err := s.Relationships(ctx, tx, object, true)
	if err != nil {
		return nil, err
	}
	query, args, err := stateDialect.Update("rebac_resource").Set(goqu.Record{"deleted_at": time.Now().UTC()}).Where(goqu.Ex{"scope": s.Scope, "resource_uuid": resource.UUID}).Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-LIFECYCLE-RETIREQUERY: %w", err)
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("REBAC-LIFECYCLE-RETIRE: %w", err)
	}
	return tuples, nil
}

// Relationships returns inheritance and direct grants for a concrete object.
func (s *StateStore) Relationships(ctx context.Context, tx *sql.Tx, object string, incoming bool) ([]Tuple, error) {
	predicate := goqu.C("object").Eq(object)
	query := stateDialect.From("rebac_relationship").Select("subject", "relation", "object").Where(goqu.C("scope").Eq(s.Scope))
	if incoming {
		query = query.Where(goqu.Or(predicate, goqu.C("subject").Eq(object)))
	} else {
		query = query.Where(predicate)
	}
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-LIFECYCLE-LINKQUERY: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-LIFECYCLE-LINKS: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := []Tuple{}
	for rows.Next() {
		var tuple Tuple
		if err = rows.Scan(&tuple.User, &tuple.Relation, &tuple.Object); err != nil {
			return nil, fmt.Errorf("REBAC-LIFECYCLE-SCAN: %w", err)
		}
		result = append(result, tuple)
	}
	return result, rows.Err()
}

// CheckMutationRevision prevents committing a mutation checked against stale permissions.
func (s *StateStore) CheckMutationRevision(ctx context.Context, tx *sql.Tx, expected int64) error {
	state, err := s.Lock(ctx, tx, true)
	if err != nil {
		return err
	}
	if state.Desired == state.Applied && state.Applied == expected {
		return nil
	}
	if state.Applied != expected {
		return ErrStaleRevision
	}
	statement, args, err := stateDialect.From("rebac_outbox").Select(goqu.L("transaction_id = txid_current()")).Where(goqu.Ex{"scope": s.Scope, "revision": state.Desired}).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-LIFECYCLE-REVISIONQUERY: %w", err)
	}
	var own bool
	if err = tx.QueryRowContext(ctx, statement, args...).Scan(&own); err != nil {
		return fmt.Errorf("REBAC-LIFECYCLE-REVISION: %w", err)
	}
	if !own {
		return ErrPending
	}
	return nil
}

// RequireLiveResource prevents management of an identity retired after resolution.
func (s *StateStore) RequireLiveResource(ctx context.Context, tx *sql.Tx, resource StoredResource) error {
	current, err := s.Resource(ctx, tx, resource.Kind, resource.Identifier)
	if err != nil {
		return err
	}
	if current.UUID != resource.UUID {
		return ErrStaleRevision
	}
	return nil
}
