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
	"time"

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
		})
	}
}

func TestNestedDateTimeCastsRenderEachOperandOnce(t *testing.T) {
	t.Parallel()

	inner := Value{DateTimeCast: fieldValue("$aas#idShort")}
	outer := Value{DateTimeCast: &Value{StrCast: &inner}}
	instant := DateTimeLiteralPattern{}
	sql := renderLogicalExpressionSQL(t, LogicalExpression{Eq: ComparisonItems{outer, {DateTimeVal: &instant}}})
	if strings.Count(sql, "basyx_validated_cast_input(") != 3 || strings.Count(sql, `"aas"."id_short"`) != 1 {
		t.Fatalf("expected one validation per cast, one for the string cast, and one field reference:\n%s", sql)
	}
}

func TestDatePartExtractsTheLocalCalendarPart(t *testing.T) {
	t.Parallel()

	one := float64(1)
	sql := renderLogicalExpressionSQL(t, LogicalExpression{Eq: ComparisonItems{
		{DayOfMonth: fieldValue("$aas#idShort")},
		{NumVal: &one},
	}})
	if !strings.Contains(sql, "EXTRACT(DAY FROM timezone('UTC', basyx_validated_cast_input(regexp_replace(basyx_validated_cast_input(") {
		t.Fatalf("date parts must come from the local date and time, read as UTC:\n%s", sql)
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
	if !strings.Contains(sql, "basyx_validated_cast_input(") || !strings.Contains(sql, "EXTRACT(YEAR") {
		t.Fatalf("date-part extraction can still throw on stored input:\n%s", sql)
	}
}

func TestDatePartOfDateTimeLiteralIsComputedInGo(t *testing.T) {
	t.Parallel()

	instant := DateTimeLiteralPattern(time.Date(2024, 1, 1, 0, 30, 0, 0, time.FixedZone("+02:00", 2*60*60)))
	year := float64(2024)
	collector, err := NewResolvedFieldPathCollectorForRoot(CollectorRootAAS)
	if err != nil {
		t.Fatal(err)
	}
	expression := LogicalExpression{Eq: ComparisonItems{{Year: &Value{DateTimeVal: &instant}}, {NumVal: &year}}}
	where, _, err := expression.EvaluateToExpression(collector)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := goqu.Dialect("postgres").From("aas").Where(where).Prepared(true).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "EXTRACT") || !argListContains(args, float64(2024)) {
		t.Fatalf("the local year of a literal must be bound as a number:\n%s %v", sql, args)
	}
}

func TestTemporalLiteralsAreTruncatedToMicrosecondsForSQL(t *testing.T) {
	t.Parallel()

	clock := TimeLiteralPattern("10:00:00.1234567Z")
	instant := DateTimeLiteralPattern(time.Date(2024, 12, 31, 23, 59, 59, 999_999_900, time.UTC))
	tests := []struct {
		literal Value
		want    time.Time
	}{
		{Value{TimeVal: &clock}, time.Date(1972, time.December, 31, 10, 0, 0, 123_456_000, time.UTC)},
		{Value{DateTimeVal: &instant}, time.Date(2024, time.December, 31, 23, 59, 59, 999_999_000, time.UTC)},
	}
	for _, test := range tests {
		sqlValue, _, err := toSQLResolvedFieldOrValue(&test.literal, "left", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, args, err := goqu.Dialect("postgres").Select(sqlValue).Prepared(true).ToSQL()
		if err != nil {
			t.Fatal(err)
		}
		if bound, ok := args[0].(time.Time); !ok || !bound.Equal(test.want) {
			t.Errorf("literal bound as %#v, want %s", args[0], test.want)
		}
	}
}

func TestNegatedMatchKeepsIndeterminateTermsUnknownInPolicies(t *testing.T) {
	t.Parallel()

	var expression LogicalExpression
	err := json.Unmarshal([]byte(`{"$not":{"$match":[
		{"$eq":[{"$field":"$sme.secret#value"},{"$strVal":"foo"}]},
		{"$gt":[{"$dateTimeCast":{"$attribute":{"CLAIM":"since"}}},{"$dateTimeVal":"2024-01-01T00:00:00Z"}]}]}}`), &expression)
	if err != nil {
		t.Fatal(err)
	}
	simplified, decision := expression.SimplifyForBackendFilterWithOptions(func(AttributeValue) any { return ClaimValue{Value: "today"} }, DefaultSimplifyOptions())
	if decision != SimplifyUndecided {
		t.Fatalf("expected the field term to keep the rule undecided, got %v", decision)
	}
	where, _, err := simplified.EvaluateToExpression(mustCollectorForRoot(t, "$sm").ForAuthorizationPolicy())
	if err != nil {
		t.Fatal(err)
	}
	sql, _, err := goqu.Dialect("postgres").From("submodel").Where(where).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "IS NULL") {
		t.Fatalf("a negated $match with an indeterminate term must keep the unknown result:\n%s", sql)
	}
}

func TestDateTimeLiteralsBeforeYearOneUsePostgreSQLNotation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ literal, want string }{
		{"0000-01-01T00:00:00Z", "'0001-01-01 00:00:00+00 BC'::timestamptz"},
		{"0001-01-01T00:00:00.5+14:00", "'0001-12-31 10:00:00.5+00 BC'::timestamptz"},
	} {
		instant, ok := parseDateTimeLiteral(test.literal)
		if !ok {
			t.Fatalf("invalid literal %s", test.literal)
		}
		sql, _, err := goqu.Dialect("postgres").Select(dateTimeLiteralSQLValue(instant)).ToSQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sql, test.want) {
			t.Errorf("literal %s rendered as %s, want %s", test.literal, sql, test.want)
		}
	}
}

func TestTypedComparisonsCastFieldsWithoutImplicitCasts(t *testing.T) {
	t.Parallel()

	for raw, cast := range map[string]string{
		`{"$eq":[{"$field":"$sm#idShort"},{"$timeVal":"10:00"}]}`:                              "substring(btrim(",
		`{"$eq":[{"$field":"$sm#idShort"},{"$dateTimeVal":"0000-01-01T00:00:00Z"}]}`:           "substring(btrim(",
		`{"$lt":[{"$dateTimeVal":"2024-01-01T00:00:00Z"},{"$field":"$sm#idShort"}]}`:           "substring(btrim(",
		`{"$eq":[{"$field":"$sm#idShort"},{"$year":{"$dateTimeVal":"2024-01-01T00:00:00Z"}}]}`: "'double precision'",
		`{"$eq":[{"$field":"$sm#idShort"},{"$numCast":{"$strVal":"3"}}]}`:                      "'double precision'",
		`{"$eq":[{"$field":"$sm#idShort"},{"$numVal":3}]}`:                                     "'double precision'",
		`{"$eq":[{"$field":"$sm#idShort"},{"$boolean":true}]}`:                                 "'boolean'",
	} {
		var expression LogicalExpression
		if err := json.Unmarshal([]byte(raw), &expression); err != nil {
			t.Fatal(err)
		}
		simplified, _ := expression.SimplifyForBackendFilterWithOptions(func(AttributeValue) any { return nil }, SimplifyOptions{})
		where, _, err := simplified.EvaluateToExpression(mustCollectorForRoot(t, "$sm"))
		if err != nil {
			t.Fatal(err)
		}
		sql, _, err := goqu.Dialect("postgres").From("submodel").Where(where).ToSQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sql, cast) {
			t.Errorf("a field compared with a typed operand must be cast (%s), got:\n%s", cast, sql)
		}
	}
}

func TestNegatedFallibleCastsStayUnknownInPolicies(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"$not":{"$gt":[{"$numCast":{"$field":"$sme.x#value"}},{"$numVal":5}]}}`,
		`{"$not":{"$boolCast":{"$field":"$sme.x#value"}}}`,
		`{"$not":{"$eq":[{"$field":"$sme.x#value"},{"$numVal":5}]}}`,
		`{"$not":{"$eq":[{"$hexCast":{"$field":"$sme.x#value"}},{"$hexVal":"16#0A"}]}}`,
		`{"$not":{"$regex":[{"$strVal":"abc"},{"$field":"$sme.x#value"}]}}`,
		`{"$not":{"$regex":[{"$field":"$sme.x#value"},{"$attribute":{"CLAIM":"pattern"}}]}}`,
		`{"$not":{"$regex":[{"$field":"$sme.x#value"},{"$strVal":"\\z"}]}}`,
	} {
		var expression LogicalExpression
		if err := json.Unmarshal([]byte(raw), &expression); err != nil {
			t.Fatal(err)
		}
		claimPattern := func(AttributeValue) any { return ClaimValue{Value: "("} }
		simplified, _ := expression.SimplifyForBackendFilterWithOptions(claimPattern, DefaultSimplifyOptions())
		where, _, err := simplified.EvaluateToExpression(mustCollectorForRoot(t, "$sm").ForAuthorizationPolicy())
		if err != nil {
			t.Fatal(err)
		}
		sql, _, err := goqu.Dialect("postgres").From("submodel").Where(where).ToSQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sql, "IS NULL") {
			t.Errorf("a failed cast or pattern under policy negation must stay unknown, got:\n%s", sql)
		}
	}
}

func TestTemporalValuesAreNotNumbersOrHexBinary(t *testing.T) {
	t.Parallel()

	clock := TimeLiteralPattern("10:00Z")
	ten := float64(10)
	hex := HexLiteralPattern("16#0A")
	clockText := StandardString("10:00:00Z")
	for _, expression := range []LogicalExpression{
		{Eq: ComparisonItems{{NumCast: &Value{TimeVal: &clock}}, {NumVal: &ten}}},
		{Eq: ComparisonItems{{StrCast: &Value{NumCast: &Value{TimeVal: &clock}}}, {StrVal: &clockText}}},
		{Ne: ComparisonItems{{HexVal: &hex}, {HexCast: &Value{TimeVal: &clock}}}},
	} {
		if _, decision := expression.SimplifyForBackendFilterNoResolver(); decision == SimplifyTrue {
			t.Errorf("casting a time to a number or hexBinary must not match: %#v", expression)
		}
	}
	sql := renderLogicalExpressionSQL(t, LogicalExpression{Ne: ComparisonItems{{HexVal: &hex}, {HexCast: &Value{TimeVal: &clock}}}})
	if !strings.Contains(sql, "NULL::text ||") {
		t.Fatalf("$hexCast of a time must be NULL instead of session-dependent text:\n%s", sql)
	}
}

func TestNestedTemporalStringCastsBindTheirOperandOnce(t *testing.T) {
	t.Parallel()

	operand := Value{Field: new(ModelStringPattern("$aas#idShort"))}
	for range 8 {
		inner := operand
		operand = Value{DateTimeCast: &Value{StrCast: &inner}}
	}
	instant := DateTimeLiteralPattern(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	sql := renderLogicalExpressionSQL(t, LogicalExpression{Eq: ComparisonItems{operand, {DateTimeVal: &instant}}})
	if strings.Count(sql, "AS value OFFSET 0") != 7 || strings.Count(sql, `"aas"."id_short"`) != 1 {
		t.Fatalf("each canonical string cast must bind its operand once behind an inlining fence:\n%s", sql)
	}
}

func TestHexCastsNormalizeLikeConstantFolding(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ input, want string }{
		{" 16#0a\t", "16#0A"}, {"\v16#AB\f", "16#AB"}, {"16#0G", ""}, {"0A", ""}, {"16#0A\u00a0", ""},
	} {
		got, ok := normalizeHexString(test.input)
		if ok != (test.want != "") || got != test.want {
			t.Errorf("normalizeHexString(%q) = %q, %t; want %q", test.input, got, ok, test.want)
		}
	}
	hex := HexLiteralPattern("16#0A")
	sql := renderLogicalExpressionSQL(t, LogicalExpression{Eq: ComparisonItems{{HexCast: fieldValue("$aas#idShort")}, {HexVal: &hex}}})
	if !strings.Contains(sql, `substring(upper(btrim("aas"."id_short"::text, $1)) from $2)`) {
		t.Fatalf("a hex cast must trim, upper-case, and validate the value:\n%s", sql)
	}
}

func TestDateTimeLiteralsAreTypedForSQL(t *testing.T) {
	t.Parallel()

	first := DateTimeLiteralPattern(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	second := DateTimeLiteralPattern(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	sql := renderLogicalExpressionSQL(t, LogicalExpression{Lt: ComparisonItems{{DateTimeVal: &first}, {DateTimeVal: &second}}})
	if strings.Count(sql, "::timestamptz") != 2 {
		t.Fatalf("date-time literals must never compare as text:\n%s", sql)
	}
}

func TestStringCastOfTimestampFieldIsCanonical(t *testing.T) {
	t.Parallel()

	created := ModelStringPattern("$aasdesc#createdAt")
	text := StandardString("2024-01-01T08:00:00Z")
	where, _, err := (&LogicalExpression{Eq: ComparisonItems{{StrCast: &Value{Field: &created}}, {StrVal: &text}}}).EvaluateToExpression(mustCollectorForRoot(t, "$aasdesc"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _, err := goqu.Dialect("postgres").From("descriptor").Where(where).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `'\+00:00$', 'Z')`) {
		t.Fatalf("$strCast of a TIMESTAMPTZ field must use the canonical Z form:\n%s", sql)
	}
}

func TestFailedNumericCastsYieldNoValue(t *testing.T) {
	t.Parallel()

	abc := StandardString("abc")
	yes := true
	for _, operand := range []Value{{StrVal: &abc}, {Boolean: &yes}, {Attribute: map[string]string{"GLOBAL": "UTCNOW"}}} {
		if value := resolveCastValue(Value{NumCast: &operand}, func(AttributeValue) any { return "2025-01-01T00:00:00Z" }); value != nil {
			t.Errorf("$numCast of %#v resolved to %#v, want no value", operand, value)
		}
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
