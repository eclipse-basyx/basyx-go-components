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

package common

import (
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

func TestPostgreSQLBigIntArrayExpressionsReuseSQLShape(t *testing.T) {
	t.Parallel()

	build := func(values []int64) (string, []any) {
		query, args, err := goqu.Dialect("postgres").
			From("resource").
			Select(goqu.I("id"), PostgreSQLBigIntArrayPosition(values, goqu.I("id"))).
			Where(PostgreSQLBigIntArrayContains(goqu.I("id"), values)).
			Prepared(true).
			ToSQL()
		require.NoError(t, err)
		return query, args
	}

	oneValueQuery, oneValueArgs := build([]int64{1})
	manyValuesQuery, manyValuesArgs := build([]int64{1, 2, 3})

	require.Equal(t, oneValueQuery, manyValuesQuery)
	require.Len(t, oneValueArgs, 2)
	require.Len(t, manyValuesArgs, 2)
	require.Contains(t, oneValueQuery, "ANY($")
	require.Contains(t, oneValueQuery, "array_position($")
}

func TestPostgreSQLTextArrayContainsReusesSQLShape(t *testing.T) {
	t.Parallel()

	build := func(values []string) (string, []any) {
		query, args, err := goqu.Dialect("postgres").
			From("resource").
			Select(goqu.I("id")).
			Where(PostgreSQLTextArrayContains(goqu.I("value"), values)).
			Prepared(true).
			ToSQL()
		require.NoError(t, err)
		return query, args
	}

	oneValueQuery, oneValueArgs := build([]string{"one"})
	manyValuesQuery, manyValuesArgs := build([]string{"one", `two"quoted`, `three\escaped`})

	require.Equal(t, oneValueQuery, manyValuesQuery)
	require.Equal(t, `SELECT "id" FROM "resource" WHERE "value" = ANY($1::text[])`, oneValueQuery)
	require.Len(t, oneValueArgs, 1)
	require.Len(t, manyValuesArgs, 1)
	require.Equal(t, `{"one","two\"quoted","three\\escaped"}`, manyValuesArgs[0])
}
