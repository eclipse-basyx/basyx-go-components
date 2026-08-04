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
	"database/sql/driver"
	"strconv"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

type postgresInt64Array []int64

type postgresTextArray []string

func (values postgresInt64Array) Value() (driver.Value, error) {
	array := make([]byte, 0, 2+len(values)*4)
	array = append(array, '{')
	for index, value := range values {
		if index > 0 {
			array = append(array, ',')
		}
		array = strconv.AppendInt(array, value, 10)
	}
	array = append(array, '}')
	return string(array), nil
}

func (values postgresTextArray) Value() (driver.Value, error) {
	array := make([]byte, 0, 2+len(values)*8)
	array = append(array, '{')
	for index, value := range values {
		if index > 0 {
			array = append(array, ',')
		}
		array = append(array, '"')
		for _, character := range []byte(value) {
			if character == '\\' || character == '"' {
				array = append(array, '\\')
			}
			array = append(array, character)
		}
		array = append(array, '"')
	}
	array = append(array, '}')
	return string(array), nil
}

// PostgreSQLBigIntArrayContains builds a stable SQL membership expression for
// a PostgreSQL bigint array parameter.
func PostgreSQLBigIntArrayContains(column exp.Expression, values []int64) exp.Expression {
	return goqu.L("? = ANY(?::bigint[])", column, postgresInt64Array(values))
}

// PostgreSQLTextArrayContains builds a stable SQL membership expression for
// a PostgreSQL text array parameter.
func PostgreSQLTextArrayContains(column exp.Expression, values []string) exp.Expression {
	return goqu.L("? = ANY(?::text[])", column, postgresTextArray(values))
}

// PostgreSQLBigIntArrayPosition returns the one-based position of value in a
// PostgreSQL bigint array parameter.
func PostgreSQLBigIntArrayPosition(values []int64, value exp.Expression) exp.LiteralExpression {
	return goqu.L("array_position(?::bigint[], ?)", postgresInt64Array(values), value)
}
