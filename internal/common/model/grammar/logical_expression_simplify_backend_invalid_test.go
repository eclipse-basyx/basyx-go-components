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

func TestSimplifyForBackendFilterNotDoesNotInvertInvalidAttribute(t *testing.T) {
	t.Parallel()

	blocked := StandardString("blocked")
	comparison := LogicalExpression{Eq: []Value{
		{Attribute: map[string]any{"CLAIM": "role"}},
		{StrVal: &blocked},
	}}
	expression := LogicalExpression{Not: &comparison}

	simplified, decision := expression.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
	if decision != SimplifyIndeterminate {
		t.Fatalf("decision = %v, want %v", decision, SimplifyIndeterminate)
	}
	if !simplified.Indeterminate {
		t.Fatalf("expected indeterminate expression, got %#v", simplified)
	}
}

func TestSimplifyForBackendFilterNotDoesNotInvertInvalidRegex(t *testing.T) {
	t.Parallel()

	value := StandardString("value")
	pattern := StandardString("[")
	comparison := LogicalExpression{Regex: StringItems{
		{StrVal: &value},
		{StrVal: &pattern},
	}}
	expression := LogicalExpression{Not: &comparison}

	_, decision := expression.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
	if decision != SimplifyIndeterminate {
		t.Fatalf("decision = %v, want %v", decision, SimplifyIndeterminate)
	}
}

func TestSimplifyForBackendFilterClaimPathContainsRequiresStringArray(t *testing.T) {
	t.Parallel()

	admin := StandardString("admin")
	roles := map[string]any{"CLAIMPATH": "/roles"}
	tests := []struct {
		name       string
		expression LogicalExpression
		resolved   any
		want       SimplifyDecision
	}{
		{
			name:       "member",
			expression: LogicalExpression{Contains: StringItems{{Attribute: roles}, {StrVal: &admin}}},
			resolved:   ClaimValue{Value: []any{"reader", "admin"}},
			want:       SimplifyTrue,
		},
		{
			name:       "not a member",
			expression: LogicalExpression{Contains: StringItems{{Attribute: roles}, {StrVal: &admin}}},
			resolved:   ClaimValue{Value: []any{"reader", "administrator"}},
			want:       SimplifyFalse,
		},
		{
			name:       "array must be first",
			expression: LogicalExpression{Contains: StringItems{{StrVal: &admin}, {Attribute: roles}}},
			resolved:   ClaimValue{Value: []any{"admin"}},
			want:       SimplifyIndeterminate,
		},
		{
			name:       "scalar right operand",
			expression: LogicalExpression{Contains: StringItems{{Attribute: roles}, {StrVal: &admin}}},
			resolved:   ClaimValue{Value: "admin"},
			want:       SimplifyIndeterminate,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, decision := testCase.expression.SimplifyForBackendFilter(func(AttributeValue) any {
				return testCase.resolved
			})
			if decision != testCase.want {
				t.Fatalf("decision = %v, want %v", decision, testCase.want)
			}
		})
	}
}

func TestSimplifyForBackendFilterThreeValuedTruthTables(t *testing.T) {
	t.Parallel()

	trueValue := true
	falseValue := false
	missing := StandardString("missing")
	indeterminate := LogicalExpression{Eq: ComparisonItems{
		{Attribute: map[string]any{"CLAIM": "missing"}},
		{StrVal: &missing},
	}}
	fieldName := ModelStringPattern("$sm#id")
	undecided := LogicalExpression{Eq: ComparisonItems{{Field: &fieldName}, {StrVal: &missing}}}
	tests := []struct {
		name       string
		expression LogicalExpression
		want       SimplifyDecision
	}{
		{name: "and false dominates", expression: LogicalExpression{And: []LogicalExpression{{Boolean: &falseValue}, indeterminate}}, want: SimplifyFalse},
		{name: "or true dominates", expression: LogicalExpression{Or: []LogicalExpression{{Boolean: &trueValue}, indeterminate}}, want: SimplifyTrue},
		{name: "and otherwise indeterminate", expression: LogicalExpression{And: []LogicalExpression{{Boolean: &trueValue}, indeterminate}}, want: SimplifyIndeterminate},
		{name: "or otherwise indeterminate", expression: LogicalExpression{Or: []LogicalExpression{{Boolean: &falseValue}, indeterminate}}, want: SimplifyIndeterminate},
		{name: "not indeterminate", expression: LogicalExpression{Not: &indeterminate}, want: SimplifyIndeterminate},
		{name: "and backend dependent remains backend dependent", expression: LogicalExpression{And: []LogicalExpression{undecided, indeterminate}}, want: SimplifyUndecided},
		{name: "or backend dependent remains backend dependent", expression: LogicalExpression{Or: []LogicalExpression{undecided, indeterminate}}, want: SimplifyUndecided},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, decision := test.expression.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
			if decision != test.want {
				t.Fatalf("decision = %v, want %v", decision, test.want)
			}
		})
	}
}

func TestIndeterminateBackendBranchRendersSQLNull(t *testing.T) {
	t.Parallel()

	expression := LogicalExpression{Indeterminate: true}
	sqlExpression, _, err := expression.EvaluateToExpression(nil)
	if err != nil {
		t.Fatalf("EvaluateToExpression() error = %v", err)
	}
	query, _, err := goqu.Dialect("postgres").From("resource").Select(goqu.V(1)).Where(sqlExpression).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL() error = %v", err)
	}
	if !strings.Contains(query, "NULL::boolean") || strings.Contains(query, "COALESCE") {
		t.Fatalf("indeterminate SQL = %q, want NULL without coercion", query)
	}
}
