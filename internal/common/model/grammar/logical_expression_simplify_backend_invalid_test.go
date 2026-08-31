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

import "testing"

func TestSimplifyForBackendFilterNotDoesNotInvertInvalidAttribute(t *testing.T) {
	t.Parallel()

	blocked := StandardString("blocked")
	comparison := LogicalExpression{Eq: []Value{
		{Attribute: map[string]any{"CLAIM": "role"}},
		{StrVal: &blocked},
	}}
	expression := LogicalExpression{Not: &comparison}

	simplified, decision := expression.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
	if decision != SimplifyInvalid {
		t.Fatalf("decision = %v, want %v", decision, SimplifyInvalid)
	}
	if simplified.Boolean == nil || *simplified.Boolean {
		t.Fatalf("expected false invalid expression, got %#v", simplified)
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
	if decision != SimplifyInvalid {
		t.Fatalf("decision = %v, want %v", decision, SimplifyInvalid)
	}
}

func TestSimplifyForBackendFilterInRequiresScalarThenStringArray(t *testing.T) {
	t.Parallel()

	admin := StandardString("admin")
	roles := map[string]any{"CLAIM": "roles"}
	tests := []struct {
		name       string
		expression LogicalExpression
		resolved   any
		want       SimplifyDecision
	}{
		{
			name:       "member",
			expression: LogicalExpression{In: []Value{{StrVal: &admin}, {Attribute: roles}}},
			resolved:   []string{"reader", "admin"},
			want:       SimplifyTrue,
		},
		{
			name:       "not a member",
			expression: LogicalExpression{In: []Value{{StrVal: &admin}, {Attribute: roles}}},
			resolved:   []string{"reader", "administrator"},
			want:       SimplifyFalse,
		},
		{
			name:       "array must be second",
			expression: LogicalExpression{In: []Value{{Attribute: roles}, {StrVal: &admin}}},
			resolved:   []string{"admin"},
			want:       SimplifyInvalid,
		},
		{
			name:       "scalar right operand",
			expression: LogicalExpression{In: []Value{{StrVal: &admin}, {Attribute: roles}}},
			resolved:   "admin",
			want:       SimplifyInvalid,
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
