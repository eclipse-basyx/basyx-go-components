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

// SQL patterns that validate and normalize xs:dateTime and xs:time text. They stay free of
// nested optional groups because PostgreSQL evaluates those an order of magnitude slower.
const (
	zonelessDateTimeSQLPattern   = `^([0-9]{4,}-[0-9]{2}-[0-9]{2}T[0-9:.]+)$`
	xsdDateTimeSQLPattern        = `^(?:[1-9][0-9]{3,}|0[0-9]{3})-[0-9]{2}-[0-9]{2}T(?:(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]+)?|24:00:00(?:\.0+)?)(?:Z|[+-](?:0[0-9]|1[0-3]):[0-5][0-9]|[+-]14:00)$`
	zonelessTimeSQLPattern       = `^([0-9:.]+)$`
	xsdTimeSQLPattern            = `^(?:(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]+)?|24:00:00(?:\.0+)?)(?:Z|[+-](?:0[0-9]|1[0-3]):[0-5][0-9]|[+-]14:00)$`
	timezoneDesignatorSQLPattern = `(?:Z|[+-][0-9]{2}:[0-9]{2})$`
	datePrefixSQLPattern         = `^[0-9]+-[0-9]{2}-[0-9]{2}T`
	endOfDayTimeSQLPattern       = `^24:00:00(?:\.0+)?(Z|[+-])`
	timeReferenceDateSQLPrefix   = "1972-12-31T"
)

// xsdDateTimeSQLText casts an SQL value to xs:dateTime and returns it as validated text that
// always carries a timezone, or NULL when the value is not an xs:dateTime PostgreSQL can parse.
//
// Text without a timezone gets "Z", the implicit timezone, and digits beyond microseconds are
// truncated. The operand is referenced once, so nested casts keep the generated SQL linear in size.
func xsdDateTimeSQLText(sqlValue interface{}) exp.Expression {
	zoned := goqu.Func("regexp_replace", goqu.Func("btrim", goqu.L("?::text", sqlValue)), zonelessDateTimeSQLPattern, `\1Z`)
	lexical := goqu.L("substring(? from ?)", zoned, xsdDateTimeSQLPattern)
	return goqu.Func("basyx_validated_cast_input", truncatedFractionSQLText(lexical), "timestamp with time zone")
}

// instantSQLValue returns the instant of validated xs:dateTime text.
func instantSQLValue(dateTimeText exp.Expression) exp.Expression {
	return goqu.L("?::timestamptz", dateTimeText)
}

// localDateTimeSQLValue returns the local date and time of validated xs:dateTime text,
// whose calendar parts XSD defines in the value's own timezone.
func localDateTimeSQLValue(dateTimeText exp.Expression) exp.Expression {
	return goqu.L("?::timestamp", goqu.Func("regexp_replace", dateTimeText, timezoneDesignatorSQLPattern, ""))
}

// timeOfDateTimeSQLValue returns the xs:time of validated xs:dateTime text: its local clock time
// and timezone as an instant on the XPath reference date.
func timeOfDateTimeSQLValue(dateTimeText exp.Expression) exp.Expression {
	timeText := goqu.Func("regexp_replace", dateTimeText, datePrefixSQLPattern, "")
	return timeOnReferenceDateSQLValue(timeText)
}

// xsdTimeSQLValue casts an SQL value to xs:time and returns it as an instant on the XPath
// reference date, or NULL when the value is not an xs:time.
func xsdTimeSQLValue(sqlValue interface{}) exp.Expression {
	zoned := goqu.Func("regexp_replace", goqu.Func("btrim", goqu.L("?::text", sqlValue)), zonelessTimeSQLPattern, `\1Z`)
	lexical := goqu.L("substring(? from ?)", zoned, xsdTimeSQLPattern)
	return timeOnReferenceDateSQLValue(truncatedFractionSQLText(lexical))
}

// timeOnReferenceDateSQLValue maps 24:00:00 to 00:00:00, as xs:time does.
func timeOnReferenceDateSQLValue(timeText exp.Expression) exp.Expression {
	startOfDay := goqu.Func("regexp_replace", timeText, endOfDayTimeSQLPattern, `00:00:00\1`)
	referenced := goqu.L("? || ?", timeReferenceDateSQLPrefix, startOfDay)
	return goqu.L("basyx_validated_cast_input(?, ?)::timestamptz", referenced, "timestamp with time zone")
}

// canonicalDateTimeSQLText renders an instant as canonical xs:dateTime text in UTC, or NULL
// before year 1, matching canonicalDateTimeString.
func canonicalDateTimeSQLText(instant exp.Expression) exp.Expression {
	utcText := goqu.L("to_json(timezone('UTC', ?)) #>> '{}'", instant)
	return goqu.L("substring(? from ?) || 'Z'", utcText, `^[0-9]{4,}-[0-9]{2}-[0-9]{2}T[0-9:.]+$`)
}

// canonicalTimeSQLText renders a time instant as canonical xs:time text in UTC.
func canonicalTimeSQLText(instant exp.Expression) exp.Expression {
	return goqu.L("(to_json(timezone('UTC', ?)::time) #>> '{}') || 'Z'", instant)
}

func truncatedFractionSQLText(text exp.Expression) exp.Expression {
	return goqu.Func("regexp_replace", text, fractionBeyondMicrosecondsPattern, `\1`)
}
