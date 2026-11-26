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
	"strings"
	"testing"
	"time"
)

func TestSimplifyDateTimeCastOnLiteralsAndClaims(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"2024-01-01T08:00:00", "2024-01-01T08:00:00Z"},
		{" 2024-01-01T08:00:00+02:00 ", "2024-01-01T06:00:00Z"},
		{"2024-12-31T23:59:59.999999999Z", "2024-12-31T23:59:59.999999999Z"},
		{"2024-02-29T24:00:00.000Z", "2024-03-01T00:00:00Z"},
		{"12024-01-01T00:00:00Z", "12024-01-01T00:00:00Z"},
	} {
		t.Run(test.input, func(t *testing.T) {
			want, ok := parseXSDDateTime(test.want)
			if !ok {
				t.Fatalf("invalid expectation %s", test.want)
			}
			for _, input := range dateTimeCastInputs(test.input) {
				cast := Value{DateTimeCast: &input.value}
				expression := LogicalExpression{Eq: ComparisonItems{cast, {DateTimeVal: new(DateTimeLiteralPattern(want))}}}
				if _, decision := expression.SimplifyForBackendFilter(input.resolver); decision != SimplifyTrue {
					t.Fatalf("cast of %#v gave %v, want instant %s", input.value, decision, test.want)
				}
			}
		})
	}
}

func TestDateTimeCastRejectsNonXSDDateTimes(t *testing.T) {
	midnight := DateTimeLiteralPattern(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	for _, input := range []string{"2024-01-01", "2024-01-01Z", "2024-01-01 00:00:00", "2024-01-01T00:00", "2024-01-01t00:00:00z", "today"} {
		t.Run(input, func(t *testing.T) {
			for _, operand := range dateTimeCastInputs(input) {
				expression := LogicalExpression{Eq: ComparisonItems{{DateTimeCast: &operand.value}, {DateTimeVal: &midnight}}}
				if _, decision := expression.SimplifyForBackendFilter(operand.resolver); decision == SimplifyTrue {
					t.Fatalf("%q is not an xs:dateTime, so the cast must not match", input)
				}
			}
		})
	}
}

type dateTimeCastInput struct {
	value    Value
	resolver AttributeResolver
}

func dateTimeCastInputs(text string) []dateTimeCastInput {
	return []dateTimeCastInput{
		{strVal(text), func(AttributeValue) any { return nil }},
		{Value{Attribute: map[string]string{"CLAIM": "timestamp"}}, func(AttributeValue) any { return ClaimValue{Value: text} }},
	}
}

func TestDateTimeCastExtendedYearAndInvalidDates(t *testing.T) {
	if got, ok := toDateTime("10000-02-29T00:00:00+00:00"); !ok || got.Year() != 10000 || got.Month() != time.February || got.Day() != 29 {
		t.Fatalf("extended year parsed as %v, valid %v", got, ok)
	}
	for _, invalid := range []string{
		"", "2023-02-29T00:00:00", "10100-02-29T00:00:00", "-0001-01-01T00:00:00", "0000-06-01T00:00:00Z", "02024-01-01T00:00:00Z", "2024-01-01T23:59:60Z", "2024-01-01T24:01", "2024-01-01T24:00:00.001Z",
		"2024-01-01T08:00:00,5Z", "2024-01-01T08:00:00+24:00", "2024-01-01T08:00:00+14:30", "294276-12-31T23:59:59-05:00",
	} {
		if value, ok := toDateTime(invalid); ok {
			t.Errorf("invalid date %q parsed as %s", invalid, value)
		}
	}
}

func TestTimeCastOfDateTimeKeepsItsTimezone(t *testing.T) {
	lexical := strVal("2024-01-01T00:30:00+02:00")
	instant := Value{DateTimeCast: &lexical}
	for _, test := range []struct {
		clock TimeLiteralPattern
		want  SimplifyDecision
	}{
		{"00:30:00+02:00", SimplifyTrue},
		{"01:30:00+03:00", SimplifyTrue},
		{"23:30:00+01:00", SimplifyFalse},
		{"22:30:00Z", SimplifyFalse},
	} {
		expression := LogicalExpression{Eq: ComparisonItems{{TimeCast: &instant}, {TimeVal: &test.clock}}}
		if _, decision := expression.SimplifyForBackendFilterNoResolver(); decision != test.want {
			t.Errorf("time of %s compared with %s gave %v, want %v", *lexical.StrVal, test.clock, decision, test.want)
		}
	}
}

func TestTimeCastOfDateTimeStringIsNotFolded(t *testing.T) {
	lexical := strVal("2024-01-01T00:30:00+02:00")
	clock := TimeLiteralPattern("00:30:00Z")
	expression := LogicalExpression{Eq: ComparisonItems{{TimeCast: &lexical}, {TimeVal: &clock}}}
	if _, decision := expression.SimplifyForBackendFilterNoResolver(); decision == SimplifyTrue {
		t.Fatal("PostgreSQL rejects a date-time string as time input, so constant folding must not accept it")
	}
}

func TestTimeCastOfDateOnlyStringRequiresAnExplicitDateTimeCast(t *testing.T) {
	date := strVal("2024-01-01")
	midnight := TimeLiteralPattern("00:00:00Z")
	expression := LogicalExpression{Eq: ComparisonItems{{TimeCast: &date}, {TimeVal: &midnight}}}
	if _, decision := expression.SimplifyForBackendFilterNoResolver(); decision == SimplifyTrue {
		t.Fatal("a date-only string supplied no time of day")
	}
}

func TestTemporalFoldingMatchesPostgreSQLSemantics(t *testing.T) {
	dateTimeCast := func(text string) Value {
		lexical := strVal(text)
		return Value{DateTimeCast: &lexical}
	}
	timeCast := func(operand Value) Value { return Value{TimeCast: &operand} }
	timeVal := func(text string) Value {
		pattern := TimeLiteralPattern(text)
		return Value{TimeVal: &pattern}
	}
	halfSecond := DateTimeLiteralPattern(time.Date(2024, 1, 1, 8, 0, 0, 500_000_000, time.UTC))
	one := float64(1)
	tests := []struct {
		name       string
		expression LogicalExpression
		want       SimplifyDecision
	}{
		{"digits beyond microseconds are truncated, not rounded", LogicalExpression{Eq: ComparisonItems{dateTimeCast("2024-12-31T23:59:59.999999999"), dateTimeCast("2025-01-01T00:00:00Z")}}, SimplifyFalse},
		{"instants within one microsecond are equal", LogicalExpression{Gt: ComparisonItems{dateTimeCast("2024-01-01T00:00:00.0000001Z"), dateTimeCast("2024-01-01T00:00:00Z")}}, SimplifyFalse},
		{"times keep fractional seconds", LogicalExpression{Gt: ComparisonItems{timeCast(Value{DateTimeVal: &halfSecond}), timeCast(strVal("08:00:00Z"))}}, SimplifyTrue},
		{"time literals are truncated to microseconds", LogicalExpression{Eq: ComparisonItems{timeVal("10:00:00.1234567Z"), timeCast(strVal("10:00:00.123456Z"))}}, SimplifyTrue},
		{"zone-less times are accepted", LogicalExpression{Eq: ComparisonItems{timeCast(strVal("10:00:00")), timeVal("10:00:00Z")}}, SimplifyTrue},
		{"24:00:00 is the start of the day", LogicalExpression{Eq: ComparisonItems{timeCast(strVal("24:00:00")), timeVal("00:00Z")}}, SimplifyTrue},
		{"times with an offset compare as instants", LogicalExpression{Eq: ComparisonItems{timeCast(strVal("10:00:00+02:00")), timeVal("08:00:00Z")}}, SimplifyTrue},
		{"time literals may omit seconds", LogicalExpression{Lt: ComparisonItems{timeVal("23:00-02:00"), timeVal("23:30")}}, SimplifyFalse},
		{"years are taken in the value's own timezone", LogicalExpression{Eq: ComparisonItems{{Year: new(dateTimeCast("0001-01-01T00:00:00+01:00"))}, {NumVal: &one}}}, SimplifyTrue},
		{"comma fractions are left to PostgreSQL", LogicalExpression{Eq: ComparisonItems{dateTimeCast("2024-01-01T08:00:00,5Z"), {DateTimeVal: &halfSecond}}}, SimplifyUndecided},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, decision := test.expression.SimplifyForBackendFilterNoResolver(); decision != test.want {
				t.Fatalf("got %v, want %v", decision, test.want)
			}
		})
	}
}

func TestDatePartsOfLocalNowUseItsOwnTimezone(t *testing.T) {
	localNow := Value{Attribute: map[string]string{"GLOBAL": "LOCALNOW"}}
	resolver := func(AttributeValue) any { return "2024-01-01T00:30:00+02:00" }
	monday := float64(time.Monday)
	expression := LogicalExpression{Eq: ComparisonItems{{DayOfWeek: &localNow}, {NumVal: &monday}}}
	if _, decision := expression.SimplifyForBackendFilter(resolver); decision != SimplifyTrue {
		t.Fatalf("Monday 00:30 local time gave %v, want Monday", decision)
	}
	clock := TimeLiteralPattern("00:30+02:00")
	timeOfDay := LogicalExpression{Eq: ComparisonItems{{TimeCast: &localNow}, {TimeVal: &clock}}}
	if _, decision := timeOfDay.SimplifyForBackendFilter(resolver); decision != SimplifyTrue {
		t.Fatalf("time of LOCALNOW gave %v, want its local clock time", decision)
	}
}

func TestStringCastOfTemporalValuesFollowsXPath(t *testing.T) {
	localNow := Value{Attribute: map[string]string{"GLOBAL": "LOCALNOW"}}
	resolver := func(AttributeValue) any { return "2024-01-01T00:30:00.500+02:00" }
	instant := DateTimeLiteralPattern(time.Date(2024, 1, 1, 0, 30, 0, 0, time.FixedZone("+02:00", 2*60*60)))
	zoneless, ok := parseDateTimeLiteral("2024-01-01T24:00")
	if !ok {
		t.Fatal("invalid fixture")
	}
	zonelessInstant := DateTimeLiteralPattern(zoneless)
	clock := TimeLiteralPattern("10:00+00:00")
	for _, test := range []struct {
		operand Value
		want    string
	}{
		{localNow, "2024-01-01T00:30:00.5+02:00"},
		{Value{DateTimeVal: &instant}, "2024-01-01T00:30:00+02:00"},
		{Value{DateTimeVal: &zonelessInstant}, "2024-01-02T00:00:00"},
		{Value{TimeVal: &clock}, "10:00:00Z"},
	} {
		want := StandardString(test.want)
		expression := LogicalExpression{Eq: ComparisonItems{{StrCast: &test.operand}, {StrVal: &want}}}
		if _, decision := expression.SimplifyForBackendFilter(resolver); decision != SimplifyTrue {
			t.Errorf("string cast of %#v gave %v, want %q", test.operand, decision, test.want)
		}
	}
}

func TestTemporalCastsTrimOnlyXSDWhitespace(t *testing.T) {
	instant := DateTimeLiteralPattern(time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC))
	for claim, match := range map[string]bool{"\t2024-01-01T08:00:00Z\r\n": true, "\v2024-01-01T08:00:00Z": false} {
		input := dateTimeCastInputs(claim)[1]
		expression := LogicalExpression{Eq: ComparisonItems{{DateTimeCast: &input.value}, {DateTimeVal: &instant}}}
		if _, decision := expression.SimplifyForBackendFilter(input.resolver); (decision == SimplifyTrue) != match {
			t.Errorf("claim %q gave %v, want match %t", claim, decision, match)
		}
	}
	if sql := renderLogicalExpressionSQL(t, LogicalExpression{Eq: ComparisonItems{{DateTimeCast: fieldValue("$aas#idShort")}, {DateTimeVal: &instant}}}); !strings.Contains(sql, "btrim(") {
		t.Fatalf("expected stored values to be trimmed:\n%s", sql)
	}
	if xsdWhitespace != " \t\n\r" {
		t.Fatalf("SQL and Go must trim the XSD whitespace characters, got %q", xsdWhitespace)
	}
}

func TestDateTimeRangeMatchesPostgreSQL(t *testing.T) {
	if _, ok := parseXSDDateTime("294276-12-31T23:59:59.9999999Z"); !ok {
		t.Error("a value that truncates to the PostgreSQL maximum must be accepted")
	}
	beyondLocalRange, ok := parseXSDDateTime("294277-01-01T00:00:00+14:00")
	if !ok {
		t.Fatal("an instant within the PostgreSQL range must be accepted")
	}
	if _, ok := extractDatePart(beyondLocalRange, "YEAR"); ok {
		t.Error("a local date beyond the PostgreSQL timestamp range must not yield a date part")
	}
	if _, ok := canonicalDateTimeString(beyondLocalRange); ok {
		t.Error("a local date beyond the PostgreSQL timestamp range must not yield a string")
	}
}
