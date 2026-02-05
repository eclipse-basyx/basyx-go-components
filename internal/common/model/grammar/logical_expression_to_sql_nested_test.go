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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestLogicalExpression_EvaluateToExpression_NestedTree_WithExistsAndNot(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Or: []LogicalExpression{
					{
						Eq: ComparisonItems{field("$aasdesc#idShort"), strVal("shell-short")},
					},
					{
						Eq: ComparisonItems{field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"), strVal("WRITTEN_BY_X")},
					},
				},
			},
			{
				Not: &LogicalExpression{
					Contains: StringItems{strField("$aasdesc#assetType"), strString("blocked")},
				},
			},
		},
	}

	collector := mustCollectorForRoot(t, "$aasdesc")
	whereExpr, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)
	ds = ds.Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if !strings.Contains(sql, "EXISTS") {
		t.Fatalf("expected EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("expected OR in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "NOT") {
		t.Fatalf("expected NOT in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "LIKE") {
		t.Fatalf("expected LIKE for $contains in SQL, got: %s", sql)
	}

	// Ensure important bindings/values are present in args.
	if !argListContains(args, 0) {
		t.Fatalf("expected args to contain %d, got %#v", 0, args)
	}
	if !argListContains(args, 1) {
		t.Fatalf("expected args to contain %d, got %#v", 1, args)
	}
	if !argListContains(args, "shell-short") {
		t.Fatalf("expected args to contain %q, got %#v", "shell-short", args)
	}
	if !argListContains(args, "WRITTEN_BY_X") {
		t.Fatalf("expected args to contain %q, got %#v", "WRITTEN_BY_X", args)
	}
	if !argListContains(args, "blocked") {
		t.Fatalf("expected args to contain %q, got %#v", "blocked", args)
	}
}

func TestLogicalExpression_EvaluateToExpression_NestedJSON_UnmarshalAndGenerateSQL(t *testing.T) {
	jsonStr := `{
		"$and": [
			{"$eq": [
				{"$field": "$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"},
				{"$strVal": "WRITTEN_BY_X"}
			]},
			{"$or": [
				{"$eq": [
					{"$field": "$aasdesc#idShort"},
					{"$strVal": "shell-short"}
				]},
				{"$not": {
					"$contains": [
						{"$field": "$aasdesc#assetType"},
						{"$strVal": "blocked"}
					]
				}}
			]}
		]
	}`

	var le LogicalExpression
	if err := json.Unmarshal([]byte(jsonStr), &le); err != nil {
		t.Fatalf("failed to unmarshal LogicalExpression: %v", err)
	}

	collector := mustCollectorForRoot(t, "$aasdesc")
	whereExpr, _, err := le.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)
	ds = ds.Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if !strings.Contains(sql, "EXISTS") {
		t.Fatalf("expected EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("expected OR in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "NOT") {
		t.Fatalf("expected NOT in SQL, got: %s", sql)
	}
	if !argListContains(args, "WRITTEN_BY_X") {
		t.Fatalf("expected args to contain %q, got %#v", "WRITTEN_BY_X", args)
	}
}

func TestLogicalExpression_EvaluateToExpression_FieldToFieldComparisonForbidden(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{field("$aasdesc#idShort"), field("$aasdesc#id")},
	}

	collector := mustCollectorForRoot(t, "$aasdesc")
	_, _, err := expr.EvaluateToExpression(collector)
	if err == nil {
		t.Fatal("expected error for field-to-field comparison, got nil")
	}
}

func TestLUL(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{field("$aasdesc#endpoints[0]"), strVal("djn")},
	}

	collector := mustCollectorForRoot(t, "$aasdesc")
	_, re, err := expr.EvaluateToExpression(collector)
	_, _ = fmt.Println(re)
	if err == nil {
		t.Fatal("expected error for field-to-field comparison, got nil")
	}
}
