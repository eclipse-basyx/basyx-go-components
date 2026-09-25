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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// ReconcileReport summarizes one reconciliation run.
type ReconcileReport struct {
	OrphanGrants      int `json:"orphanGrants"`
	OrphanLinks       int `json:"orphanLinks"`
	OrphanDerivations int `json:"orphanDerivations"`
	OrphanInvitations int `json:"orphanInvitations"`
}

// ReconcileOrphans removes state that no longer applies: grants and
// invitations of deleted resources, links whose reference is gone and
// derivations whose object or source is gone. Such
// state appears when resources change while ReBAC is disabled. It never
// grants access, because authorization UUIDs are not reused and links are
// validated against live references, so this is housekeeping.
func (c *Coordinator) ReconcileOrphans(ctx context.Context) (ReconcileReport, error) {
	report := ReconcileReport{}
	err := common.ExecuteInTransaction(c.db, "REBAC-RECONCILE-STARTTX", "REBAC-RECONCILE-COMMIT", func(tx *sql.Tx) error {
		grants, err := deleteMatching(ctx, tx, "REBAC-RECONCILE-GRANTS", dialect.Delete(grantTable).
			Where(orphanResourceCondition(goqu.I(grantTable+".object_type"), goqu.I(grantTable+".object_uuid"))))
		if err != nil {
			return err
		}
		links, err := deleteMatching(ctx, tx, "REBAC-RECONCILE-LINKS", dialect.Delete("rebac_submodel_link").
			Where(goqu.L("NOT EXISTS (?)", submodelReferenceExists("rebac_submodel_link"))))
		if err != nil {
			return err
		}
		derivations, err := deleteMatching(ctx, tx, "REBAC-RECONCILE-DERIVATIONS", dialect.Delete(derivationTable).
			Where(goqu.Or(
				orphanResourceCondition(goqu.I(derivationTable+".object_type"), goqu.I(derivationTable+".object_uuid")),
				orphanResourceCondition(goqu.I(derivationTable+".source_type"), goqu.I(derivationTable+".source_uuid")),
			)))
		if err != nil {
			return err
		}
		invitations, err := deleteMatching(ctx, tx, "REBAC-RECONCILE-INVITATIONS", dialect.Delete(invitationTable).
			Where(orphanResourceCondition(goqu.I(invitationTable+".object_type"), goqu.I(invitationTable+".object_uuid"))))
		report = ReconcileReport{OrphanGrants: grants, OrphanLinks: links, OrphanDerivations: derivations, OrphanInvitations: invitations}
		return err
	})
	return report, err
}

func deleteMatching(ctx context.Context, tx *sql.Tx, code string, ds *goqu.DeleteDataset) (int, error) {
	result, err := execDataset(ctx, tx, code, ds.Prepared(true))
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s-ROWS: %w", code, err)
	}
	return int(affected), nil
}

// resourceExists matches rows of table whose auth_uuid equals uuidColumn.
func resourceExists(table string, uuidColumn exp.IdentifierExpression) *goqu.SelectDataset {
	alias := "existing_" + table
	return dialect.From(goqu.T(table).As(alias)).Select(goqu.L("1")).
		Where(goqu.I(alias + ".auth_uuid").Eq(uuidColumn))
}

// orphanResourceCondition matches rows referencing a resource that no
// longer exists. Element rows reference their Submodel.
func orphanResourceCondition(typeColumn exp.IdentifierExpression, uuidColumn exp.IdentifierExpression) exp.Expression {
	conditions := make([]exp.Expression, 0, len(AllKinds))
	for _, kind := range AllKinds {
		types := []any{kind.ObjectType}
		if kind.ObjectType == TypeSubmodel {
			types = append(types, TypeElement)
		}
		conditions = append(conditions, goqu.And(typeColumn.In(types...), goqu.L("NOT EXISTS (?)", resourceExists(kind.AuthTable, uuidColumn))))
	}
	return goqu.Or(conditions...)
}

// submodelReferenceExists matches when the shell of linkAlias still
// references the Submodel of linkAlias.
func submodelReferenceExists(linkAlias string) *goqu.SelectDataset {
	return submodelReferenceQuery().Where(
		goqu.I("ref_aas.auth_uuid").Eq(goqu.I(linkAlias+".aas_uuid")),
		goqu.I("ref_sm.auth_uuid").Eq(goqu.I(linkAlias+".submodel_uuid")),
	)
}

func submodelReferenceQuery() *goqu.SelectDataset {
	return dialect.From(goqu.T("aas").As("ref_aas")).
		InnerJoin(goqu.T("aas_submodel_reference").As("ref"), goqu.On(goqu.I("ref.aas_id").Eq(goqu.I("ref_aas.id")))).
		InnerJoin(goqu.T("aas_submodel_reference_key").As("ref_key"), goqu.On(goqu.I("ref_key.reference_id").Eq(goqu.I("ref.id")))).
		InnerJoin(goqu.T("submodel").As("ref_sm"), goqu.On(goqu.I("ref_sm.submodel_identifier").Eq(goqu.I("ref_key.value")))).
		Select(goqu.L("1"))
}

// shellsReferencing selects the authorization UUIDs of shells referencing
// Submodels by identifier. identifiers is one identifier or a query, so the
// selection also works after the Submodel row was deleted.
func shellsReferencing(identifiers any) *goqu.SelectDataset {
	return dialect.From(goqu.T("aas").As("ref_aas")).
		InnerJoin(goqu.T("aas_submodel_reference").As("ref"), goqu.On(goqu.I("ref.aas_id").Eq(goqu.I("ref_aas.id")))).
		InnerJoin(goqu.T("aas_submodel_reference_key").As("ref_key"), goqu.On(goqu.I("ref_key.reference_id").Eq(goqu.I("ref.id")))).
		Select(goqu.I("ref_aas.auth_uuid")).
		Where(goqu.I("ref_key.value").In(identifiers))
}

// submodelIdentifiers selects the identifiers of the Submodels in sources.
func submodelIdentifiers(sources *goqu.SelectDataset) *goqu.SelectDataset {
	return dialect.From(goqu.T("submodel").As("source_sm")).
		Select(goqu.I("source_sm.submodel_identifier")).
		Where(goqu.I("source_sm.auth_uuid").In(sources))
}

// SubmodelReferenced reports whether the shell references the Submodel.
func SubmodelReferenced(ctx context.Context, q Queryer, aasUUID string, submodelUUID string) (bool, error) {
	ds := submodelReferenceQuery().Where(
		goqu.I("ref_aas.auth_uuid").Eq(goqu.L("?::uuid", aasUUID)),
		goqu.I("ref_sm.auth_uuid").Eq(goqu.L("?::uuid", submodelUUID)),
	).Limit(1).Prepared(true)
	var marker int
	return queryRowDataset(ctx, q, "REBAC-SUBMODELREFERENCED", ds, &marker)
}
