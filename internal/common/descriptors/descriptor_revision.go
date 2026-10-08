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

package descriptors

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/conditional"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// touchRevisions records that tx changes the revisions of resources of one
// kind. The new revisions are written when the transaction commits.
func touchRevisions(ctx context.Context, tx *sql.Tx, kind conditional.Kind, op conditional.Operation, identifiers ...string) error {
	for _, identifier := range identifiers {
		if err := conditional.Touch(ctx, tx, conditional.Ref(kind, identifier), op); err != nil {
			return common.NewInternalServerError("DESC-TOUCHREVISION-" + string(kind) + " " + err.Error())
		}
	}
	return nil
}

// TouchAdministrationShellDescriptorTx records a change of an AAS
// descriptor, including changes of its embedded Submodel descriptors.
func TouchAdministrationShellDescriptorTx(ctx context.Context, tx *sql.Tx, aasID string, op conditional.Operation) error {
	return touchRevisions(ctx, tx, conditional.KindAASDescriptor, op, aasID)
}

// TouchSubmodelDescriptorTx records a change of a global Submodel descriptor.
func TouchSubmodelDescriptorTx(ctx context.Context, tx *sql.Tx, submodelID string, op conditional.Operation) error {
	return touchRevisions(ctx, tx, conditional.KindSubmodelDescriptor, op, submodelID)
}

// touchCreatedResourcesTx records created descriptors by their ReBAC
// resource kind.
func touchCreatedResourcesTx(ctx context.Context, tx *sql.Tx, resource auth.SemanticResourceKind, identifiers ...string) error {
	kind, ok := revisionKindOf(resource)
	if !ok {
		return nil
	}
	if err := touchRevisions(ctx, tx, kind, conditional.OpCreate, identifiers...); err != nil {
		return err
	}
	if kind == conditional.KindAASDescriptor {
		return touchIntegratedDiscoveryTx(ctx, tx, identifiers...)
	}
	return nil
}

// CreatedAdministrationShellDescriptorRevisions returns the resources whose
// revisions change when AAS descriptors are created: the descriptors and,
// with discovery integration, their discovery entries.
func CreatedAdministrationShellDescriptorRevisions(ctx context.Context, aasIDs ...string) []conditional.ResourceRef {
	refs := make([]conditional.ResourceRef, 0, 2*len(aasIDs))
	for _, aasID := range aasIDs {
		refs = append(refs, conditional.Ref(conditional.KindAASDescriptor, aasID))
	}
	if cfg, ok := common.ConfigFromContext(ctx); ok && cfg.General.DiscoveryIntegration {
		for _, aasID := range aasIDs {
			refs = append(refs, conditional.Ref(conditional.KindDiscoveryEntry, aasID))
		}
	}
	return refs
}

func revisionKindOf(resource auth.SemanticResourceKind) (conditional.Kind, bool) {
	switch resource {
	case auth.SemanticResourceAASDesc:
		return conditional.KindAASDescriptor, true
	case auth.SemanticResourceSMDesc:
		return conditional.KindSubmodelDescriptor, true
	default:
		return "", false
	}
}

// touchIntegratedDiscoveryTx records a change of the discovery entries that
// the discovery integration maintains for shell descriptors.
func touchIntegratedDiscoveryTx(ctx context.Context, tx *sql.Tx, aasIDs ...string) error {
	if !discoveryIntegrated(ctx) {
		return nil
	}
	return touchRevisions(ctx, tx, conditional.KindDiscoveryEntry, conditional.OpUpdate, aasIDs...)
}

func discoveryIntegrated(ctx context.Context) bool {
	cfg, ok := common.ConfigFromContext(ctx)
	return ok && cfg.General.DiscoveryIntegration
}

// TouchDiscoveryEntryTx records a change of the asset links of an AAS.
func TouchDiscoveryEntryTx(ctx context.Context, tx *sql.Tx, aasID string, op conditional.Operation) error {
	return touchRevisions(ctx, tx, conditional.KindDiscoveryEntry, op, aasID)
}

// recordAdministrationShellDescriptorsDeletedTx records deleted AAS
// descriptors. With discovery integration, their asset links change too.
// Without a precondition to evaluate, that revision write joins batch, which
// must already contain the deletes.
func recordAdministrationShellDescriptorsDeletedTx(
	ctx context.Context,
	tx *sql.Tx,
	batch *common.PostgreSQLBatch,
	aasIDs ...string,
) error {
	if err := touchRevisions(ctx, tx, conditional.KindAASDescriptor, conditional.OpDelete, aasIDs...); err != nil {
		return err
	}
	if !discoveryIntegrated(ctx) {
		return nil
	}
	if conditional.EvaluatesPreconditions(ctx) {
		return touchRevisions(ctx, tx, conditional.KindDiscoveryEntry, conditional.OpUpdate, aasIDs...)
	}
	refs := make([]conditional.ResourceRef, 0, len(aasIDs))
	for _, aasID := range aasIDs {
		refs = append(refs, conditional.Ref(conditional.KindDiscoveryEntry, aasID))
	}
	if err := batch.AppendDataset(conditional.RevisionUpsertDataset(refs...)); err != nil {
		return common.NewInternalServerError("DESC-RECORDDELETED-BUILDREVISION " + err.Error())
	}
	return nil
}

// UpdateOperation returns the revision operation of an update that changed
// the resource or left it unchanged.
func UpdateOperation(changed bool) conditional.Operation {
	if changed {
		return conditional.OpUpdate
	}
	return conditional.OpNoOp
}

// UpdateGlobalSubmodelDescriptorTx applies full PUT semantics to a global
// Submodel descriptor and records the change of its revision.
func UpdateGlobalSubmodelDescriptorTx(
	ctx context.Context,
	tx *sql.Tx,
	descriptorID int64,
	previous model.SubmodelDescriptor,
	next model.SubmodelDescriptor,
) (bool, error) {
	changed, err := UpdateSubmodelDescriptorTx(ctx, tx, descriptorID, previous, next, 0, false)
	if err != nil {
		return false, err
	}
	return changed, TouchSubmodelDescriptorTx(ctx, tx, next.Id, UpdateOperation(changed))
}

// UpdateEmbeddedSubmodelDescriptorTx applies full PUT semantics to a
// Submodel descriptor of an AAS descriptor and records the change of the
// AAS descriptor's revision.
func UpdateEmbeddedSubmodelDescriptorTx(
	ctx context.Context,
	tx *sql.Tx,
	aasID string,
	descriptorID int64,
	previous model.SubmodelDescriptor,
	next model.SubmodelDescriptor,
	position int,
) (bool, error) {
	changed, err := UpdateSubmodelDescriptorTx(ctx, tx, descriptorID, previous, next, position, false)
	if err != nil {
		return false, err
	}
	return changed, TouchAdministrationShellDescriptorTx(ctx, tx, aasID, UpdateOperation(changed))
}

// DeleteDiscoveryEntryTx deletes the discovery entry of aasID with all its
// asset links and reports whether it existed. When some links also belonged
// to the shell descriptor, the descriptor change is recorded too. One
// statement does both, so the delete needs no additional round trip.
func DeleteDiscoveryEntryTx(ctx context.Context, tx *sql.Tx, aasID string) (bool, error) {
	d := goqu.Dialect(common.Dialect)
	identifier := goqu.T(common.TblAASIdentifier)
	ownedLinks := d.From(common.TSpecificAssetID).Select(goqu.L("1")).Where(
		common.TSpecificAssetID.Col(common.ColAASRef).Eq(identifier.Col(common.ColID)),
		common.TSpecificAssetID.Col(common.ColDescriptorID).IsNotNull(),
	)
	query, args, err := d.Delete(identifier).
		Where(identifier.Col("aasid").Eq(aasID)).
		Returning(goqu.L("EXISTS ?", ownedLinks)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return false, common.NewInternalServerError("DISC-DELETEENTRY-BUILDSQL " + err.Error())
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return false, common.NewInternalServerError("DISC-DELETEENTRY-EXECSQL " + err.Error())
	}
	deleted, ownedByDescriptor, err := readDeletedEntry(rows)
	if err != nil {
		return false, common.NewInternalServerError("DISC-DELETEENTRY-READSQL " + err.Error())
	}
	if !deleted || !ownedByDescriptor {
		return deleted, nil
	}
	return true, TouchAdministrationShellDescriptorTx(ctx, tx, aasID, conditional.OpUpdate)
}

func readDeletedEntry(rows *sql.Rows) (bool, bool, error) {
	defer func() { _ = rows.Close() }()
	deleted, owned := false, false
	for rows.Next() {
		deleted = true
		if err := rows.Scan(&owned); err != nil {
			return false, false, err
		}
	}
	return deleted, owned, rows.Err()
}
