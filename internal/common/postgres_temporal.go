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

import "fmt"

// PostgreSQLTemporalColumnTextSQL renders a PostgreSQL time, date, or timestamp column
// as the ISO 8601 text produced by to_json, independent of the session DateStyle.
//
// Parameters:
//   - column: SQL expression of the temporal column, or "?" for a Goqu placeholder
//
// Returns:
//   - string: SQL fragment evaluating to the ISO 8601 text, or NULL for NULL input
//
// Example:
//
//	goqu.L(common.PostgreSQLTemporalColumnTextSQL("?"), goqu.I("pe.value_date"))
func PostgreSQLTemporalColumnTextSQL(column string) string {
	return fmt.Sprintf(`trim(both '"' from to_json(%s)::text)`, column)
}

// PostgreSQLLegacyDateTimeColumnTextSQL renders a TIMESTAMPTZ column as an xs:dateTime
// in UTC with an explicit "+00:00" offset, independent of the session TimeZone.
//
// Parameters:
//   - column: SQL expression of the TIMESTAMPTZ column, or "?" for a Goqu placeholder
//
// Returns:
//   - string: SQL fragment evaluating to the UTC xs:dateTime text, or NULL for NULL input
//
// Example:
//
//	goqu.L(common.PostgreSQLLegacyDateTimeColumnTextSQL("?"), goqu.I("pe.value_datetime"))
func PostgreSQLLegacyDateTimeColumnTextSQL(column string) string {
	return PostgreSQLTemporalColumnTextSQL(fmt.Sprintf("timezone('UTC', %s)", column)) + " || '+00:00'"
}
