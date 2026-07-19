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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"context"
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func (s *SubmodelDatabase) appendSubmodelHistoryTx(ctx context.Context, tx *sql.Tx, submodel types.ISubmodel, previousSnapshot map[string]any, changeType string, deleted bool) error {
	snapshot, err := submodelToHistorySnapshot(submodel)
	if err != nil {
		return err
	}
	return history.AppendVersionTx(ctx, tx, history.TableSubmodel, submodel.ID(), changeType, previousSnapshot, snapshot, deleted)
}

func (s *SubmodelDatabase) appendCurrentSubmodelHistoryTx(ctx context.Context, tx *sql.Tx, submodelIdentifier string, previousSnapshot map[string]any, changeType string) error {
	stateReadCtx := ctx
	if history.ActiveConfig().EvidenceEnabled {
		stateReadCtx = auth.ContextWithoutQueryFilter(ctx)
	}
	submodel, err := s.getSubmodelByIDInTransaction(stateReadCtx, tx, submodelIdentifier, "deep", false)
	if err != nil {
		return err
	}
	return s.appendSubmodelHistoryTx(ctx, tx, submodel, previousSnapshot, changeType, false)
}

func (s *SubmodelDatabase) loadSubmodelHistorySnapshotBeforeMutationTx(ctx context.Context, tx *sql.Tx, submodelIdentifier string) (map[string]any, error) {
	if !history.ActiveConfig().EvidenceEnabled {
		return nil, nil
	}
	if err := history.LockMutationTx(ctx, tx, history.TableSubmodel, submodelIdentifier); err != nil {
		return nil, err
	}
	submodel, err := s.getSubmodelByIDInTransaction(auth.ContextWithoutQueryFilter(ctx), tx, submodelIdentifier, "deep", false)
	if err != nil {
		return nil, err
	}
	return submodelToHistorySnapshot(submodel)
}

func submodelToHistorySnapshot(submodel types.ISubmodel) (map[string]any, error) {
	jsonable, err := jsonization.ToJsonable(submodel)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-HISTORY-TOJSONABLE " + err.Error())
	}
	return jsonable, nil
}
