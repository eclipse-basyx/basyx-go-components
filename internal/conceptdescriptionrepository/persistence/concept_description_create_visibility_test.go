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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestConceptDescriptionRepositoryReadPoolSelection(t *testing.T) {
	writer, writerMock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer func() { _ = writer.Close() }()
	reader, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()
	writerMock.ExpectPing()

	backend, err := NewConceptDescriptionBackendFromPools(writer, reader)
	require.NoError(t, err)
	require.Same(t, reader, backend.readDB(t.Context()))
	require.Same(t, writer, backend.readDB(common.WithWriterPostgresReads(t.Context())))
	require.NoError(t, writerMock.ExpectationsWereMet())
}

func TestGetConceptDescriptionsValidatesCursorInPageQuery(t *testing.T) {
	t.Parallel()

	matcher := sqlmock.QueryMatcherFunc(func(_ string, actualSQL string) error {
		for _, expected := range []string{
			`SELECT 1 FROM "concept_description" AS "cursor_cd"`,
			`"cursor_cd"."id" = $1`,
			`"id" >= $2`,
		} {
			if !strings.Contains(actualSQL, expected) {
				return fmt.Errorf("expected SQL to contain %q, got: %s", expected, actualSQL)
			}
		}
		return nil
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	backend, err := NewConceptDescriptionBackendFromDB(db)
	require.NoError(t, err)

	mock.ExpectQuery("cursor query").
		WithArgs("urn:cd:cursor", "urn:cd:cursor", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "id_short", "data"}))

	cursor := "urn:cd:cursor"
	items, nextCursor, err := backend.GetConceptDescriptions(
		contextWithConceptDescriptionConfig(t),
		nil,
		nil,
		nil,
		10,
		&cursor,
		time.Time{},
		time.Time{},
	)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Empty(t, nextCursor)
	require.NoError(t, mock.ExpectationsWereMet())
}

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
	mock.ExpectQuery(`SELECT "id" FROM "concept_description".*\$2`).
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

	return auth.WithQueryFilter(contextWithConceptDescriptionConfig(t), limitedCreateQueryFilterForRepositoryTests())
}

func contextWithConceptDescriptionConfig(t *testing.T) context.Context {
	t.Helper()

	cfg := &common.Config{}
	var cfgCtx context.Context
	handler := common.ConfigMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cfgCtx = r.Context()
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	require.NotNil(t, cfgCtx)
	return cfgCtx
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
