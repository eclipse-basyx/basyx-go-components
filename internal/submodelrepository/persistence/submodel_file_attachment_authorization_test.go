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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package persistence

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestAttachmentPutSelectsRightFromLockedTransactionState(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "update"}[exists], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectBegin()
			tx, err := db.Begin()
			require.NoError(t, err)
			mock.ExpectQuery(`SELECT "id" FROM "submodel".*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			var marker any
			if exists {
				marker = 1
			}
			mock.ExpectQuery(`SELECT "fe"\."id".*FROM "submodel"`).WillReturnRows(sqlmock.NewRows([]string{"file_element_id", "file_oid"}).AddRow(2, marker))
			allow, deny := true, false
			ctx := auth.WithQueryFilter(contextWithABACDisabled(t), &auth.QueryFilter{FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
				grammar.RightsEnumCREATE: {Boolean: &allow}, grammar.RightsEnumUPDATE: {Boolean: &deny},
			}})
			ctx = auth.SelectFormulaForRight(ctx, grammar.RightsEnumCREATE)
			selected, err := attachmentPutContext(ctx, tx, "id", "File")
			require.NoError(t, err)
			right, present := auth.SelectedFormulaRight(selected)
			require.True(t, present)
			expected := grammar.RightsEnumCREATE
			if exists {
				expected = grammar.RightsEnumUPDATE
			}
			require.Equal(t, expected, right)
			require.Equal(t, !exists, *auth.GetQueryFilter(selected).Formula.Boolean)
			mock.ExpectRollback()
			require.NoError(t, tx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAttachmentPutRejectsMissingOwnerBeforeWriting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT "id" FROM "submodel".*FOR UPDATE`).WillReturnError(sql.ErrNoRows)
	_, err = attachmentPutContext(restrictedReadSubmodelContext(t), tx, "missing", "File")
	require.True(t, common.IsErrNotFound(err))
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
