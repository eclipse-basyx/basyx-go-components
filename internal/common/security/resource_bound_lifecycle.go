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

package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"

	"github.com/doug-martin/goqu/v9"
)

// ResourceBoundCreatedTx records ownership for newly persisted resources in the caller's transaction.
func ResourceBoundCreatedTx(ctx context.Context, tx *sql.Tx, kind, identifier string) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	column := kind + "_identifier"
	if kind == "aas" {
		column = "aas_id"
	}
	query, args, err := goqu.Dialect("postgres").From(kind).Select("id").Where(goqu.Ex{column: identifier}).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-CREATED-BUILD %w", err)
	}
	var id int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		return fmt.Errorf("REBAC-CREATED-QUERY %w", err)
	}
	actor, err := boundActor(ctx)
	if err != nil {
		return err
	}
	if err = seedBoundResources(ctx, tx, state.repo.scope, kind, &id, actor); err != nil {
		return err
	}
	if kind == "submodel" {
		return ResourceBoundElementsCreatedTx(ctx, tx, id)
	}
	return nil
}

// ResourceBoundElementsCreatedTx adopts only new element identities; existing ownership is unchanged.
func ResourceBoundElementsCreatedTx(ctx context.Context, tx *sql.Tx, submodelID int64) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	actor, err := boundActor(ctx)
	if err != nil {
		return err
	}
	if err := seedBoundResources(ctx, tx, state.repo.scope, "submodel", &submodelID, actor); err != nil {
		return err
	}
	return seedBoundResources(ctx, tx, state.repo.scope, "submodel_element", &submodelID, actor)
}

// ResourceBoundPrepareMutationTx repeats checks after acquiring the resource lock in the write transaction.
func ResourceBoundPrepareMutationTx(ctx context.Context, tx *sql.Tx, kind, identifier string) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	column := kind + "_identifier"
	if kind == "aas" {
		column = "aas_id"
	}
	query, args, err := goqu.Dialect("postgres").From(kind).Select("id").Where(goqu.Ex{column: identifier}).ForUpdate(goqu.Wait).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-PREPARE-BUILD %w", err)
	}
	var id int64
	err = tx.QueryRowContext(ctx, query, args...).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("REBAC-PREPARE-LOCK %w", err)
	}
	if kind == "aas" && state.right == grammar.RightsEnumDELETE && state.target.Suffix == "" {
		if err := ResourceBoundReferencesTx(ctx, tx, identifier, nil, true); err != nil {
			return err
		}
	}
	if state.creationTarget != nil {
		return state.checkTarget(ctx, tx, *state.creationTarget)
	}
	if err = state.checkTarget(ctx, tx, state.target); err != nil {
		return err
	}
	return state.checkDescendants(ctx, tx, state.target)
}

// ResourceBoundReconcileTx checks each changed element using its actual lifecycle action.
func ResourceBoundReconcileTx(ctx context.Context, tx *sql.Tx, submodelID string, creates, updates, deletes []string) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	operations := []struct {
		right grammar.RightsEnum
		paths []string
	}{
		{grammar.RightsEnumCREATE, creates}, {grammar.RightsEnumUPDATE, updates}, {grammar.RightsEnumDELETE, deletes},
	}
	for _, operation := range operations {
		if len(operation.paths) == 0 {
			continue
		}
		check, err := state.repo.requestState(ctx, tx, state.target, state.input, operation.right, state.revision)
		if err != nil {
			return err
		}
		for _, path := range operation.paths {
			target := boundTarget{Kind: "sme", Submodel: submodelID, Path: path, AAS: state.target.AAS}
			if err = check.checkElementMutation(ctx, tx, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func (state *boundRequest) checkElementMutation(ctx context.Context, tx *sql.Tx, target boundTarget) error {
	if state.right == grammar.RightsEnumCREATE {
		parent, err := state.existingCreationParent(ctx, tx, target)
		if err != nil {
			return err
		}
		state.creationTarget = &parent
		return state.checkTarget(ctx, tx, parent)
	}
	if err := state.checkTarget(ctx, tx, target); err != nil {
		return err
	}
	if state.right == grammar.RightsEnumDELETE {
		return state.checkDescendants(ctx, tx, target)
	}
	return nil
}

func (state *boundRequest) existingCreationParent(ctx context.Context, tx *sql.Tx, target boundTarget) (boundTarget, error) {
	for depth := 0; depth < 256; depth++ {
		parent, err := creationBoundParent(target)
		if err != nil {
			return parent, err
		}
		_, err = state.repo.load(ctx, tx, parent)
		if err == nil {
			return parent, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return parent, err
		}
		target = parent
	}
	return boundTarget{}, fmt.Errorf("REBAC-CREATE-DEPTH no existing parent within 256 levels")
}

// ResourceBoundRequestActive reports whether this request uses resource-bound enforcement.
func ResourceBoundRequestActive(ctx context.Context) bool { return boundRequestFromContext(ctx) != nil }

func authorizeBoundCreatedBindings(ctx context.Context, tx *sql.Tx, table string, ids []int64) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	check, err := state.repo.requestState(ctx, tx, state.target, state.input, grammar.RightsEnumCREATE, state.revision)
	if err != nil {
		return err
	}
	check.creationTarget = state.creationTarget
	root, alias := grammar.CollectorRootSM, table
	if table == "aas" {
		root = grammar.CollectorRootAAS
	}
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(root)
	if table == "submodel_element" {
		alias = "sme"
		collector, err = grammar.NewResolvedFieldPathCollectorForSMERow(alias)
	}
	if err != nil {
		return err
	}
	predicate, err := check.expression(collector, "")
	if err != nil {
		return err
	}
	bindings := goqu.Dialect("postgres").From("rebac_access").Select(boundForeignKey(table)).Where(goqu.C("id").In(ids))
	ds := goqu.Dialect("postgres").From(goqu.T(table).As(alias)).Select(goqu.L("1")).Where(goqu.I(alias+".id").In(bindings), goqu.L("NOT (?)", predicate)).Limit(1)
	query, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-CREATEDCHECK-BUILD %w", err)
	}
	var denied int
	err = tx.QueryRowContext(ctx, query, args...).Scan(&denied)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("REBAC-CREATEDCHECK-QUERY %w", err)
	}
	return boundError(403, "CREATE embedded resource is not permitted")
}
