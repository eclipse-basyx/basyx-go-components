package grammar

import (
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
	ds := d.From(goqu.T("submodel_element").As("submodel_element")).
		LeftJoin(
			goqu.T("property_element").As("property_element"),
			goqu.On(goqu.I("property_element.id").Eq(goqu.I("submodel_element.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)

	sql, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	return sql, args
}

func TestLogicalExpression_SME_WithCollector_BuildsCTE(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$sme.temperature#value"),
			strVal("100"),
		},
	}

	sql, _ := buildSMESQL(t, expr)
	t.Logf("SQL: %s", sql)

	if !strings.Contains(sql, "submodel_element__exists\".\"idshort_path\"") {
		t.Fatalf("expected idshort_path binding in EXISTS SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "property_element") {
		t.Fatalf("expected property_element in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "'temperature'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'temperature'", sql)
	}
	if !strings.Contains(sql, "'100'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'100'", sql)
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

	sql, _ := buildSMESQL(t, expr)
	t.Logf("SQL: %s", sql)

	if !strings.Contains(sql, "submodel_element__exists\".\"idshort_path\"") {
		t.Fatalf("expected idshort_path binding in EXISTS SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "property_element__exists\".\"value_type\"") {
		t.Fatalf("expected property_element.value_type in EXISTS SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "'temperature'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'temperature'", sql)
	}
	if !strings.Contains(sql, "'100'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'100'", sql)
	}
	if !strings.Contains(sql, "'string'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'string'", sql)
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

	sql, _ := buildSMESQL(t, expr)
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("expected OR in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "'temperature'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'temperature'", sql)
	}
	if !strings.Contains(sql, "'100'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'100'", sql)
	}
	if !strings.Contains(sql, "'string'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'string'", sql)
	}
	if !strings.Contains(sql, "'block'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'block'", sql)
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

	sql, _ := buildSMESQL(t, expr)
	if !strings.Contains(sql, "::double precision") {
		t.Fatalf("expected double precision cast in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "::time") {
		t.Fatalf("expected time cast in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, " 10") {
		t.Fatalf("expected SQL to contain %q, got: %s", " 10", sql)
	}
	if !strings.Contains(sql, "'12:00'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'12:00'", sql)
	}
}

func TestLogicalExpression_SME_SemanticID(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$sme.temperature#semanticId.keys[0].value"),
			strVal("urn:sm"),
		},
	}

	sql, _ := buildSMESQL(t, expr)
	if !strings.Contains(sql, "semantic_id_reference_key") {
		t.Fatalf("expected semantic_id_reference_key in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "'urn:sm'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'urn:sm'", sql)
	}
	if !strings.Contains(sql, "position\" = 0") {
		t.Fatalf("expected SQL to contain position binding 0, got: %s", sql)
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

	sql, _ := buildSMESQL(t, expr)
	if !strings.Contains(sql, "NOT") {
		t.Fatalf("expected NOT in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "'^foo.*'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'^foo.*'", sql)
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

	sql, _ := buildSMESQL(t, expr)
	if !strings.Contains(sql, "submodel_element__exists\".\"idshort_path\"") {
		t.Fatalf("expected idshort_path binding in EXISTS SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "'motor.speed'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'motor.speed'", sql)
	}
	if !strings.Contains(sql, "'motor.temperature'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'motor.temperature'", sql)
	}
	if !strings.Contains(sql, "'900'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'900'", sql)
	}
	if !strings.Contains(sql, "'55'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'55'", sql)
	}
}

func timePtr(v string) *TimeLiteralPattern {
	t := TimeLiteralPattern(v)
	return &t
}
