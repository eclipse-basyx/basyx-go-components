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
	"testing"
	"time"
)

func TestSimplifyDateTimeCastOnLiteralsAndClaims(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"2024-01-01T08:00:00", "2024-01-01T08:00:00Z"},
		{"2024-01-01", "2024-01-01T00:00:00Z"},
		{"2024-01-01Z", "2024-01-01T00:00:00Z"},
		{"2024-01-01-05:00", "2024-01-01T05:00:00Z"},
		{"2024-01-01+02:00", "2023-12-31T22:00:00Z"},
		{"2024-01-01 08:00", "2024-01-01T08:00:00Z"},
		{"2024-01-01T08:00+02:00", "2024-01-01T06:00:00Z"},
		{"2024-12-31 23:59:59.999999999", "2024-12-31T23:59:59.999999999Z"},
		{"2024-02-29T24:00:00.000Z", "2024-03-01T00:00:00Z"},
	} {
		t.Run(test.input, func(t *testing.T) {
			want, err := time.Parse(time.RFC3339Nano, test.want)
			if err != nil {
				t.Fatal(err)
			}
			literal := strVal(test.input)
			claim := Value{Attribute: map[string]string{"CLAIM": "timestamp"}}
			resolver := func(AttributeValue) any { return ClaimValue{Value: test.input} }
			for _, input := range []Value{literal, claim} {
				cast := Value{DateTimeCast: &input}
				expression := LogicalExpression{Eq: ComparisonItems{cast, {DateTimeVal: new(DateTimeLiteralPattern(want))}}}
				_, decision := expression.SimplifyForBackendFilter(resolver)
				if decision != SimplifyTrue {
					t.Fatalf("cast of %#v gave %v, want instant %s", input, decision, test.want)
				}
				datePart := LogicalExpression{Eq: ComparisonItems{{Year: &cast}, {NumVal: new(float64(want.Year()))}}}
				if _, decision := datePart.SimplifyForBackendFilter(resolver); decision != SimplifyTrue {
					t.Fatalf("year of %#v gave %v, want %d", input, decision, want.Year())
				}
			}
		})
	}
}

func TestDateTimeCastExtendedYearAndInvalidDates(t *testing.T) {
	if got, ok := toDateTime("10000-02-29+00:00"); !ok || got.Year() != 10000 || got.Month() != time.February || got.Day() != 29 {
		t.Fatalf("extended year parsed as %v, valid %v", got, ok)
	}
	for _, invalid := range []string{"", "2023-02-29", "10100-02-29", "-0001-01-01", "2024-01-01T24:01", "2024-01-01T24:00:00.001Z"} {
		if value, ok := toDateTime(invalid); ok {
			t.Errorf("invalid date %q parsed as %s", invalid, value)
		}
	}
}

func TestSimplifyTimeCastOfDateTimeUsesUTC(t *testing.T) {
	lexical := strVal("2024-01-01T00:30:00+02:00")
	instant := Value{DateTimeCast: &lexical}
	tests := []struct {
		operand Value
		want    TimeLiteralPattern
	}{
		{instant, "22:30:00Z"},
		{lexical, "00:30:00Z"},
	}
	for _, test := range tests {
		expression := LogicalExpression{Eq: ComparisonItems{{TimeCast: &test.operand}, {TimeVal: &test.want}}}
		if _, decision := expression.SimplifyForBackendFilterNoResolver(); decision != SimplifyTrue {
			t.Errorf("time cast of %#v gave %v, want clock time %s", test.operand, decision, test.want)
		}
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
