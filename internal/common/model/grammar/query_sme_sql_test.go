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

	if !strings.Contains(sql, `"submodel_element"."idshort_path" ~`) {
		t.Fatalf("expected regex idshort_path constraint for [] wildcard, got: %s", sql)
	}
	if !argListContains(args, `^New_TestList\[[0-9]+\]$`) {
		t.Fatalf("expected args to contain escaped prefix, got %#v", args)
	}
}

func TestSMEIDShortPathRegexKeepsListWildcardsWithinTheirSegments(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"a[]":       `^a\[[0-9]+\]$`,
		"a[].b[]":   `^a\[[0-9]+\]\.b\[[0-9]+\]$`,
		"a[1].b[]":  `^a\[1\]\.b\[[0-9]+\]$`,
		"A_B.C+D[]": `^A_B\.C\+D\[[0-9]+\]$`,
	}
	for path, expected := range tests {
		if actual := smeIDShortPathRegex(path); actual != expected {
			t.Fatalf("path %q: got %q, want %q", path, actual, expected)
		}
	}
}

func TestSMEIDShortPathSupportsStructuralFragments(t *testing.T) {
	tests := map[string]string{
		"$sme.a[]":                "a[]",
		"$sme.a[1].b[]":           "a[1].b[]",
		"$sme.a[].b[]#semanticId": "a[].b[]",
	}
	for field, expected := range tests {
		actual, ok := smeIDShortPathFromField(field)
		if !ok || actual != expected {
			t.Fatalf("field %q: got path %q with found=%t, want %q", field, actual, ok, expected)
		}
	}
	if _, ok := smeIDShortPathFromField("$sme"); ok {
		t.Fatal("root SME fragment must not report an idShort path")
	}
}

func TestSMEFragmentMatchPathlessSMEFieldsUseCurrentRow(t *testing.T) {
	collector, err := NewResolvedFieldPathCollectorForSMERow("sme")
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForSMERow returned error: %v", err)
	}
	fragment := FragmentStringPattern("$sme.NewTestList[]")
	matchCollector := collector.ForFragmentMatch(fragment)

	tests := []struct {
		field       ModelStringPattern
		correlation smeMatchCorrelation
	}{
		{field: "$sme#idShort", correlation: smeMatchSameRow},
		{field: "$sme#value", correlation: smeMatchSameRow},
		{field: "$sme#semanticId.keys[].value", correlation: smeMatchSameRow},
		{field: "$sm#idShort", correlation: smeMatchContainingSubmodel},
	}
	for _, test := range tests {
		resolved, resolveErr := ResolveScalarFieldToSQL(&test.field)
		if resolveErr != nil {
			t.Fatalf("ResolveScalarFieldToSQL(%q) returned error: %v", test.field, resolveErr)
		}
		if actual := matchCollector.smeCorrelationForResolved([]ResolvedFieldPath{resolved}); actual != test.correlation {
			t.Fatalf("field %q: got correlation %v, want %v", test.field, actual, test.correlation)
		}
	}
}

func TestSMERootFragmentMatchUsesCurrentRowForPathCondition(t *testing.T) {
	collector, err := NewResolvedFieldPathCollectorForSMERow("sme")
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForSMERow returned error: %v", err)
	}
	fragment := FragmentStringPattern("$sme")
	field := ModelStringPattern("$sme.NewTestList[]#value")
	resolved, err := ResolveScalarFieldToSQL(&field)
	if err != nil {
		t.Fatalf("ResolveScalarFieldToSQL returned error: %v", err)
	}

	actual := collector.ForFragmentMatch(fragment).smeCorrelationForResolved([]ResolvedFieldPath{resolved})
	if actual != smeMatchSameRow {
		t.Fatalf("got correlation %v, want current-row correlation", actual)
	}
}
