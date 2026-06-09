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
)

func (s *SubmodelDatabase) appendSubmodelHistoryTx(ctx context.Context, tx *sql.Tx, submodel types.ISubmodel, changeType string, deleted bool) error {
	snapshot, err := submodelToHistorySnapshot(submodel)
	if err != nil {
		return err
	}
	return history.AppendVersionTx(ctx, tx, history.TableSubmodel, submodel.ID(), changeType, snapshot, deleted)
}

func (s *SubmodelDatabase) appendCurrentSubmodelHistoryTx(ctx context.Context, tx *sql.Tx, submodelIdentifier string, changeType string) error {
	submodel, err := s.getSubmodelByIDInTransaction(ctx, tx, submodelIdentifier, "deep", false)
	if err != nil {
		return err
	}
	return s.appendSubmodelHistoryTx(ctx, tx, submodel, changeType, false)
}

func submodelToHistorySnapshot(submodel types.ISubmodel) (map[string]any, error) {
	jsonable, err := jsonization.ToJsonable(submodel)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-HISTORY-TOJSONABLE " + err.Error())
	}
	return jsonable, nil
}
