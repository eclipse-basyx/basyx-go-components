package grammar

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestLogicalExpression_SME_WithCollector_BuildsCTE(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$sme.temperature#value"),
			strVal("100"),
		},
	}

	collector, err := NewResolvedFieldPathCollectorForRoot("$sme", "sme_flags")
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}
	whereExpr, resolved, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}
	t.Logf("resolved: %#v", resolved)

	ctes, err := BuildResolvedFieldPathFlagCTEsWithCollector(collector, collector.Entries(), nil)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithCollector returned error: %v", err)
	}
	if len(ctes) != 1 {
		t.Fatalf("expected 1 SME CTE, got %d", len(ctes))
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("submodel_element").As("submodel_element")).
		LeftJoin(
			goqu.T("property_element").As("property_element"),
			goqu.On(goqu.I("property_element.id").Eq(goqu.I("submodel_element.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)

	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset).
			LeftJoin(
				goqu.T(cte.Alias),
				goqu.On(goqu.I(cte.Alias+".descriptor_id").Eq(goqu.I("submodel_element.id"))),
			)
	}

	sql, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	t.Logf("SQL: %s", sql)
	t.Logf("Args: %#v", args)

	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "WITH sme_flags_1") {
		t.Fatalf("expected sme_flags_1 CTE in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "\"sme_flags_1\".\"rfp_1\"") {
		t.Fatalf("expected SME flag alias in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "\"submodel_element\".\"idshort_path\"") {
		t.Fatalf("expected idshort_path binding in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "property_element") {
		t.Fatalf("expected property_element in SQL, got: %s", sql)
	}
	if !argListContains(args, "temperature") {
		t.Fatalf("expected args to contain %q, got %#v", "temperature", args)
	}
	if !argListContains(args, "100") {
		t.Fatalf("expected args to contain %q, got %#v", "100", args)
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

	collector, err := NewResolvedFieldPathCollectorForRoot("$sme", "sme_flags")
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}
	whereExpr, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	ctes, err := BuildResolvedFieldPathFlagCTEsWithCollector(collector, collector.Entries(), nil)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithCollector returned error: %v", err)
	}
	if len(ctes) != 1 {
		t.Fatalf("expected 1 SME CTE, got %d", len(ctes))
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("submodel_element").As("submodel_element")).
		LeftJoin(
			goqu.T("property_element").As("property_element"),
			goqu.On(goqu.I("property_element.id").Eq(goqu.I("submodel_element.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)

	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset).
			LeftJoin(
				goqu.T(cte.Alias),
				goqu.On(goqu.I(cte.Alias+".descriptor_id").Eq(goqu.I("submodel_element.id"))),
			)
	}

	sql, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	t.Logf("SQL: %s", sql)
	t.Logf("Args: %#v", args)

	if !strings.Contains(sql, "WITH sme_flags_1") {
		t.Fatalf("expected sme_flags_1 CTE in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "\"submodel_element\".\"idshort_path\"") {
		t.Fatalf("expected idshort_path binding in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "\"property_element\".\"value_type\"") {
		t.Fatalf("expected property_element.value_type in SQL, got: %s", sql)
	}
	if !argListContains(args, "temperature") {
		t.Fatalf("expected args to contain %q, got %#v", "temperature", args)
	}
	if !argListContains(args, "100") {
		t.Fatalf("expected args to contain %q, got %#v", "100", args)
	}
	if !argListContains(args, "string") {
		t.Fatalf("expected args to contain %q, got %#v", "string", args)
	}
}
