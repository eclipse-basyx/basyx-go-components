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
	"fmt"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func buildSMESQL(t *testing.T, expr LogicalExpression) (string, []interface{}) {
	t.Helper()
	collector := mustCollectorForRoot(t, "$sme")
	whereExpr, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("submodel_element").As("sme")).
		LeftJoin(
			goqu.T("property_element").As("property_element"),
			goqu.On(goqu.I("property_element.id").Eq(goqu.I("sme.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)

	sql, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	return sql, args
}

func argsString(args []interface{}) string {
	return fmt.Sprint(args)
}

func argsContainInt(args []interface{}, want int) bool {
	for _, arg := range args {
		switch v := arg.(type) {
		case int:
			if v == want {
				return true
			}
		case int32:
			if int(v) == want {
				return true
			}
		case int64:
			if int(v) == want {
				return true
			}
		}
	}
	return false
}

func TestLogicalExpression_SME_WithCollector_BuildsCTE(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$sme.temperature#value"),
			strVal("100"),
		},
	}

	sql, args := buildSMESQL(t, expr)
	t.Logf("SQL: %s", sql)

	if !strings.Contains(sql, "\"submodel_element\".\"idshort_path\" = ?") {
		t.Fatalf("expected idshort_path binding in EXISTS SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "property_element") {
		t.Fatalf("expected property_element in SQL, got: %s", sql)
	}
	if !strings.Contains(argsString(args), "temperature") {
		t.Fatalf("expected args to contain %q, got: %v", "temperature", args)
	}
	if !strings.Contains(argsString(args), "100") {
		t.Fatalf("expected args to contain %q, got: %v", "100", args)
	}
}

func TestLogicalExpression_SME_WithCollector_MultiConditions(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Eq: ComparisonItems{
					field("$sme.temperature#value"),
					strVal("100"),
				},
			},
			{
				Eq: ComparisonItems{
					field("$sme.temperature#valueType"),
					strVal("string"),
				},
			},
		},
	}

	sql, args := buildSMESQL(t, expr)
	t.Logf("SQL: %s", sql)

	if !strings.Contains(sql, "\"submodel_element\".\"idshort_path\" = ?") {
		t.Fatalf("expected idshort_path binding in EXISTS SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "CASE WHEN property_element.value_bool IS NOT NULL") {
		t.Fatalf("expected CASE-based valueType expression in EXISTS SQL, got: %s", sql)
	}
	argsText := argsString(args)
	if !strings.Contains(argsText, "temperature") {
		t.Fatalf("expected args to contain %q, got: %v", "temperature", args)
	}
	if !strings.Contains(argsText, "100") {
		t.Fatalf("expected args to contain %q, got: %v", "100", args)
	}
	if !strings.Contains(argsText, "string") {
		t.Fatalf("expected args to contain %q, got: %v", "string", args)
	}
}

func TestLogicalExpression_SME_NestedOrAnd(t *testing.T) {
	expr := LogicalExpression{
		Or: []LogicalExpression{
			{
				And: []LogicalExpression{
					{
						Eq: ComparisonItems{
							field("$sme.temperature#value"),
							strVal("100"),
						},
					},
					{
						Eq: ComparisonItems{
							field("$sme.temperature#valueType"),
							strVal("string"),
						},
					},
				},
			},
			{
				Contains: StringItems{
					strField("$sme.temperature#value"),
					strString("block"),
				},
			},
		},
	}

	sql, args := buildSMESQL(t, expr)
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("expected OR in SQL, got: %s", sql)
	}
	argsText := argsString(args)
	if !strings.Contains(argsText, "temperature") {
		t.Fatalf("expected args to contain %q, got: %v", "temperature", args)
	}
	if !strings.Contains(argsText, "100") {
		t.Fatalf("expected args to contain %q, got: %v", "100", args)
	}
	if !strings.Contains(argsText, "string") {
		t.Fatalf("expected args to contain %q, got: %v", "string", args)
	}
	if !strings.Contains(argsText, "block") {
		t.Fatalf("expected args to contain %q, got: %v", "block", args)
	}
}

func TestLogicalExpression_SME_WithCasts(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Gt: ComparisonItems{
					Value{NumCast: valuePtr(field("$sme.temperature#value"))},
					Value{NumVal: floatPtr(10)},
				},
			},
			{
				Lt: ComparisonItems{
					Value{TimeCast: valuePtr(field("$sme.temperature#value"))},
					Value{TimeVal: timePtr("12:00")},
				},
			},
		},
	}

	sql, args := buildSMESQL(t, expr)
	if !strings.Contains(sql, "::double precision") {
		t.Fatalf("expected double precision cast in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "::time") {
		t.Fatalf("expected time cast in SQL, got: %s", sql)
	}
	argsText := argsString(args)
	if !strings.Contains(argsText, "10") {
		t.Fatalf("expected args to contain %q, got: %v", "10", args)
	}
	if !strings.Contains(argsText, "12:00") {
		t.Fatalf("expected args to contain %q, got: %v", "12:00", args)
	}
}

func TestLogicalExpression_SME_SemanticID(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$sme.temperature#semanticId.keys[0].value"),
			strVal("urn:sm"),
		},
	}

	sql, args := buildSMESQL(t, expr)
	if !strings.Contains(sql, "semantic_id_reference_key") {
		t.Fatalf("expected semantic_id_reference_key in SQL, got: %s", sql)
	}
	argsText := argsString(args)
	if !strings.Contains(argsText, "urn:sm") {
		t.Fatalf("expected args to contain %q, got: %v", "urn:sm", args)
	}
	if !strings.Contains(sql, "\"position\" = ?") {
		t.Fatalf("expected SQL to contain position placeholder, got: %s", sql)
	}
	if !argsContainInt(args, 0) {
		t.Fatalf("expected args to include position 0, got: %v", args)
	}
}

func TestLogicalExpression_SME_NotRegex(t *testing.T) {
	expr := LogicalExpression{
		Not: &LogicalExpression{
			Regex: StringItems{
				strField("$sme.temperature#value"),
				strString("^foo.*"),
			},
		},
	}

	sql, args := buildSMESQL(t, expr)
	if !strings.Contains(sql, "NOT") {
		t.Fatalf("expected NOT in SQL, got: %s", sql)
	}
	if !strings.Contains(argsString(args), "^foo.*") {
		t.Fatalf("expected args to contain %q, got: %v", "^foo.*", args)
	}
}

func TestLogicalExpression_SME_MultipleIdShortPaths(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Eq: ComparisonItems{
					field("$sme.motor.speed#value"),
					strVal("900"),
				},
			},
			{
				Eq: ComparisonItems{
					field("$sme.motor.temperature#value"),
					strVal("55"),
				},
			},
		},
	}

	sql, args := buildSMESQL(t, expr)
	if !strings.Contains(sql, "\"submodel_element\".\"idshort_path\" = ?") {
		t.Fatalf("expected idshort_path binding in EXISTS SQL, got: %s", sql)
	}
	argsText := argsString(args)
	if !strings.Contains(argsText, "motor.speed") {
		t.Fatalf("expected args to contain %q, got: %v", "motor.speed", args)
	}
	if !strings.Contains(argsText, "motor.temperature") {
		t.Fatalf("expected args to contain %q, got: %v", "motor.temperature", args)
	}
	if !strings.Contains(argsText, "900") {
		t.Fatalf("expected args to contain %q, got: %v", "900", args)
	}
	if !strings.Contains(argsText, "55") {
		t.Fatalf("expected args to contain %q, got: %v", "55", args)
	}
}

func timePtr(v string) *TimeLiteralPattern {
	t := TimeLiteralPattern(v)
	return &t
}
