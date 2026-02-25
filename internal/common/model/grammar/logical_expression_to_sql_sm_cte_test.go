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

	"github.com/doug-martin/goqu/v9"
)

func buildSMSQL(t *testing.T, expr LogicalExpression) (string, []interface{}) {
	t.Helper()
	collector := mustCollectorForRoot(t, "$sm")
	whereExpr, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("submodel")).Select(goqu.V(1)).Where(whereExpr)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	return sql, args
}

func TestLogicalExpression_SM_WithCollector_BuildsCTE(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Eq: ComparisonItems{
					field("$sm#idShort"),
					strVal("sm-1"),
				},
			},
			{
				Eq: ComparisonItems{
					field("$sm#semanticId.keys[0].value"),
					strVal("urn:sm"),
				},
			},
		},
	}

	sql, _ := buildSMSQL(t, expr)
	t.Logf("SQL: %s", sql)

	if !strings.Contains(sql, "'sm-1'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'sm-1'", sql)
	}
	if !strings.Contains(sql, "'urn:sm'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'urn:sm'", sql)
	}
	if !strings.Contains(sql, "position\" = 0") {
		t.Fatalf("expected SQL to contain position binding 0, got: %s", sql)
	}
}

func TestLogicalExpression_SM_NestedAndOr(t *testing.T) {
	expr := LogicalExpression{
		Or: []LogicalExpression{
			{
				And: []LogicalExpression{
					{
						Eq: ComparisonItems{
							field("$sm#idShort"),
							strVal("sm-1"),
						},
					},
					{
						Eq: ComparisonItems{
							field("$sm#semanticId.keys[1].value"),
							strVal("urn:sm"),
						},
					},
				},
			},
			{
				Regex: StringItems{
					strField("$sm#idShort"),
					strString("^sm-.*"),
				},
			},
		},
	}

	sql, _ := buildSMSQL(t, expr)
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("expected OR in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "'sm-1'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'sm-1'", sql)
	}
	if !strings.Contains(sql, "'urn:sm'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'urn:sm'", sql)
	}
	if !strings.Contains(sql, "'^sm-.*'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'^sm-.*'", sql)
	}
	if !strings.Contains(sql, "position\" = 1") {
		t.Fatalf("expected SQL to contain position binding 1, got: %s", sql)
	}
}

func TestLogicalExpression_SM_NotContains(t *testing.T) {
	expr := LogicalExpression{
		Not: &LogicalExpression{
			Contains: StringItems{
				strField("$sm#idShort"),
				strString("blocked"),
			},
		},
	}

	sql, _ := buildSMSQL(t, expr)
	if !strings.Contains(sql, "NOT") {
		t.Fatalf("expected NOT in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "'blocked'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'blocked'", sql)
	}
}

func TestLogicalExpression_SM_WithCasts(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Gt: ComparisonItems{
					Value{NumCast: valuePtr(field("$sm#idShort"))},
					Value{NumVal: floatPtr(5)},
				},
			},
			{
				Lt: ComparisonItems{
					Value{TimeCast: valuePtr(field("$sm#idShort"))},
					Value{TimeVal: timePtr("12:00")},
				},
			},
		},
	}

	sql, _ := buildSMSQL(t, expr)
	if !strings.Contains(sql, "::double precision") {
		t.Fatalf("expected double precision cast in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "::time") {
		t.Fatalf("expected time cast in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, " 5") {
		t.Fatalf("expected SQL to contain %q, got: %s", " 5", sql)
	}
	if !strings.Contains(sql, "'12:00'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'12:00'", sql)
	}
}

func TestLogicalExpression_SM_SemanticType(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$sm#semanticId.keys[0].type"),
			strVal("GlobalReference"),
		},
	}

	sql, _ := buildSMSQL(t, expr)
	if !strings.Contains(sql, "semantic_id_reference_key") {
		t.Fatalf("expected semantic_id_reference_key in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "'GlobalReference'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'GlobalReference'", sql)
	}
	if !strings.Contains(sql, "position\" = 0") {
		t.Fatalf("expected SQL to contain position binding 0, got: %s", sql)
	}
}

func TestLogicalExpression_SM_SemanticValueDifferentIndex(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$sm#semanticId.keys[2].value"),
			strVal("urn:other"),
		},
	}

	sql, _ := buildSMSQL(t, expr)
	if !strings.Contains(sql, "'urn:other'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'urn:other'", sql)
	}
	if !strings.Contains(sql, "position\" = 2") {
		t.Fatalf("expected SQL to contain position binding 2, got: %s", sql)
	}
}
