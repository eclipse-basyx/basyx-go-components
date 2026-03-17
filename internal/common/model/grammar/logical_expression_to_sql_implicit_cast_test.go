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
// Author: Martin Stemmer ( Fraunhofer IESE )

package grammar

import (
	"strings"
	"testing"
)

func TestLogicalExpression_ToSQL_StrValUsesTextCast(t *testing.T) {
	// Use a field that previously used implicit casting when compared to NumVal.
	// Here we compare it to StrVal("123") and expect no implicit cast in SQL.
	le := LogicalExpression{Eq: ComparisonItems{field("$aasdesc#id"), strVal("123")}}

	sql, args := toPreparedSQLForDescriptor(t, le)

	if strings.Contains(sql, "::double precision") {
		t.Fatalf("did not expect numeric cast for StrVal operand, got: %s", sql)
	}
	if strings.Contains(sql, "::text") {
		t.Fatalf("did not expect implicit ::text cast in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "= ?") {
		t.Fatalf("expected SQL to contain '= ?', got: %s", sql)
	}
	if !argListContains(args, "123") {
		t.Fatalf("expected args to contain %q, got %#v", "123", args)
	}
}

func TestLogicalExpression_ToSQL_WithCollector_ExistsPredicateUsesTextCast(t *testing.T) {
	// This fieldidentifier requires joins and therefore gets translated into an EXISTS predicate.
	// The test verifies that the predicate is still generated in the EXISTS subquery
	// without implicit ::text casting.
	le := LogicalExpression{Eq: ComparisonItems{field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"), strVal("123")}}

	sql, args := toPreparedSQLForDescriptor(t, le)

	if !strings.Contains(sql, "EXISTS") {
		t.Fatalf("expected EXISTS in SQL, got: %s", sql)
	}
	if strings.Contains(sql, "::double precision") {
		t.Fatalf("did not expect numeric cast for StrVal operand, got: %s", sql)
	}
	if strings.Contains(sql, "::text") {
		t.Fatalf("did not expect implicit ::text cast in SQL, got: %s", sql)
	}
	if !argListContains(args, "123") {
		t.Fatalf("expected args to contain %q, got %#v", "123", args)
	}
	// Ensure the array index bindings are present (specificAssetIds[0], keys[1]).
	if !argListContains(args, 0) {
		t.Fatalf("expected args to contain %d, got %#v", 0, args)
	}
	if !argListContains(args, 1) {
		t.Fatalf("expected args to contain %d, got %#v", 1, args)
	}
}

func TestLogicalExpression_SimplifyForBackendFilter_EnumTypeString_ConvertsToNumericLiteral(t *testing.T) {
	le := LogicalExpression{
		Eq: ComparisonItems{
			field("$aasdesc#specificAssetIds[0].externalSubjectId.type"),
			strVal("ExternalReference"),
		},
	}

	opts := DefaultSimplifyOptions()
	opts.EnableImplicitCasts = false
	simplified, decision := le.SimplifyForBackendFilterWithOptions(func(AttributeValue) any { return nil }, opts)
	if decision != SimplifyUndecided {
		t.Fatalf("expected undecided simplification result, got %v", decision)
	}
	if len(simplified.Eq) != 2 {
		t.Fatalf("expected 2 operands in simplified comparison, got %d", len(simplified.Eq))
	}
	if simplified.Eq[1].NumVal == nil {
		t.Fatalf("expected enum literal to be converted to numeric value, got %#v", simplified.Eq[1])
	}
	if simplified.Eq[1].StrVal != nil {
		t.Fatalf("did not expect enum literal to remain string, got %#v", simplified.Eq[1])
	}
	if simplified.Eq[0].StrCast != nil {
		t.Fatalf("did not expect field to be cast to text for valid enum literal, got %#v", simplified.Eq[0])
	}
}

func TestLogicalExpression_SimplifyForBackendFilter_EnumTypeInvalidString_FallsBackToTextCast(t *testing.T) {
	le := LogicalExpression{
		Eq: ComparisonItems{
			field("$aasdesc#specificAssetIds[0].externalSubjectId.type"),
			strVal("VIEWER_KEY"),
		},
	}

	opts := DefaultSimplifyOptions()
	opts.EnableImplicitCasts = false
	simplified, decision := le.SimplifyForBackendFilterWithOptions(func(AttributeValue) any { return nil }, opts)
	if decision != SimplifyUndecided {
		t.Fatalf("expected undecided simplification result, got %v", decision)
	}
	if len(simplified.Eq) != 2 {
		t.Fatalf("expected 2 operands in simplified comparison, got %d", len(simplified.Eq))
	}
	if simplified.Eq[0].StrCast == nil {
		t.Fatalf("expected enum field to be wrapped in text cast for invalid enum literal, got %#v", simplified.Eq[0])
	}
	if simplified.Eq[1].StrVal == nil {
		t.Fatalf("expected invalid enum literal to stay string, got %#v", simplified.Eq[1])
	}

	sql, args := toPreparedSQLForDescriptor(t, simplified)
	if !strings.Contains(sql, "::text") {
		t.Fatalf("expected SQL to contain text cast for enum mismatch fallback, got: %s", sql)
	}
	if !argListContains(args, "VIEWER_KEY") {
		t.Fatalf("expected args to contain %q, got %#v", "VIEWER_KEY", args)
	}
}
