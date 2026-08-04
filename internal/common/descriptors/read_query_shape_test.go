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
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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
