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
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestLoadAASDescriptorForUpdateUsesGenericPlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	mock.ExpectQuery("SELECT current_setting").WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow("auto"))
	mock.ExpectQuery("SELECT set_config.*force_generic_plan").WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow("force_generic_plan"))
	mock.ExpectQuery("SELECT jsonb_strip_nulls").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(`{"id":"aas-1","idShort":"Shell","assetKind":"Instance","description":[{"language":"en","text":"before"}]}`))
	mock.ExpectQuery("SELECT set_config.*auto").WillReturnRows(sqlmock.NewRows([]string{"mode"}).AddRow("auto"))
	descriptor, err := loadAASDescriptorForUpdateTx(common.ContextWithConfig(t.Context(), &common.Config{}), tx, "aas-1")
	require.NoError(t, err)
	require.Equal(t, "aas-1", descriptor.Id)
	require.Equal(t, "before", descriptor.Description[0].Text())
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadAASDescriptorForUpdatePreservesRestrictedAuthorization(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	allowed := false
	ctx := auth.WithQueryFilter(common.ContextWithConfig(t.Context(), &common.Config{}), &auth.QueryFilter{
		Formula: &grammar.LogicalExpression{Boolean: &allowed},
	})
	mock.ExpectQuery("SELECT jsonb_strip_nulls").WillReturnRows(sqlmock.NewRows([]string{"payload"}))
	_, err = loadAASDescriptorForUpdateTx(ctx, tx, "aas-1")
	require.True(t, common.IsErrNotFound(err))
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
