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
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// SQL patterns that validate and normalize xs:dateTime and xs:time text. They stay free of
// nested optional groups because PostgreSQL evaluates those an order of magnitude slower.
//
// Validated text marks a value without a timezone with a lowercase "z". PostgreSQL reads it as
// UTC, the implicit timezone, and the lexical patterns never accept it, so the marker cannot be
// confused with a submitted "Z" when the value is cast back to a string.
const (
	xsdDateTimeSQLPattern        = `^(?:[1-9][0-9]{3,}|0[0-9]{3})-[0-9]{2}-[0-9]{2}T(?:(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]+)?|24:00:00(?:\.0+)?)(?:Z|[+-](?:0[0-9]|1[0-3]):[0-5][0-9]|[+-]14:00)?$`
	xsdTimeSQLPattern            = `^(?:(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]+)?|24:00:00(?:\.0+)?)(?:Z|[+-](?:0[0-9]|1[0-3]):[0-5][0-9]|[+-]14:00)?$`
	zonelessDateTimeSQLPattern   = `^([0-9]+-[0-9]{2}-[0-9]{2}T[0-9:.]+)$`
	zonelessTimeSQLPattern       = `^([0-9:.]+)$`
	timezoneDesignatorSQLPattern = `z|Z|[+-][0-9]{2}:[0-9]{2}`
	timezoneSQLPattern           = `(?:` + timezoneDesignatorSQLPattern + `)$`
	datePrefixSQLPattern         = `^[0-9]+-[0-9]{2}-[0-9]{2}T`
	endOfDaySQLPattern           = `^24:00:00(?:\.0+)?`
	timeReferenceDateSQLText     = "1972-12-31T"
)

func containsTemporalOperand(value *Value) bool {
	if value == nil {
		return false
	}
	if isTemporalOperand(*value) || isDatePart(*value) {
		return true
	}
	for _, child := range valueChildren(*value) {
		if containsTemporalOperand(child) {
			return true
		}
	}
	return false
}

// xsdDateTimeSQLText casts an SQL value to xs:dateTime and returns it as validated text, or NULL
// when the value is not an xs:dateTime PostgreSQL can represent. Digits beyond microseconds are
// truncated. The operand is referenced once, so nested casts keep the generated SQL linear in size.
func xsdDateTimeSQLText(sqlValue interface{}) exp.Expression {
	lexical := goqu.L("substring(? from ?)", xsdTrimmedSQLText(sqlValue), xsdDateTimeSQLPattern)
	marked := goqu.Func("regexp_replace", lexical, zonelessDateTimeSQLPattern, `\1z`)
	return goqu.Func("basyx_validated_cast_input", truncatedFractionSQLText(marked), "timestamp with time zone")
}

// xsdTimeSQLText casts an SQL value to xs:time and returns it as validated text, or NULL.
func xsdTimeSQLText(sqlValue interface{}) exp.Expression {
	lexical := goqu.L("substring(? from ?)", xsdTrimmedSQLText(sqlValue), xsdTimeSQLPattern)
	return truncatedFractionSQLText(goqu.Func("regexp_replace", lexical, zonelessTimeSQLPattern, `\1z`))
}

// instantSQLValue returns the instant of validated xs:dateTime text.
func instantSQLValue(dateTimeText exp.Expression) exp.Expression {
	return goqu.L("?::timestamptz", dateTimeText)
}

// localDateTimeSQLValue returns the local date and time of validated xs:dateTime text, whose
// calendar parts XSD defines in the value's own timezone, or NULL when they exceed the
// PostgreSQL timestamp range.
func localDateTimeSQLValue(dateTimeText exp.Expression) exp.Expression {
	asUTC := goqu.Func("regexp_replace", dateTimeText, timezoneSQLPattern, "Z")
	return goqu.L("timezone('UTC', ?::timestamptz)", goqu.Func("basyx_validated_cast_input", asUTC, "timestamp with time zone"))
}

// timeTextOfDateTimeSQLValue returns the xs:time of validated xs:dateTime text: its local clock
// time and timezone.
func timeTextOfDateTimeSQLValue(dateTimeText exp.Expression) exp.Expression {
	return goqu.Func("regexp_replace", dateTimeText, datePrefixSQLPattern, "")
}

// timeInstantSQLValue returns validated xs:time text as an instant on the XPath reference date.
// 24:00:00 maps to 00:00:00, as xs:time does.
func timeInstantSQLValue(timeText exp.Expression) exp.Expression {
	startOfDay := goqu.Func("regexp_replace", timeText, endOfDaySQLPattern, "00:00:00")
	referenced := goqu.L("? || ?", timeReferenceDateSQLText, startOfDay)
	return goqu.L("basyx_validated_cast_input(?, ?)::timestamptz", referenced, "timestamp with time zone")
}

// canonicalDateTimeSQLText casts validated xs:dateTime text to xs:string the way XPath does,
// matching canonicalDateTimeString.
func canonicalDateTimeSQLText(dateTimeText exp.Expression) exp.Expression {
	value := goqu.L("value")
	localText := goqu.L("to_json(?) #>> '{}'", localDateTimeSQLValue(value))
	return goqu.L("(SELECT ? || ? FROM (SELECT ? AS value) AS xsd_date_time)", localText, canonicalTimezoneSQLText(value), dateTimeText)
}

// canonicalTimeSQLText casts validated xs:time text to xs:string the way XPath does,
// matching canonicalTimeString.
func canonicalTimeSQLText(timeText exp.Expression) exp.Expression {
	startOfDay := goqu.Func("regexp_replace", timeText, endOfDaySQLPattern, "00:00:00")
	suffix := `(` + timezoneDesignatorSQLPattern + `)?$`
	trailingZeros := goqu.Func("regexp_replace", startOfDay, `(\.[0-9]*[1-9])0+`+suffix, `\1\2`)
	zeroFraction := goqu.Func("regexp_replace", trailingZeros, `\.0+`+suffix, `\1`)
	return canonicalTimezoneSQLSuffix(zeroFraction)
}

func canonicalTimezoneSQLText(temporalText exp.Expression) exp.Expression {
	return canonicalTimezoneSQLSuffix(goqu.L("substring(? from ?)", temporalText, timezoneSQLPattern))
}

// canonicalTimezoneSQLSuffix writes a zero offset as "Z" and drops the zone-less marker.
func canonicalTimezoneSQLSuffix(text exp.Expression) exp.Expression {
	utc := goqu.Func("regexp_replace", text, `(?:[+-]00:00|Z)$`, "Z")
	return goqu.Func("regexp_replace", utc, `z$`, "")
}

func xsdTrimmedSQLText(sqlValue interface{}) exp.Expression {
	return goqu.Func("btrim", goqu.L("?::text", sqlValue), xsdWhitespace)
}

func truncatedFractionSQLText(text exp.Expression) exp.Expression {
	return goqu.Func("regexp_replace", text, fractionBeyondMicrosecondsPattern, `\1`)
}

// dateTimeLiteralSQLValue binds a $dateTimeVal instant, truncated to microseconds. PostgreSQL
// reads instants before year 1, such as "0000-01-01T00:00:00Z", only with its BC notation,
// which the default time rendering of interpolated SQL lacks.
func dateTimeLiteralSQLValue(t time.Time) exp.Expression {
	instant := normalizeTime(t)
	if instant.Year() >= 1 {
		return goqu.V(instant)
	}
	bcText := fmt.Sprintf("%04d-%s+00 BC", 1-instant.Year(), instant.Format("01-02 15:04:05.999999"))
	return goqu.L("?::timestamptz", bcText)
}
