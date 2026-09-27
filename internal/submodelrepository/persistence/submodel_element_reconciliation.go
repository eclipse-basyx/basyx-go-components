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

package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// submodelElementChildrenToReconcile returns the children a write replaces.
// PUT always replaces the children of a container, PATCH only when the patch
// carries the children field.
func submodelElementChildrenToReconcile(
	modelType types.ModelType,
	submodelElement types.ISubmodelElement,
	isPut bool,
) ([]types.ISubmodelElement, bool) {
	if !submodelelements.IsContainerModelType(modelType) {
		return nil, false
	}
	children, provided := submodelelements.ProvidedChildElements(submodelElement)
	return children, isPut || provided
}

// lockSubmodelForChildReconciliationTx serializes child reconciliations of one
// Submodel. It must run before element rows are updated to keep the lock order
// of other element writes.
func lockSubmodelForChildReconciliationTx(ctx context.Context, tx *sql.Tx, submodelID string) (int, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseIDForUpdateContext(ctx, tx, submodelID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, common.NewErrNotFound("SMREPO-RECONSMECHILDREN-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
	}
	if err != nil {
		return 0, common.NewInternalServerError("SMREPO-RECONSMECHILDREN-LOCKSUBMODEL " + err.Error())
	}
	return submodelDatabaseID, nil
}

// reconcileSubmodelElementChildrenTx replaces the child subtree of the
// container at parentPath with children, writing only the differences.
// Unchanged children keep their rows.
func (s *SubmodelDatabase) reconcileSubmodelElementChildrenTx(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	submodelDatabaseID int,
	parentPath string,
	children []types.ISubmodelElement,
) error {
	parent, err := submodelelements.LoadReconciliationParentTx(ctx, tx, submodelDatabaseID, parentPath)
	if err != nil {
		return err
	}
	persistedParent, err := submodelelements.GetSubmodelElementByIDShortOrPathTx(
		auth.ContextWithoutQueryFilter(ctx), tx, submodelID, parentPath, true, "",
	)
	if err != nil {
		return err
	}
	persistedPositions, err := submodelelements.LoadPersistedPositionsTx(ctx, tx, submodelDatabaseID, parentPath)
	if err != nil {
		return err
	}
	plan, err := s.buildElementReconciliationPlan(
		submodelelements.ChildElements(persistedParent),
		children,
		parent.ChildInsertContext(),
		persistedPositions,
	)
	if err != nil {
		return err
	}
	return s.executeSubmodelReconciliationTx(ctx, tx, submodelID, plan)
}
