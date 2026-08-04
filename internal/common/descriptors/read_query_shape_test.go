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

package descriptors

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestDescriptorBatchReadReusesSQLShape(t *testing.T) {
	t.Parallel()

	build := func(ids []int64) string {
		var query string
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
			query = actual
			return nil
		})))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("descriptor batch read").
			WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "extensions_payload"}))
		_, err = ReadExtensionsByDescriptorIDs(context.Background(), db, ids)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
		return query
	}

	oneIDQuery := build([]int64{1})
	manyIDsQuery := build([]int64{1, 2, 3})

	require.Equal(t, oneIDQuery, manyIDsQuery)
	require.Contains(t, oneIDQuery, `= ANY($1::bigint[])`)
}

func TestListSubmodelDescriptorsForAASReusesLookupSQLShape(t *testing.T) {
	t.Parallel()

	build := func(aasID string) string {
		var query string
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
			query = actual
			return nil
		})))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery("AAS descriptor lookup").
			WithArgs(aasID, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"descriptor_id"}))
		descriptors, cursor, err := ListSubmodelDescriptorsForAAS(context.Background(), db, aasID, 100, "")
		require.NoError(t, err)
		require.Empty(t, descriptors)
		require.Empty(t, cursor)
		require.NoError(t, mock.ExpectationsWereMet())
		return query
	}

	firstQuery := build("urn:example:aas:first")
	secondQuery := build("urn:example:aas:second")

	require.Equal(t, firstQuery, secondQuery)
	require.Equal(t, `SELECT "aas"."descriptor_id" FROM "aas_descriptor" AS "aas" WHERE ("aas"."id" = $1) LIMIT $2`, firstQuery)
}

func TestReadSubmodelDescriptorsByAASDescriptorIDsNonMatchFilterCorrelatesToParentDescriptor(t *testing.T) {
	field := grammar.ModelStringPattern("$aasdesc#submodelDescriptors[].idShort")
	value := grammar.StandardString("matching-sibling")
	fragment := grammar.FragmentStringPattern("$aasdesc#submodelDescriptors[]")
	condition := grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			{Field: &field},
			{StrVal: &value},
		},
	}
	ctx := auth.WithQueryFilter(context.Background(), &auth.QueryFilter{
		Filters: auth.FragmentFilters{fragment: auth.NewFragmentFilterPredicate(condition, false)},
	})

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
		if !strings.Contains(actual, `EXISTS`) {
			return fmt.Errorf("expected non-MATCH filter to use EXISTS, got: %s", actual)
		}
		if !strings.Contains(actual, `"submodel_descriptor"."aas_descriptor_id" = "descriptor"."id"`) {
			return fmt.Errorf("expected filter correlation to parent AAS descriptor, got: %s", actual)
		}
		return nil
	})))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("nested submodel descriptor non-MATCH correlation").
		WillReturnRows(sqlmock.NewRows([]string{
			"c0",
			"c1",
			"c2",
			"c3",
			"c4",
			"c5",
			"c6",
			"c7",
		}))

	descriptors, err := ReadSubmodelDescriptorsByAASDescriptorIDs(ctx, db, []int64{12}, false)
	require.NoError(t, err)
	require.Nil(t, descriptors[12])
	require.NoError(t, mock.ExpectationsWereMet())
}
