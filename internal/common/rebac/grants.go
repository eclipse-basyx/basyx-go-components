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
	"errors"
	"fmt"
	"sort"

	"github.com/doug-martin/goqu/v9"
)

// ErrStaleRevision indicates an outdated grant revision.
var ErrStaleRevision = errors.New("REBAC-GRANTS-STALE authorization revision changed")

// ErrLastOwner prevents ordinary callers from removing all direct owners.
var ErrLastOwner = errors.New("REBAC-GRANTS-LASTOWNER a direct owner must remain")

// ErrForbidden indicates missing management authority.
var ErrForbidden = errors.New("REBAC-GRANTS-FORBIDDEN management permission required")

// Grant assigns a fixed role to an issuer-scoped principal.
type Grant struct {
	Principal Principal `json:"principal"`
	Role      string    `json:"role"`
}

// GrantActor contains trusted caller identity and administrator status.
type GrantActor struct {
	User          string
	Groups        []Tuple
	Administrator bool
}

// GrantSnapshot describes desired grants and projection progress.
type GrantSnapshot struct {
	Revision        int64   `json:"revision"`
	AppliedRevision int64   `json:"appliedRevision"`
	Grants          []Grant `json:"grants"`
	Inheritance     []Tuple `json:"inheritance"`
}

// GrantAudit records a grant mutation in the caller transaction.
type GrantAudit func(context.Context, *sql.Tx, GrantActor, string, []Tuple, []Tuple, int64) error

// GrantService serializes grant changes with the authorization projection.
type GrantService struct {
	State   *StateStore
	Checker Checker
	Audit   GrantAudit
}

// Replace atomically replaces direct grants at an expected revision.
func (g *GrantService) Replace(ctx context.Context, actor GrantActor, resource StoredResource, expected int64, grants []Grant) (int64, error) {
	object, err := ResourceObject(g.State.Scope, resource.Kind, resource.UUID)
	if err != nil {
		return 0, err
	}
	desired, err := grantTuples(resource.Kind, object, grants)
	if err != nil {
		return 0, err
	}
	tx, err := g.State.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("REBAC-GRANTS-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	revision, err := g.replaceTx(ctx, tx, actor, resource, object, expected, grants, desired)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("REBAC-GRANTS-COMMIT: %w", err)
	}
	return revision, nil
}

func (g *GrantService) replaceTx(ctx context.Context, tx *sql.Tx, actor GrantActor, resource StoredResource, object string, expected int64, grants []Grant, desired []Tuple) (int64, error) {
	state, err := g.State.Lock(ctx, tx, true)
	if err != nil {
		return 0, err
	}
	if state.Desired != expected {
		return 0, ErrStaleRevision
	}
	if state.Desired != state.Applied {
		return 0, ErrPending
	}
	if err = g.State.RequireLiveResource(ctx, tx, resource); err != nil {
		return 0, err
	}
	if err = g.authorize(ctx, actor, object); err != nil {
		return 0, err
	}
	previous, err := g.State.DirectGrants(ctx, tx, object)
	if err != nil {
		return 0, err
	}
	if !actor.Administrator && !hasOwner(desired) {
		return 0, ErrLastOwner
	}
	if err = g.State.StoreGrantPrincipals(ctx, tx, grants); err != nil {
		return 0, err
	}
	writes, deletes := tupleDifference(desired, previous), tupleDifference(previous, desired)
	if len(writes)+len(deletes) == 0 {
		return state.Desired, nil
	}
	revision, err := g.State.Enqueue(ctx, tx, writes, deletes)
	if err != nil {
		return 0, err
	}
	if g.Audit != nil {
		if err = g.Audit(ctx, tx, actor, object, writes, deletes, revision); err != nil {
			return 0, fmt.Errorf("REBAC-GRANTS-AUDIT: %w", err)
		}
	}
	return revision, nil
}

func (g *GrantService) authorize(ctx context.Context, actor GrantActor, object string) error {
	if actor.Administrator {
		return nil
	}
	if g.Checker == nil {
		return fmt.Errorf("REBAC-GRANTS-CHECKER authorization checker is required")
	}
	allowed, err := g.Checker.Check(ctx, actor.User, "manage", object, actor.Groups)
	if err != nil {
		return fmt.Errorf("REBAC-GRANTS-CHECK: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// DirectGrants loads only explicitly assigned resource roles.
func (s *StateStore) DirectGrants(ctx context.Context, tx *sql.Tx, object string) ([]Tuple, error) {
	query := stateDialect.From("rebac_relationship").Select("subject", "relation", "object").Where(goqu.Ex{"scope": s.Scope, "object": object, "provenance": "direct"}).Where(goqu.C("relation").In("viewer", "editor", "executor", "owner", "creator")).Order(goqu.C("subject").Asc(), goqu.C("relation").Asc())
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-GRANTS-SELECTSQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-GRANTS-SELECT: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := []Tuple{}
	for rows.Next() {
		var tuple Tuple
		if err = rows.Scan(&tuple.User, &tuple.Relation, &tuple.Object); err != nil {
			return nil, fmt.Errorf("REBAC-GRANTS-SCAN: %w", err)
		}
		result = append(result, tuple)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-GRANTS-ROWS: %w", err)
	}
	return result, nil
}

func grantTuples(resourceKind, object string, grants []Grant) ([]Tuple, error) {
	result := make([]Tuple, 0, len(grants))
	seen := map[Tuple]bool{}
	for _, grant := range grants {
		switch grant.Role {
		case "viewer", "editor", "executor", "owner":
		case "creator":
			if resourceKind != "repository" {
				return nil, fmt.Errorf("REBAC-GRANTS-ROLE creator is only supported for repositories")
			}
		default:
			return nil, fmt.Errorf("REBAC-GRANTS-ROLE unsupported role")
		}
		subject, err := grant.Principal.Object()
		if err != nil {
			return nil, err
		}
		if grant.Principal.Kind == "group" {
			subject += "#member"
		}
		tuple := Tuple{User: subject, Relation: grant.Role, Object: object}
		if seen[tuple] {
			return nil, fmt.Errorf("REBAC-GRANTS-DUPLICATE duplicate grant")
		}
		seen[tuple] = true
		result = append(result, tuple)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].User == result[j].User {
			return result[i].Relation < result[j].Relation
		}
		return result[i].User < result[j].User
	})
	return result, nil
}

func hasOwner(tuples []Tuple) bool {
	for _, tuple := range tuples {
		if tuple.Relation == "owner" {
			return true
		}
	}
	return false
}
func tupleDifference(left, right []Tuple) []Tuple {
	existing := map[Tuple]bool{}
	for _, tuple := range right {
		existing[tuple] = true
	}
	result := []Tuple{}
	for _, tuple := range left {
		if !existing[tuple] {
			result = append(result, tuple)
		}
	}
	return result
}
