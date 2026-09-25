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
	"strings"

	"github.com/doug-martin/goqu/v9"
)

// Snapshot returns direct grants, inheritance relationships, and projection
// revisions when the caller currently has management authority.
func (g *GrantService) Snapshot(ctx context.Context, actor GrantActor, resource StoredResource) (GrantSnapshot, error) {
	if g.State == nil || g.State.DB == nil {
		return GrantSnapshot{}, fmt.Errorf("REBAC-GRANTS-SNAPSHOTCONFIG state store is required")
	}
	object, err := ResourceObject(g.State.Scope, resource.Kind, resource.UUID)
	if err != nil {
		return GrantSnapshot{}, err
	}
	tx, err := g.State.DB.BeginTx(ctx, nil)
	if err != nil {
		return GrantSnapshot{}, fmt.Errorf("REBAC-GRANTS-SNAPSHOTBEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	state, err := g.State.Lock(ctx, tx, false)
	if err != nil {
		return GrantSnapshot{}, err
	}
	if state.Desired != state.Applied {
		return GrantSnapshot{}, ErrPending
	}
	if err = g.State.RequireLiveResource(ctx, tx, resource); err != nil {
		return GrantSnapshot{}, err
	}
	if err = g.authorize(ctx, actor, object); err != nil {
		return GrantSnapshot{}, err
	}
	grants, err := g.State.SnapshotDirectGrants(ctx, tx, resource.Kind, object)
	if err != nil {
		return GrantSnapshot{}, err
	}
	inheritance, err := g.State.InheritanceRelationships(ctx, tx, object)
	if err != nil {
		return GrantSnapshot{}, err
	}
	if err = tx.Commit(); err != nil {
		return GrantSnapshot{}, fmt.Errorf("REBAC-GRANTS-SNAPSHOTCOMMIT: %w", err)
	}
	return GrantSnapshot{Revision: state.Desired, AppliedRevision: state.Applied, Grants: grants, Inheritance: inheritance}, nil
}

// StoreGrantPrincipals records direct-grant identities in the caller transaction.
func (s *StateStore) StoreGrantPrincipals(ctx context.Context, tx *sql.Tx, grants []Grant) error {
	seen := make(map[string]struct{}, len(grants))
	for _, grant := range grants {
		subject, err := grant.Principal.Object()
		if err != nil {
			return fmt.Errorf("REBAC-GRANTS-PRINCIPAL: %w", err)
		}
		if _, exists := seen[subject]; exists {
			continue
		}
		seen[subject] = struct{}{}
		if err = s.SavePrincipal(ctx, tx, grant.Principal); err != nil {
			return fmt.Errorf("REBAC-GRANTS-PRINCIPAL: %w", err)
		}
	}
	return nil
}

// SnapshotDirectGrants resolves explicitly assigned roles to their original principals.
func (s *StateStore) SnapshotDirectGrants(ctx context.Context, tx *sql.Tx, resourceKind, object string) ([]Grant, error) {
	tuples, err := s.DirectGrants(ctx, tx, object)
	if err != nil {
		return nil, err
	}
	principals, err := s.grantPrincipals(ctx, tx, tuples)
	if err != nil {
		return nil, err
	}
	grants := make([]Grant, 0, len(tuples))
	for _, tuple := range tuples {
		grant, err := snapshotGrant(resourceKind, tuple, principals)
		if err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	return grants, nil
}

func (s *StateStore) grantPrincipals(ctx context.Context, tx *sql.Tx, tuples []Tuple) (map[string]Principal, error) {
	subjects := make([]string, 0, len(tuples))
	seen := make(map[string]struct{}, len(tuples))
	for _, tuple := range tuples {
		subject := strings.TrimSuffix(tuple.User, "#member")
		if _, exists := seen[subject]; exists {
			continue
		}
		seen[subject] = struct{}{}
		subjects = append(subjects, subject)
	}
	if len(subjects) == 0 {
		return map[string]Principal{}, nil
	}
	query := stateDialect.From("rebac_principal").Select("subject", "kind", "issuer", "identifier").Where(goqu.Ex{"scope": s.Scope}).Where(goqu.C("subject").In(subjects))
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-GRANTS-PRINCIPALSQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-GRANTS-PRINCIPALS: %w", err)
	}
	defer func() { _ = rows.Close() }()
	principals := make(map[string]Principal, len(subjects))
	for rows.Next() {
		var subject string
		var principal Principal
		if err = rows.Scan(&subject, &principal.Kind, &principal.Issuer, &principal.ID); err != nil {
			return nil, fmt.Errorf("REBAC-GRANTS-PRINCIPALSCAN: %w", err)
		}
		principals[subject] = principal
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-GRANTS-PRINCIPALROWS: %w", err)
	}
	return principals, nil
}

func snapshotGrant(resourceKind string, tuple Tuple, principals map[string]Principal) (Grant, error) {
	subject := strings.TrimSuffix(tuple.User, "#member")
	principal, found := principals[subject]
	if !found {
		return Grant{}, fmt.Errorf("REBAC-GRANTS-PRINCIPAL missing metadata for direct grant")
	}
	expectedSubject, err := principal.Object()
	if err != nil || expectedSubject != subject {
		return Grant{}, fmt.Errorf("REBAC-GRANTS-PRINCIPAL invalid principal metadata")
	}
	if principal.Kind == "group" && tuple.User != subject+"#member" {
		return Grant{}, fmt.Errorf("REBAC-GRANTS-PRINCIPAL group grant must use member userset")
	}
	if principal.Kind == "user" && tuple.User != subject {
		return Grant{}, fmt.Errorf("REBAC-GRANTS-PRINCIPAL user grant has unexpected userset")
	}
	if tuple.Relation == "creator" && resourceKind != "repository" {
		return Grant{}, fmt.Errorf("REBAC-GRANTS-ROLE creator is only supported for repositories")
	}
	return Grant{Principal: principal, Role: tuple.Relation}, nil
}

// InheritanceRelationships loads the direct topology relationships for an object.
func (s *StateStore) InheritanceRelationships(ctx context.Context, tx *sql.Tx, object string) ([]Tuple, error) {
	query := stateDialect.From("rebac_relationship").Select("subject", "relation", "object").Where(goqu.Ex{"scope": s.Scope, "object": object}).Where(goqu.C("relation").In("parent", "aas_parent", "source")).Order(goqu.C("relation").Asc(), goqu.C("subject").Asc())
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-GRANTS-INHERITSQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-GRANTS-INHERIT: %w", err)
	}
	defer func() { _ = rows.Close() }()
	relationships := []Tuple{}
	for rows.Next() {
		var relationship Tuple
		if err = rows.Scan(&relationship.User, &relationship.Relation, &relationship.Object); err != nil {
			return nil, fmt.Errorf("REBAC-GRANTS-INHERITSCAN: %w", err)
		}
		relationships = append(relationships, relationship)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-GRANTS-INHERITROWS: %w", err)
	}
	return relationships, nil
}
