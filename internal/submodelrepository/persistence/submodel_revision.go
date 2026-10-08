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

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/conditional"
)

// touchSubmodel records that tx changes the revision of a Submodel. The new
// revision is written when the transaction commits.
func touchSubmodel(ctx context.Context, tx *sql.Tx, submodelID string, op conditional.Operation) error {
	if err := conditional.Touch(ctx, tx, conditional.Ref(conditional.KindSubmodel, submodelID), op); err != nil {
		return common.NewInternalServerError("SMREPO-TOUCHSM-REVISION " + err.Error())
	}
	return nil
}

// beginSubmodelMutationTx loads the history snapshot of a Submodel before an
// update and records the update of its revision.
func (s *SubmodelDatabase) beginSubmodelMutationTx(ctx context.Context, tx *sql.Tx, submodelID string) (map[string]any, error) {
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return nil, err
	}
	return previousSnapshot, touchSubmodel(ctx, tx, submodelID, conditional.OpUpdate)
}

// VerifyConditionalTarget evaluates If-Match of an action on a Submodel that
// does not change it, such as an operation invocation.
func (s *SubmodelDatabase) VerifyConditionalTarget(ctx context.Context) error {
	if err := conditional.VerifyTarget(ctx, s.db); err != nil {
		if conditional.IsPreconditionError(err) {
			return err
		}
		return common.NewInternalServerError("SMREPO-VERIFYTARGET-REVISION " + err.Error())
	}
	return nil
}
