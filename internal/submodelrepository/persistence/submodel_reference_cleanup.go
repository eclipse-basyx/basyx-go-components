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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

// DeleteUnreferencedSubmodelInTransaction removes a submodel only when no other live model references it.
func (s *SubmodelDatabase) DeleteUnreferencedSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodelID string) (bool, error) {
	if tx == nil {
		return false, common.NewInternalServerError("SMREPO-DELORPHAN-NILTX transaction must not be nil")
	}
	if err := history.LockMutationTx(ctx, tx, history.TableSubmodel, submodelID); err != nil {
		return false, err
	}
	id, err := lockSubmodelForReferenceCleanup(ctx, tx, submodelID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-DELORPHAN-LOCK " + err.Error())
	}
	referenced, err := submodelHasInboundReferences(ctx, tx, submodelID, id)
	if err != nil || referenced {
		return false, err
	}
	if err := s.deleteSubmodelInTransaction(ctx, tx, submodelID); err != nil {
		return false, err
	}
	return true, nil
}

func lockSubmodelForReferenceCleanup(ctx context.Context, tx *sql.Tx, submodelID string) (int64, error) {
	query, args, err := goqu.Dialect("postgres").From("submodel").Select("id").
		Where(goqu.Ex{"submodel_identifier": submodelID}).ForUpdate(goqu.Wait).Prepared(true).ToSQL()
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, query, args...).Scan(&id)
	return id, err
}

func submodelHasInboundReferences(ctx context.Context, tx *sql.Tx, identifier string, id int64) (bool, error) {
	query, args, err := goqu.Dialect("postgres").From("submodel_inbound_reference").Select("source_id").
		Where(goqu.C("target_id").Eq(identifier), goqu.Or(goqu.C("owner_submodel_id").IsNull(), goqu.C("owner_submodel_id").Neq(id))).
		Limit(1).Prepared(true).ToSQL()
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-DELORPHAN-BUILDREFERENCES " + err.Error())
	}
	var found int64
	err = tx.QueryRowContext(ctx, query, args...).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-DELORPHAN-READREFERENCES " + err.Error())
	}
	return true, nil
}
