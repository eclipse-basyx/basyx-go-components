package grammar

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func buildSMESQL(t *testing.T, expr LogicalExpression) (string, []interface{}) {
	t.Helper()
	collector := mustCollectorForRoot(t, "$sme", "sme_flags")
	whereExpr, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	ctes, err := BuildResolvedFieldPathFlagCTEsWithCollector(collector, collector.Entries(), nil)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithCollector returned error: %v", err)
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
				goqu.On(goqu.I(cte.Alias+".root_id").Eq(goqu.I("submodel_element.id"))),
			)
	}

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

	sql, args := buildSMESQL(t, expr)
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

	sql, args := buildSMESQL(t, expr)
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
	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "WITH sme_flags_1") {
		t.Fatalf("expected sme_flags_1 CTE in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("expected OR in SQL, got: %s", sql)
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
	if !argListContains(args, "block") {
		t.Fatalf("expected args to contain %q, got %#v", "block", args)
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
	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "::double precision") {
		t.Fatalf("expected double precision cast in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "::time") {
		t.Fatalf("expected time cast in SQL, got: %s", sql)
	}
	if !argListContains(args, float64(10)) {
		t.Fatalf("expected args to contain %v, got %#v", float64(10), args)
	}
	if !argListContains(args, "12:00") {
		t.Fatalf("expected args to contain %q, got %#v", "12:00", args)
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
	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "semantic_id_reference_key") {
		t.Fatalf("expected semantic_id_reference_key in SQL, got: %s", sql)
	}
	if !argListContains(args, "urn:sm") {
		t.Fatalf("expected args to contain %q, got %#v", "urn:sm", args)
	}
	if !argListContains(args, 0) {
		t.Fatalf("expected args to contain %d, got %#v", 0, args)
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
	if !argListContains(args, "^foo.*") {
		t.Fatalf("expected args to contain %q, got %#v", "^foo.*", args)
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
	if !strings.Contains(sql, "\"submodel_element\".\"idshort_path\"") {
		t.Fatalf("expected idshort_path binding in SQL, got: %s", sql)
	}
	if !argListContains(args, "motor.speed") {
		t.Fatalf("expected args to contain %q, got %#v", "motor.speed", args)
	}
	if !argListContains(args, "motor.temperature") {
		t.Fatalf("expected args to contain %q, got %#v", "motor.temperature", args)
	}
	if !argListContains(args, "900") {
		t.Fatalf("expected args to contain %q, got %#v", "900", args)
	}
	if !argListContains(args, "55") {
		t.Fatalf("expected args to contain %q, got %#v", "55", args)
	}
}

func timePtr(v string) *TimeLiteralPattern {
	t := TimeLiteralPattern(v)
	return &t
}
