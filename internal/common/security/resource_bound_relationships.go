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
	"net/http"
	"sort"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// ResourceBoundReferencesTx protects changes to the AAS containment context.
func ResourceBoundReferencesTx(ctx context.Context, tx *sql.Tx, aasID string, references []types.IReference, replace bool) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	previous, err := boundReferenceIDs(ctx, tx, aasID)
	if err != nil {
		return err
	}
	next := map[string]bool{}
	for _, ref := range references {
		if ref.Type() == types.ReferenceTypesModelReference && len(ref.Keys()) == 1 && ref.Keys()[0].Type() == types.KeyTypesSubmodel {
			next[ref.Keys()[0].Value()] = true
		}
	}
	changed := changedBoundReferences(previous, next, replace)
	for _, id := range changed {
		if err = state.checkRelationship(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

func changedBoundReferences(previous, next map[string]bool, replace bool) []string {
	changed := []string{}
	for id := range next {
		if !previous[id] {
			changed = append(changed, id)
		}
	}
	if replace {
		for id := range previous {
			if !next[id] {
				changed = append(changed, id)
			}
		}
	}
	sort.Strings(changed)
	return changed
}

func boundReferenceIDs(ctx context.Context, tx *sql.Tx, aasID string) (map[string]bool, error) {
	ds := goqu.Dialect("postgres").From(goqu.T("aas_submodel_reference").As("r")).Join(goqu.T("aas_submodel_reference_key").As("k"), goqu.On(goqu.I("k.reference_id").Eq(goqu.I("r.id")))).Join(goqu.T("aas").As("a"), goqu.On(goqu.I("a.id").Eq(goqu.I("r.aas_id")))).Select(goqu.I("k.value")).Where(goqu.Ex{"a.aas_id": aasID, "k.position": 0, "k.type": int(types.KeyTypesSubmodel), "r.type": int(types.ReferenceTypesModelReference)})
	query, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-REFERENCES-BUILD %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-REFERENCES-QUERY %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("REBAC-REFERENCES-SCAN %w", err)
		}
		ids[id] = true
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-REFERENCES-ROWS %w", err)
	}
	return ids, nil
}

// ResourceBoundUnlinkTx checks administration before removing an inheritance edge.
func ResourceBoundUnlinkTx(ctx context.Context, tx *sql.Tx, submodelID string) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	return state.checkRelationship(ctx, tx, submodelID)
}

func (state *boundRequest) checkRelationship(ctx context.Context, tx *sql.Tx, submodelID string) error {
	query, args, err := goqu.Dialect("postgres").From("submodel").Select("id").Where(goqu.Ex{"submodel_identifier": submodelID}).ForUpdate(goqu.Wait).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-RELATIONSHIP-BUILD %w", err)
	}
	var id int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return boundError(http.StatusConflict, "RELATIONSHIP referenced Submodel must exist locally")
	}
	if err != nil {
		return fmt.Errorf("REBAC-RELATIONSHIP-LOCK %w", err)
	}
	target := boundTarget{Kind: "submodel", Submodel: submodelID}
	access, err := state.repo.load(ctx, tx, target)
	if err != nil {
		return err
	}
	actor, err := boundActor(ctx)
	if err != nil {
		return err
	}
	if containsBoundPrincipal(access.Owners, actor) {
		return nil
	}
	target.AAS = state.target.AAS
	effective, err := state.repo.effective(ctx, tx, target)
	if err != nil {
		return err
	}
	if effective != nil && containsBoundPrincipal(effective.Managers, actor) {
		return nil
	}
	return boundError(http.StatusForbidden, "RELATIONSHIP Submodel administration required to change inheritance")
}

// AddResourceBoundReferenceFilter omits undiscoverable Submodels before reference pagination.
func AddResourceBoundReferenceFilter(ctx context.Context, ds *goqu.SelectDataset, alias string) (*goqu.SelectDataset, error) {
	state := boundRequestFromContext(ctx)
	if state == nil || (state.right != grammar.RightsEnumREAD && state.right != grammar.RightsEnumVIEW) {
		return ds, nil
	}
	input := resourceBoundReferenceInput(state)
	view, err := state.repo.requestState(ctx, state.repo.db, boundTarget{Kind: "submodel"}, input, grammar.RightsEnumVIEW, state.revision)
	if err != nil {
		return nil, err
	}
	view.aasContext = goqu.I(alias + ".aas_id")
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
	if err != nil {
		return nil, err
	}
	expression, err := view.expression(ctx, collector, "")
	if err != nil {
		return nil, err
	}
	keys := goqu.Dialect("postgres").From(goqu.T("aas_submodel_reference_key").As("rb_reference_key")).Select(goqu.I("rb_reference_key.value")).Where(goqu.I("rb_reference_key.reference_id").Eq(goqu.I(alias+".id")), goqu.Ex{"rb_reference_key.type": int(types.KeyTypesSubmodel), "rb_reference_key.position": 0})
	visible := goqu.Dialect("postgres").From("submodel").Select(goqu.L("1")).Where(goqu.I("submodel.submodel_identifier").In(keys), expression)
	return ds.Where(goqu.I(alias+".type").Eq(int(types.ReferenceTypesModelReference)), goqu.L("EXISTS ?", visible)), nil
}

func resourceBoundReferenceInput(state *boundRequest) EvalInput {
	input := state.input
	if input.RoutePath == "" {
		input.RoutePath = input.Path
	}
	input.Path = joinBasePath(state.repo.basePath, "/submodels")
	return input
}
