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
	"context"
	"io"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestStreamFileAttachmentDoesNotExposeABACHiddenElement(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	sut := &SubmodelDatabase{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "id" FROM "submodel"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery(`SELECT .*"sme"\."id".*FROM "submodel_element" AS "sme"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*"sme"\."id".*FROM "submodel_element" AS "sme".*FALSE`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	consumed := false

	err = sut.StreamFileAttachmentWithContext(restrictedReadSubmodelContext(t), "submodel-id", "File", func(string, string, int64, io.Reader) error {
		consumed = true
		return nil
	})

	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.False(t, consumed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func restrictedReadSubmodelContext(t *testing.T) context.Context {
	t.Helper()
	denied := false
	expression := grammar.LogicalExpression{Boolean: &denied}
	return auth.WithQueryFilter(contextWithABACDisabled(t), &auth.QueryFilter{
		Formula: &expression,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: expression,
		},
	})
}
