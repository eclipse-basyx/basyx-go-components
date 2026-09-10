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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func boundStorage(kind string) (table, identifierColumn string, err error) {
	switch kind {
	case "aas":
		return "aas", "aas_id", nil
	case "submodel":
		return "submodel", "submodel_identifier", nil
	case "submodel_element":
		return "submodel_element", "idshort_path", nil
	case "aas_descriptor":
		return "aas_descriptor", "id", nil
	case "submodel_descriptor":
		return "submodel_descriptor", "id", nil
	case "concept_description":
		return "concept_description", "id", nil
	case "aas_identifier":
		return "aas_identifier", "aasid", nil
	default:
		return "", "", fmt.Errorf("REBAC-STORAGE-KIND unsupported resource kind %q", kind)
	}
}

// ResourceBoundCreatedTx records ownership for newly persisted resources in the caller's transaction.
func ResourceBoundCreatedTx(ctx context.Context, tx *sql.Tx, kind, identifier string, aasContext ...string) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	table, column, err := boundStorage(kind)
	if err != nil {
		return err
	}
	actor, err := boundActor(ctx)
	if err != nil {
		return err
	}
	ids, err := insertBoundResourceForIdentifier(ctx, tx, state.repo.scope, table, column, identifier, firstBoundContext(aasContext))
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		if err = authorizeBoundCreatedBindings(ctx, tx, table, ids); err != nil {
			return err
		}
		if err = insertBoundOwners(ctx, tx, ids, actor); err != nil {
			return err
		}
	}
	if kind == "submodel" {
		query, args, buildErr := goqu.Dialect("postgres").From(table).Select("id").Where(goqu.Ex{column: identifier}).Prepared(true).ToSQL()
		if buildErr != nil {
			return fmt.Errorf("REBAC-CREATED-BUILD %w", buildErr)
		}
		var id int64
		if scanErr := tx.QueryRowContext(ctx, query, args...).Scan(&id); scanErr != nil {
			return fmt.Errorf("REBAC-CREATED-QUERY %w", scanErr)
		}
		return ResourceBoundElementsCreatedTx(ctx, tx, id)
	}
	return nil
}

// ResourceBoundDiscoveryCreatedTx records a discovery identity created as part of a registry transaction.
func ResourceBoundDiscoveryCreatedTx(ctx context.Context, tx *sql.Tx, aasIdentifier string) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	actor, err := boundActor(ctx)
	if err != nil {
		return err
	}
	ids, err := insertBoundResourceForIdentifier(ctx, tx, state.repo.scope, "aas_identifier", "aasid", aasIdentifier, "")
	if err != nil || len(ids) == 0 {
		return err
	}
	target := boundTarget{Kind: "discovery", AAS: aasIdentifier}
	parent, _, err := matchingAASParent(ctx, tx, aasIdentifier, "lookup/shells")
	if err != nil {
		return err
	}
	check, err := state.repo.requestState(ctx, tx, target, state.input, grammar.RightsEnumCREATE, state.revision)
	if err != nil {
		return err
	}
	check.creationTarget = &parent
	relatedContext := context.WithValue(ctx, boundRequestKey, check)
	if err = authorizeBoundCreatedBindings(relatedContext, tx, "aas_identifier", ids); err != nil {
		return err
	}
	return insertBoundOwners(ctx, tx, ids, actor)
}

func insertBoundResourceForIdentifier(ctx context.Context, tx *sql.Tx, scope, table, identifierColumn, identifier, aasContext string) ([]int64, error) {
	dialect := goqu.Dialect("postgres")
	accessColumn := boundForeignKey(table)
	sourceID := boundSourceID(table)
	source := dialect.From(goqu.T(table).As("source")).Select(goqu.V(scope), goqu.I("source."+sourceID)).Where(goqu.Ex{"source." + identifierColumn: identifier})
	if table == "submodel_descriptor" && aasContext != "" {
		aasDescriptor := dialect.From("aas_descriptor").Select("descriptor_id").Where(goqu.Ex{"id": aasContext})
		source = source.Where(goqu.I("source.aas_descriptor_id").Eq(aasDescriptor))
	}
	ds := dialect.Insert("rebac_access").Cols("scope", accessColumn).FromQuery(source).OnConflict(goqu.DoNothing()).Returning("id").Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-CREATED-BUILD %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-CREATED-QUERY %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("REBAC-CREATED-SCAN %w", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-CREATED-ROWS %w", err)
	}
	return ids, nil
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
func ResourceBoundPrepareMutationTx(ctx context.Context, tx *sql.Tx, kind, identifier string, aasContext ...string) error {
	state := boundRequestFromContext(ctx)
	if state == nil {
		return nil
	}
	table, column, err := boundStorage(kind)
	if err != nil {
		return err
	}
	dialect := goqu.Dialect("postgres")
	ds := dialect.From(table).Select(goqu.L("1")).Where(goqu.Ex{column: identifier})
	contextID := firstBoundContext(aasContext)
	if table == "submodel_descriptor" && contextID != "" {
		aasDescriptor := dialect.From("aas_descriptor").Select("descriptor_id").Where(goqu.Ex{"id": contextID})
		ds = ds.Where(goqu.C("aas_descriptor_id").Eq(aasDescriptor))
	}
	query, args, err := ds.ForUpdate(goqu.Wait).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-PREPARE-BUILD %w", err)
	}
	var id int64
	err = tx.QueryRowContext(ctx, query, args...).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("REBAC-PREPARE-LOCK %w", err)
	}
	exists := err == nil
	if kind == "aas" && state.right == grammar.RightsEnumDELETE && state.target.Suffix == "" {
		if err := ResourceBoundReferencesTx(ctx, tx, identifier, nil, true); err != nil {
			return err
		}
	}
	storageTarget := kind == "aas_descriptor" || kind == "submodel_descriptor" || kind == "concept_description" || kind == "aas_identifier"
	if state.creationTarget != nil && (!storageTarget || !exists) {
		return state.checkTarget(ctx, tx, *state.creationTarget)
	}
	target := state.target
	switch kind {
	case "aas_descriptor":
		target = boundTarget{Kind: kind, AAS: identifier}
	case "submodel_descriptor":
		target = boundTarget{Kind: kind, Submodel: identifier, AAS: contextID}
	case "concept_description":
		target = boundTarget{Kind: kind, Submodel: identifier}
	case "aas_identifier":
		target = boundTarget{Kind: "discovery", AAS: identifier}
	}
	check := state
	if storageTarget {
		right := grammar.RightsEnumUPDATE
		if state.right == grammar.RightsEnumDELETE {
			right = grammar.RightsEnumDELETE
		}
		check, err = state.repo.requestState(ctx, tx, target, state.input, right, state.revision)
		if err != nil {
			return err
		}
	}
	if err = check.checkTarget(ctx, tx, target); err != nil {
		return err
	}
	return check.checkDescendants(ctx, tx, target)
}

func firstBoundContext(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
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
	switch table {
	case "aas":
		root = grammar.CollectorRootAAS
	case "aas_descriptor":
		root, alias = grammar.CollectorRootAASDesc, "aas_descriptor"
	case "submodel_descriptor":
		root = grammar.CollectorRootSMDesc
	case "concept_description":
		root = grammar.CollectorRootCD
	case "aas_identifier":
		root, alias = grammar.CollectorRootBD, "aas_identifier"
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
	ds := goqu.Dialect("postgres").From(goqu.T(table).As(alias)).Select(goqu.L("1"))
	if table == "aas_descriptor" {
		ds = goqu.Dialect("postgres").From(goqu.T("descriptor").As("descriptor")).Join(goqu.T(table).As(alias), goqu.On(goqu.I(alias+".descriptor_id").Eq(goqu.I("descriptor.id")))).Select(goqu.L("1"))
	}
	ds = ds.Where(goqu.I(alias+"."+boundSourceID(table)).In(bindings), goqu.L("NOT (?)", predicate)).Limit(1)
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
