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
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestConceptDescriptionRepositoryCreateExistingUnauthorizedConceptDescriptionDoesNotReturnConflict(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	sut := &ConceptDescriptionBackend{db: db}
	conceptDescription := types.NewConceptDescription("urn:example:cd:hidden-existing")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT 1 FROM "concept_description"`).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectQuery(`SELECT 1 FROM "concept_description"`).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectQuery(`SELECT "id" FROM "concept_description".*FALSE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	err = sut.CreateConceptDescription(contextWithRestrictedCreateConceptDescription(t), conceptDescription)
	require.Error(t, err)
	require.True(t, common.IsErrDenied(err))
	require.False(t, common.IsErrConflict(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func contextWithRestrictedCreateConceptDescription(t *testing.T) context.Context {
	t.Helper()

	cfg := &common.Config{}
	var cfgCtx context.Context
	handler := common.ConfigMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cfgCtx = r.Context()
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	require.NotNil(t, cfgCtx)
	return auth.WithQueryFilter(cfgCtx, limitedCreateQueryFilterForRepositoryTests())
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
