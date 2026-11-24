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

package grammar

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// SQL patterns that normalize xs:date and xs:dateTime text before the timestamptz cast.
// Offsets may also take the "+01" form PostgreSQL uses when it renders a timestamptz column as text.
// The patterns stay free of nested optional groups because PostgreSQL evaluates those slowly.
const (
	zonelessDateTimeSQLPattern = `^([0-9]{4,}-[0-9]{2}-[0-9]{2}(?:[Tt ][0-9:.]+)?)$`
	zonedDateSQLPattern        = `^([0-9]{4,}-[0-9]{2}-[0-9]{2})([+-][0-9]{2}:[0-9]{2}|[Zz])$`
	zonedDateTimeSQLPattern    = `^[0-9]{4,}-[0-9]{2}-[0-9]{2}[Tt ][0-9:.]+(?:[Zz]|[+-][0-9]{2}(?::?[0-9]{2}){0,2})$`
)

// utcTimestampSQLValue casts an SQL value to timestamptz independently of the session TimeZone.
//
// Text without a timezone is interpreted as UTC, dates get an explicit midnight so negative
// offsets are not misread, digits beyond microseconds are truncated, and anything other
// than an ISO 8601 date or date-time yields NULL. The operand is referenced once, so nested
// casts keep the generated SQL linear in size.
func utcTimestampSQLValue(sqlValue interface{}) exp.Expression {
	text := truncatedFractionSQLText(goqu.Func("btrim", goqu.L("?::text", sqlValue)))
	explicitUTC := goqu.Func("regexp_replace", text, zonelessDateTimeSQLPattern, `\1Z`)
	explicitMidnight := goqu.Func("regexp_replace", explicitUTC, zonedDateSQLPattern, `\1T00:00:00\2`)
	isoDateTime := goqu.L("substring(? from ?)", explicitMidnight, zonedDateTimeSQLPattern)
	return goqu.L("basyx_validated_cast_input(?, ?)::timestamptz", isoDateTime, "timestamp with time zone")
}

// timeOfDaySQLValue casts an SQL value to time without time zone after truncating digits beyond microseconds.
func timeOfDaySQLValue(sqlValue interface{}) exp.Expression {
	text := truncatedFractionSQLText(goqu.L("?::text", sqlValue))
	return goqu.L("basyx_validated_cast_input(?, ?)::time", text, "time without time zone")
}

func truncatedFractionSQLText(text exp.Expression) exp.Expression {
	return goqu.Func("regexp_replace", text, fractionBeyondMicrosecondsPattern, `\1`)
}
