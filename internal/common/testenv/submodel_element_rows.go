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

package testenv

import (
	"database/sql"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

// OverwriteSubmodelElementRow overwrites columns of the type-specific row of one submodel element.
// Integration tests use it to reproduce rows written by earlier schema or code versions.
//
// Parameters:
//   - t: test whose context bounds the update
//   - dsn: PostgreSQL connection URL of the repository database
//   - submodelID: identifier of the submodel that contains the element
//   - idShortPath: idShort path of the element within the submodel
//   - table: type-specific table, for example "property_element"
//   - record: columns to overwrite
//
// Example:
//
//	testenv.OverwriteSubmodelElementRow(t, dsn, "urn:example", "Timestamp", "property_element",
//		goqu.Record{"value_text": nil, "value_datetime": "2024-01-01T10:00:00+02:00"})
func OverwriteSubmodelElementRow(t *testing.T, dsn, submodelID, idShortPath, table string, record goqu.Record) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	dialect := goqu.Dialect(common.Dialect)
	elementID := dialect.From("submodel_element").Select(goqu.I("submodel_element.id")).
		Join(goqu.T("submodel"), goqu.On(goqu.I("submodel.id").Eq(goqu.I("submodel_element.submodel_id")))).
		Where(goqu.I("submodel.submodel_identifier").Eq(submodelID), goqu.I("submodel_element.idshort_path").Eq(idShortPath))
	query, args, err := dialect.Update(table).Set(record).Where(goqu.C("id").Eq(elementID)).ToSQL()
	require.NoError(t, err)
	result, err := db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
	updated, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
}
