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
	"encoding/json"
	"errors"
	"fmt"
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
			if !strings.Contains(sql, "basyx_validated_cast_input") {
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
	if !strings.Contains(sql, "basyx_validated_cast_input") || !strings.Contains(sql, "EXTRACT(YEAR") {
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
	if !strings.Contains(sql, " ~ basyx_safe_regex_pattern(") {
		t.Fatalf("field-derived regular expression is not safely validated at the regex operand:\n%s", sql)
	}
}

func TestNestedSQLOperandsAlwaysUseFieldDecorator(t *testing.T) {
	t.Parallel()
	wrappers := []struct {
		name string
		wrap func(*Value) Value
	}{
		{"string", func(v *Value) Value { return Value{StrCast: v} }},
		{"number", func(v *Value) Value { return Value{NumCast: v} }},
		{"boolean", func(v *Value) Value { return Value{BoolCast: v} }},
		{"datetime", func(v *Value) Value { return Value{DateTimeCast: v} }},
		{"time", func(v *Value) Value { return Value{TimeCast: v} }},
		{"hex", func(v *Value) Value { return Value{HexCast: v} }},
		{"year", func(v *Value) Value { return Value{Year: v} }},
		{"month", func(v *Value) Value { return Value{Month: v} }},
		{"day", func(v *Value) Value { return Value{DayOfMonth: v} }},
		{"weekday", func(v *Value) Value { return Value{DayOfWeek: v} }},
	}
	for _, outer := range wrappers {
		for _, inner := range wrappers {
			t.Run(outer.name+"/"+inner.name, func(t *testing.T) {
				t.Parallel()
				child := inner.wrap(fieldValue("$aas#assetInformation.assetType"))
				operand := outer.wrap(&child)
				assertNestedSQLOperandDecoration(t, &operand)
			})
		}
	}
}

func assertNestedSQLOperandDecoration(t *testing.T, operand *Value) {
	t.Helper()
	collector, err := NewResolvedFieldPathCollectorForRoot(CollectorRootAAS)
	if err != nil {
		t.Fatal(err)
	}
	denied := collector.WithFieldValueDecorator(func(SemanticFieldAccess) (FieldValueDecoration, error) {
		return FieldValueDecoration{SQLValue: goqu.L("NULL")}, nil
	})
	value, resolved, err := toSQLResolvedFieldOrValue(operand, "test", denied)
	if err != nil || len(resolved) != 0 {
		t.Fatalf("denied field retained dependencies: %v, %v", resolved, err)
	}
	sql, _, err := goqu.Dialect("postgres").From("aas").Select(value).ToSQL()
	if err != nil || !strings.Contains(sql, "NULL") || strings.Contains(sql, "asset_type") {
		t.Fatalf("denied nested field did not become NULL: %s, %v", sql, err)
	}
	_, resolved, err = toSQLResolvedFieldOrValue(operand, "test", collector)
	if err != nil || len(resolved) != 1 {
		t.Fatalf("visible nested field lost its join dependency: %v, %v", resolved, err)
	}
}

func TestNestedSQLOperandPropagatesAuthorizationError(t *testing.T) {
	t.Parallel()
	collector, err := NewResolvedFieldPathCollectorForRoot(CollectorRootAAS)
	if err != nil {
		t.Fatal(err)
	}
	authorizationError := errors.New("GRAMMAR-TEST-AUTHORIZATION")
	collector = collector.WithFieldValueDecorator(func(SemanticFieldAccess) (FieldValueDecoration, error) {
		return FieldValueDecoration{}, authorizationError
	})
	operand := Value{NumCast: &Value{Year: &Value{DateTimeCast: fieldValue("$aas#assetInformation.assetType")}}}
	if _, _, err := toSQLResolvedFieldOrValue(&operand, "test", collector); !errors.Is(err, authorizationError) {
		t.Fatalf("nested operand suppressed authorization error: %v", err)
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

func TestNestedCastSQLSizeIsLinear(t *testing.T) {
	for _, depth := range []int{2, 4, 8, 12} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			raw := `{"$eq":[` + strings.Repeat(`{"$numCast":`, depth) + `{"$field":"$aas#idShort"}` + strings.Repeat(`}`, depth) + `,{"$numVal":1}]}`
			var expression LogicalExpression
			if err := json.Unmarshal([]byte(raw), &expression); err != nil {
				t.Fatal(err)
			}
			expression, _ = expression.SimplifyForBackendFilter(nil)
			collector, err := NewResolvedFieldPathCollectorForRoot(CollectorRootAAS)
			if err != nil {
				t.Fatal(err)
			}
			predicate, _, err := expression.EvaluateToExpression(collector)
			if err != nil {
				t.Fatal(err)
			}
			sql, _, err := goqu.Dialect("postgres").From("aas").Select("aas_id").Where(predicate).ToSQL()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("depth=%d input=%d SQL=%d", depth, len(raw), len(sql))
			if len(sql) > 256+depth*128 {
				t.Errorf("nested cast SQL exceeds linear size budget: %d bytes", len(sql))
			}
		})
	}
}
