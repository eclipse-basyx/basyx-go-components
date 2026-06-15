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
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestQueryWrapper_SMECondition_ToSQL(t *testing.T) {
	jsonStr := `{
		"Query": {
			"$condition": {
				"$eq": [
					{"$field": "$sme.temperature#value"},
					{"$strVal": "100"}
				]
			}
		}
	}`

	var wrapper QueryWrapper
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		t.Fatalf("Failed to unmarshal SME query: %v", err)
	}
	if wrapper.Query.Condition == nil {
		t.Fatal("Expected Condition to be set")
	}

	whereExpr, _, err := wrapper.Query.Condition.EvaluateToExpression(nil)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	// Use aliases matching the resolver output.
	ds := d.From(goqu.T("submodel_element").As("submodel_element")).
		LeftJoin(goqu.T("property_element").As("property_element"), goqu.On(goqu.I("property_element.id").Eq(goqu.I("submodel_element.id")))).
		Select(goqu.V(1)).
		Where(whereExpr).
		Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	// We expect the idShortPath binding to become a plain AND constraint (no EXISTS join graph for SME).
	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS for SME query, got: %s", sql)
	}
	if !strings.Contains(sql, "\"submodel_element\".\"idshort_path\"") {
		t.Fatalf("expected idshort_path constraint in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "property_element") {
		t.Fatalf("expected SME value expression to reference property_element, got: %s", sql)
	}

	if !argListContains(args, "temperature") {
		t.Fatalf("expected args to contain %q, got %#v", "temperature", args)
	}
	if !argListContains(args, "100") {
		t.Fatalf("expected args to contain %q, got %#v", "100", args)
	}
}

func TestQueryWrapper_SMECondition_ListWildcardValueType_ToSQL(t *testing.T) {
	jsonStr := `{
		"Query": {
			"$condition": {
				"$eq": [
					{"$field": "$sme.New_TestList[]#valueType"},
					{"$strVal": "xs:string"}
				]
			}
		}
	}`

	var wrapper QueryWrapper
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		t.Fatalf("Failed to unmarshal SME query: %v", err)
	}
	if wrapper.Query.Condition == nil {
		t.Fatal("Expected Condition to be set")
	}

	whereExpr, _, err := wrapper.Query.Condition.EvaluateToExpression(nil)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("submodel_element").As("submodel_element")).
		LeftJoin(goqu.T("property_element").As("property_element"), goqu.On(goqu.I("property_element.id").Eq(goqu.I("submodel_element.id")))).
		Select(goqu.V(1)).
		Where(whereExpr).
		Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if !strings.Contains(sql, `"submodel_element"."idshort_path" LIKE`) {
		t.Fatalf("expected LIKE idshort_path constraint for [] wildcard, got: %s", sql)
	}
	if !strings.Contains(sql, `ESCAPE`) {
		t.Fatalf("expected ESCAPE clause for [] wildcard idshort_path constraint, got: %s", sql)
	}
	if !argListContains(args, "New!_TestList[%") {
		t.Fatalf("expected args to contain escaped prefix, got %#v", args)
	}
}
