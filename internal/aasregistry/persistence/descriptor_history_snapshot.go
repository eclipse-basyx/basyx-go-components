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

package aasregistrydatabase

import (
	"context"
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const descriptorSubmodelsSnapshotField = "submodelDescriptors"

func (p *PostgreSQLAASRegistryDatabase) appendMutatedDescriptorHistoryTx(ctx context.Context, tx *sql.Tx, aasID string, mutate history.SnapshotMutator) error {
	err := history.AppendMutatedVersionTx(ctx, tx, history.TableDescriptor, aasID, history.ChangeUpdated, mutate)
	if err == nil || !common.IsErrNotFound(err) {
		return err
	}

	parent, err := descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasID)
	if err != nil {
		return err
	}
	return appendDescriptorHistoryTx(ctx, tx, parent, history.ChangeUpdated, false)
}

func (p *PostgreSQLAASRegistryDatabase) appendAddedSubmodelDescriptorHistoryTx(ctx context.Context, tx *sql.Tx, aasID string, submodel model.SubmodelDescriptor) error {
	jsonable, err := submodel.ToJsonable()
	if err != nil {
		return common.NewInternalServerError("AASREG-HISTORY-SMDESC-TOJSONABLE " + err.Error())
	}
	return p.appendMutatedDescriptorHistoryTx(ctx, tx, aasID, func(snapshot map[string]any) error {
		return history.AppendSnapshotArrayItem(snapshot, descriptorSubmodelsSnapshotField, jsonable)
	})
}

func (p *PostgreSQLAASRegistryDatabase) appendReplacedSubmodelDescriptorHistoryTx(ctx context.Context, tx *sql.Tx, aasID string, submodel model.SubmodelDescriptor) error {
	jsonable, err := submodel.ToJsonable()
	if err != nil {
		return common.NewInternalServerError("AASREG-HISTORY-SMDESC-TOJSONABLE " + err.Error())
	}
	return p.appendMutatedDescriptorHistoryTx(ctx, tx, aasID, func(snapshot map[string]any) error {
		return history.ReplaceSnapshotArrayItem(snapshot, descriptorSubmodelsSnapshotField, snapshotSubmodelDescriptorMatchesID(submodel.Id), jsonable)
	})
}

func (p *PostgreSQLAASRegistryDatabase) appendRemovedSubmodelDescriptorHistoryTx(ctx context.Context, tx *sql.Tx, aasID string, submodelID string) error {
	return p.appendMutatedDescriptorHistoryTx(ctx, tx, aasID, func(snapshot map[string]any) error {
		return history.RemoveSnapshotArrayItem(snapshot, descriptorSubmodelsSnapshotField, snapshotSubmodelDescriptorMatchesID(submodelID))
	})
}

func snapshotSubmodelDescriptorMatchesID(submodelID string) history.SnapshotArrayItemMatcher {
	return func(submodel map[string]any) bool {
		return submodel["id"] == submodelID
	}
}
