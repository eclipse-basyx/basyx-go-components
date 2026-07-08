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
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestAASRepositoryCreateExistingUnauthorizedShellDoesNotReturnConflict(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &AssetAdministrationShellDatabase{db: db}
	aasID := "urn:example:aas:hidden-existing"
	assetInformation := types.NewAssetInformation(types.AssetKindInstance)
	globalAssetID := "urn:example:asset:hidden-existing"
	assetInformation.SetGlobalAssetID(&globalAssetID)
	aas := types.NewAssetAdministrationShell(aasID, assetInformation)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "id" FROM "aas"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectQuery(`SELECT "id" FROM "aas"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	mock.ExpectQuery(`FROM "aas" AS "aas".*FALSE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	err = sut.CreateAssetAdministrationShell(contextWithRestrictedCreateAAS(t), aas)
	require.Error(t, err)
	require.Truef(t, common.IsErrDenied(err), "got %v", err)
	require.False(t, common.IsErrConflict(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func contextWithRestrictedCreateAAS(t *testing.T) context.Context {
	t.Helper()

	return auth.WithQueryFilter(aasSigningTestContext(t), limitedCreateQueryFilterForRepositoryTests())
}

func limitedCreateQueryFilterForRepositoryTests() *auth.QueryFilter {
	denied := false
	expr := grammar.LogicalExpression{Boolean: &denied}
	return &auth.QueryFilter{
		Formula: &expr,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumCREATE: expr,
		},
	}
}
