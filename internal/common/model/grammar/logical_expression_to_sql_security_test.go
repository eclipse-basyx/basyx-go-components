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

	"github.com/doug-martin/goqu/v9"
)

func TestFallibleCastsUsePostgreSQLTotalInputValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value Value
	}{
		{name: "date time", value: Value{DateTimeCast: fieldValue("$aas#idShort")}},
		{name: "time", value: Value{TimeCast: fieldValue("$aas#idShort")}},
		{name: "number", value: Value{NumCast: fieldValue("$aas#idShort")}},
		{name: "boolean", value: Value{BoolCast: fieldValue("$aas#idShort")}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			one := float64(1)
			expression := LogicalExpression{Eq: ComparisonItems{
				test.value,
				{NumVal: &one},
			}}
			if test.name == "date time" {
				now := DateTimeLiteralPattern{}
				expression.Eq[1] = Value{DateTimeVal: &now}
			}
			if test.name == "time" {
				value := TimeLiteralPattern("01:02:03Z")
				expression.Eq[1] = Value{TimeVal: &value}
			}
			if test.name == "boolean" {
				value := true
				expression.Eq[1] = Value{Boolean: &value}
			}

			sql := renderLogicalExpressionSQL(t, expression)
			if !strings.Contains(sql, "pg_input_is_valid") {
				t.Fatalf("fallible %s cast is not total:\n%s", test.name, sql)
			}
			if strings.Contains(sql, " ~ ") {
				t.Fatalf("fallible %s cast rejects PostgreSQL-valid lexical forms:\n%s", test.name, sql)
			}
		})
	}
}

func TestDatePartUsesTotalDateTimeCast(t *testing.T) {
	t.Parallel()

	one := float64(1)
	expression := LogicalExpression{Eq: ComparisonItems{
		{Year: fieldValue("$aas#idShort")},
		{NumVal: &one},
	}}
	sql := renderLogicalExpressionSQL(t, expression)
	if !strings.Contains(sql, "pg_input_is_valid") || !strings.Contains(sql, "EXTRACT(YEAR") {
		t.Fatalf("date-part extraction can still throw on stored input:\n%s", sql)
	}
}

func TestRegexUsesSafeDatabaseFunctionForFieldDerivedPattern(t *testing.T) {
	t.Parallel()

	field := ModelStringPattern("$aas#idShort")
	pattern := StandardString("value")
	expression := LogicalExpression{Regex: StringItems{
		{StrVal: &pattern},
		{Field: &field},
	}}
	sql := renderLogicalExpressionSQL(t, expression)
	if !strings.Contains(sql, "basyx_safe_regex_match") || strings.Contains(sql, " ~ ") {
		t.Fatalf("field-derived regular expression can still expose invalid patterns:\n%s", sql)
	}
}

func fieldValue(field string) *Value {
	value := ModelStringPattern(field)
	return &Value{Field: &value}
}

func renderLogicalExpressionSQL(t *testing.T, expression LogicalExpression) string {
	t.Helper()
	collector, err := NewResolvedFieldPathCollectorForRoot(CollectorRootAAS)
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	where, _, err := expression.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("evaluate expression: %v", err)
	}
	sql, _, err := goqu.Dialect("postgres").From(goqu.T("aas").As("aas")).Where(where).Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("render expression: %v", err)
	}
	return sql
}
