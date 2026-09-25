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

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// InheritanceService maintains direct AAS-to-Submodel inheritance relationships.
type InheritanceService struct {
	State   *StateStore
	Checker Checker
	Audit   GrantAudit
}

// ReplaceAASParents replaces the approved AAS parent links for one submodel in
// the caller transaction. Every desired link must exist in the persisted AAS
// reference data and management permission is required on the submodel and on
// all old and new AAS resources.
func (s *InheritanceService) ReplaceAASParents(ctx context.Context, tx *sql.Tx, actor GrantActor, submodel StoredResource, expected int64, aasIdentifiers []string) (int64, error) {
	if s.State == nil || tx == nil || submodel.Kind != "submodel" {
		return 0, fmt.Errorf("REBAC-INHERITANCE-CONFIG state, transaction, and submodel resource are required")
	}
	if err := s.State.CheckMutationRevision(ctx, tx, expected); err != nil {
		return 0, err
	}
	if err := s.State.RequireLiveResource(ctx, tx, submodel); err != nil {
		return 0, err
	}
	submodelObject, err := ResourceObject(s.State.Scope, submodel.Kind, submodel.UUID)
	if err != nil {
		return 0, err
	}
	desiredIdentifiers, err := uniqueAASIdentifiers(aasIdentifiers)
	if err != nil {
		return 0, err
	}
	referenced, err := s.referencedAASIdentifiers(ctx, tx, submodel.Identifier, desiredIdentifiers)
	if err != nil {
		return 0, err
	}
	if len(referenced) != len(desiredIdentifiers) {
		return 0, fmt.Errorf("REBAC-INHERITANCE-REFERENCE every AAS parent must reference the submodel")
	}
	desired, err := s.aasParentTuples(ctx, tx, submodelObject, desiredIdentifiers)
	if err != nil {
		return 0, err
	}
	existing, err := s.aasParentTuplesForSubmodel(ctx, tx, submodelObject)
	if err != nil {
		return 0, err
	}
	if err = s.authorizeInheritance(ctx, actor, submodelObject, existing, desired); err != nil {
		return 0, err
	}
	writes, deletes := tupleDifference(desired, existing), tupleDifference(existing, desired)
	revision, err := s.State.QueueChanged(ctx, tx, writes, deletes)
	if err != nil {
		return 0, err
	}
	return revision, s.auditInheritanceChange(ctx, tx, actor, submodelObject, writes, deletes, revision)
}

// RemoveUnreferencedInheritance removes AAS parent links whose source AAS no
// longer references the linked submodel. It must run in the source AAS change
// transaction after its reference rows have been updated.
func (s *InheritanceService) RemoveUnreferencedInheritance(ctx context.Context, tx *sql.Tx, aasIdentifier string) (int64, error) {
	if s.State == nil || tx == nil || aasIdentifier == "" {
		return 0, fmt.Errorf("REBAC-INHERITANCE-CLEANUPCONFIG state, transaction, and AAS identifier are required")
	}
	aas, err := s.State.Resource(ctx, tx, "aas", aasIdentifier)
	if err != nil {
		return 0, err
	}
	aasObject, err := ResourceObject(s.State.Scope, aas.Kind, aas.UUID)
	if err != nil {
		return 0, err
	}
	existing, err := s.aasParentTuplesForAAS(ctx, tx, aasObject)
	if err != nil {
		return 0, err
	}
	valid, err := s.currentAASParentTuples(ctx, tx, aasObject, aasIdentifier)
	if err != nil {
		return 0, err
	}
	return s.State.QueueChanged(ctx, tx, nil, tupleDifference(existing, valid))
}

func uniqueAASIdentifiers(identifiers []string) ([]string, error) {
	seen := make(map[string]struct{}, len(identifiers))
	result := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		if identifier == "" {
			return nil, fmt.Errorf("REBAC-INHERITANCE-IDENTIFIER AAS identifier must not be empty")
		}
		if _, exists := seen[identifier]; exists {
			return nil, fmt.Errorf("REBAC-INHERITANCE-DUPLICATE duplicate AAS parent")
		}
		seen[identifier] = struct{}{}
		result = append(result, identifier)
	}
	sort.Strings(result)
	return result, nil
}

func (s *InheritanceService) referencedAASIdentifiers(ctx context.Context, tx *sql.Tx, submodelIdentifier string, aasIdentifiers []string) (map[string]struct{}, error) {
	if len(aasIdentifiers) == 0 {
		return map[string]struct{}{}, nil
	}
	key := goqu.T("aas_submodel_reference_key").As("reference_key")
	reference := goqu.T("aas_submodel_reference").As("reference")
	aas := goqu.T("aas").As("aas")
	query := stateDialect.From(key).Select(goqu.I("aas.aas_id")).InnerJoin(reference, goqu.On(goqu.I("reference.id").Eq(goqu.I("reference_key.reference_id")))).InnerJoin(aas, goqu.On(goqu.I("aas.id").Eq(goqu.I("reference.aas_id")))).Where(goqu.I("reference_key.value").Eq(submodelIdentifier), goqu.I("reference_key.type").Eq(int(types.KeyTypesSubmodel)), goqu.I("aas.aas_id").In(aasIdentifiers)).Distinct()
	return readAASIdentifiers(ctx, tx, query)
}

func (s *InheritanceService) aasParentTuples(ctx context.Context, tx *sql.Tx, submodelObject string, aasIdentifiers []string) ([]Tuple, error) {
	tuples := make([]Tuple, 0, len(aasIdentifiers))
	for _, identifier := range aasIdentifiers {
		aas, err := s.State.Resource(ctx, tx, "aas", identifier)
		if err != nil {
			return nil, err
		}
		aasObject, err := ResourceObject(s.State.Scope, aas.Kind, aas.UUID)
		if err != nil {
			return nil, err
		}
		tuples = append(tuples, Tuple{User: aasObject, Relation: "aas_parent", Object: submodelObject})
	}
	return tuples, nil
}

func (s *InheritanceService) aasParentTuplesForSubmodel(ctx context.Context, tx *sql.Tx, submodelObject string) ([]Tuple, error) {
	return s.inheritanceTuples(ctx, tx, goqu.C("object").Eq(submodelObject))
}

func (s *InheritanceService) aasParentTuplesForAAS(ctx context.Context, tx *sql.Tx, aasObject string) ([]Tuple, error) {
	return s.inheritanceTuples(ctx, tx, goqu.C("subject").Eq(aasObject))
}

func (s *InheritanceService) inheritanceTuples(ctx context.Context, tx *sql.Tx, predicate exp.Expression) ([]Tuple, error) {
	query := stateDialect.From("rebac_relationship").Select("subject", "relation", "object").Where(goqu.Ex{"scope": s.State.Scope, "relation": "aas_parent"}).Where(predicate).Order(goqu.C("subject").Asc(), goqu.C("object").Asc())
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-INHERITANCE-SELECTSQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-INHERITANCE-SELECT: %w", err)
	}
	defer func() { _ = rows.Close() }()
	tuples := []Tuple{}
	for rows.Next() {
		var tuple Tuple
		if err = rows.Scan(&tuple.User, &tuple.Relation, &tuple.Object); err != nil {
			return nil, fmt.Errorf("REBAC-INHERITANCE-SCAN: %w", err)
		}
		tuples = append(tuples, tuple)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-INHERITANCE-ROWS: %w", err)
	}
	return tuples, nil
}

func (s *InheritanceService) authorizeInheritance(ctx context.Context, actor GrantActor, submodelObject string, old, desired []Tuple) error {
	authorizer := GrantService{Checker: s.Checker}
	if err := authorizer.authorize(ctx, actor, submodelObject); err != nil {
		return err
	}
	objects := make(map[string]struct{}, len(old)+len(desired))
	for _, tuple := range append(old, desired...) {
		objects[tuple.User] = struct{}{}
	}
	for object := range objects {
		if err := authorizer.authorize(ctx, actor, object); err != nil {
			return err
		}
	}
	return nil
}

func (s *InheritanceService) currentAASParentTuples(ctx context.Context, tx *sql.Tx, aasObject, aasIdentifier string) ([]Tuple, error) {
	key := goqu.T("aas_submodel_reference_key").As("reference_key")
	reference := goqu.T("aas_submodel_reference").As("reference")
	aas := goqu.T("aas").As("aas")
	query := stateDialect.From(key).Select(goqu.I("reference_key.value")).InnerJoin(reference, goqu.On(goqu.I("reference.id").Eq(goqu.I("reference_key.reference_id")))).InnerJoin(aas, goqu.On(goqu.I("aas.id").Eq(goqu.I("reference.aas_id")))).Where(goqu.I("aas.aas_id").Eq(aasIdentifier), goqu.I("reference_key.type").Eq(int(types.KeyTypesSubmodel))).Distinct()
	submodels, err := readAASIdentifiers(ctx, tx, query)
	if err != nil {
		return nil, err
	}
	tuples := make([]Tuple, 0, len(submodels))
	for identifier := range submodels {
		resource, resourceErr := s.State.Resource(ctx, tx, "submodel", identifier)
		if resourceErr != nil {
			if errors.Is(resourceErr, sql.ErrNoRows) {
				continue
			}
			return nil, resourceErr
		}
		object, objectErr := ResourceObject(s.State.Scope, resource.Kind, resource.UUID)
		if objectErr != nil {
			return nil, objectErr
		}
		tuples = append(tuples, Tuple{User: aasObject, Relation: "aas_parent", Object: object})
	}
	return tuples, nil
}

func readAASIdentifiers(ctx context.Context, tx *sql.Tx, query *goqu.SelectDataset) (map[string]struct{}, error) {
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-INHERITANCE-REFERENCESQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-INHERITANCE-REFERENCE: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := map[string]struct{}{}
	for rows.Next() {
		var identifier string
		if err = rows.Scan(&identifier); err != nil {
			return nil, fmt.Errorf("REBAC-INHERITANCE-REFERENCESCAN: %w", err)
		}
		result[identifier] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-INHERITANCE-REFERENCEROWS: %w", err)
	}
	return result, nil
}

func (s *InheritanceService) auditInheritanceChange(ctx context.Context, tx *sql.Tx, actor GrantActor, object string, writes, deletes []Tuple, revision int64) error {
	if s.Audit == nil || len(writes)+len(deletes) == 0 {
		return nil
	}
	if err := s.Audit(ctx, tx, actor, object, writes, deletes, revision); err != nil {
		return fmt.Errorf("REBAC-INHERITANCE-AUDIT: %w", err)
	}
	return nil
}
