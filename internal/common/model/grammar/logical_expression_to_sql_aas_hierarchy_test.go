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

func buildAASHierarchySQL(t *testing.T, expression LogicalExpression) string {
	t.Helper()

	collector := mustCollectorForRoot(t, "$aas")
	whereExpression, _, err := expression.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	dataset := goqu.Dialect("postgres").
		From(goqu.T("aas")).
		Select(goqu.V(1)).
		Where(whereExpression)
	sql, _, err := dataset.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	return sql
}

func TestLogicalExpressionAASSimpleSubmodelConditionBuildsReferencedSubmodelExists(t *testing.T) {
	expression := LogicalExpression{
		Eq: ComparisonItems{
			field("$sm#idShort"),
			strVal("CarbonFootprint"),
		},
	}

	sql := buildAASHierarchySQL(t, expression)

	assertSQLContainsAll(t, sql,
		"EXISTS",
		"aas_submodel_reference",
		"aas_submodel_reference_key",
		"submodel_identifier",
		"id_short",
		"CarbonFootprint",
	)
}

func TestLogicalExpressionAASSimpleSMEConditionBuildsReferencedSubmodelElementExists(t *testing.T) {
	expression := LogicalExpression{
		Lt: ComparisonItems{
			Value{NumCast: valuePtr(field("$sme.AggregatedCarbonFootprint#value"))},
			Value{NumVal: floatPtr(48)},
		},
	}

	sql := buildAASHierarchySQL(t, expression)

	assertSQLContainsAll(t, sql,
		"aas_submodel_reference",
		"aas_submodel_reference_key",
		"submodel",
		"submodel_element",
		"property_element",
		"property_element__exists.value_num",
		"AggregatedCarbonFootprint",
		"double precision",
	)
	if strings.Contains(sql, "property_element.value_num") {
		t.Fatalf("expected raw property expression aliases to be rewritten for the correlated EXISTS: %s", sql)
	}
}

func TestLogicalExpressionAASMatchCorrelatesSubmodelAndSMEInOneExists(t *testing.T) {
	expression := LogicalExpression{
		Match: []MatchExpression{
			{Eq: ComparisonItems{field("$sm#idShort"), strVal("CarbonFootprint")}},
			{
				Lt: ComparisonItems{
					Value{NumCast: valuePtr(field("$sme.AggregatedCarbonFootprint#value"))},
					Value{NumVal: floatPtr(48)},
				},
			},
		},
	}

	sql := buildAASHierarchySQL(t, expression)

	if count := strings.Count(sql, "EXISTS"); count != 1 {
		t.Fatalf("expected one correlated EXISTS for MATCH, got %d: %s", count, sql)
	}
	assertSQLContainsAll(t, sql,
		"aas_submodel_reference",
		"submodel_identifier",
		"CarbonFootprint",
		"AggregatedCarbonFootprint",
	)
}

func TestLogicalExpressionAASAndKeepsSubmodelAndSMEInIndependentExists(t *testing.T) {
	expression := LogicalExpression{
		And: []LogicalExpression{
			{Eq: ComparisonItems{field("$sm#idShort"), strVal("CarbonFootprint")}},
			{
				Lt: ComparisonItems{
					Value{NumCast: valuePtr(field("$sme.AggregatedCarbonFootprint#value"))},
					Value{NumVal: floatPtr(48)},
				},
			},
		},
	}

	sql := buildAASHierarchySQL(t, expression)

	if count := strings.Count(sql, "EXISTS"); count != 2 {
		t.Fatalf("expected two independent EXISTS expressions without MATCH, got %d: %s", count, sql)
	}
}

func TestLogicalExpressionAASComplexConditionCombinesAASAndMatchedHierarchy(t *testing.T) {
	expression := LogicalExpression{
		And: []LogicalExpression{
			{Regex: StringItems{strField("$aas#idShort"), strString("^Factory-")}},
			{
				Match: []MatchExpression{
					{StartsWith: StringItems{strField("$sm#idShort"), strString("Carbon")}},
					{Eq: ComparisonItems{field("$sme.Metrics.AggregatedCarbonFootprint#valueType"), strVal("xs:double")}},
					{
						Ge: ComparisonItems{
							Value{NumCast: valuePtr(field("$sme.Metrics.AggregatedCarbonFootprint#value"))},
							Value{NumVal: floatPtr(10)},
						},
					},
				},
			},
		},
	}

	sql := buildAASHierarchySQL(t, expression)

	assertSQLContainsAll(t, sql,
		"Factory-",
		"Carbon",
		"Metrics.AggregatedCarbonFootprint",
		"xs:double",
		"double precision",
	)
}

func assertSQLContainsAll(t *testing.T, sql string, expected ...string) {
	t.Helper()
	for _, value := range expected {
		if !strings.Contains(sql, value) {
			t.Fatalf("expected SQL to contain %q, got: %s", value, sql)
		}
	}
}
