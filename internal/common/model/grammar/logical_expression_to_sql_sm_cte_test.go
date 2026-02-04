//go:build cte
// +build cte

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

	ctes, err := BuildResolvedFieldPathFlagCTEsWithCollector(collector, collector.Entries(), nil)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithCollector returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("submodel").As("s")).Select(goqu.V(1)).Where(whereExpr)
	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset).
			LeftJoin(
				goqu.T(cte.Alias),
				goqu.On(goqu.I(cte.Alias+".root_id").Eq(goqu.I("s.id"))),
			)
	}

	sql, args, err := ds.Prepared(true).ToSQL()
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

	sql, args := buildSMSQL(t, expr)
	t.Logf("SQL: %s", sql)
	t.Logf("Args: %#v", args)

	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "WITH flagtable_1") || !strings.Contains(sql, "flagtable_2") {
		t.Fatalf("expected multiple SM CTEs in SQL, got: %s", sql)
	}
	if !argListContains(args, "sm-1") {
		t.Fatalf("expected args to contain %q, got %#v", "sm-1", args)
	}
	if !argListContains(args, "urn:sm") {
		t.Fatalf("expected args to contain %q, got %#v", "urn:sm", args)
	}
	if !argListContains(args, 0) {
		t.Fatalf("expected args to contain %d, got %#v", 0, args)
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

	sql, args := buildSMSQL(t, expr)
	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "WITH flagtable_1") || !strings.Contains(sql, "flagtable_2") {
		t.Fatalf("expected multiple SM CTEs in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("expected OR in SQL, got: %s", sql)
	}
	if !argListContains(args, "sm-1") {
		t.Fatalf("expected args to contain %q, got %#v", "sm-1", args)
	}
	if !argListContains(args, "urn:sm") {
		t.Fatalf("expected args to contain %q, got %#v", "urn:sm", args)
	}
	if !argListContains(args, "^sm-.*") {
		t.Fatalf("expected args to contain %q, got %#v", "^sm-.*", args)
	}
	if !argListContains(args, 1) {
		t.Fatalf("expected args to contain %d, got %#v", 1, args)
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

	sql, args := buildSMSQL(t, expr)
	if !strings.Contains(sql, "NOT") {
		t.Fatalf("expected NOT in SQL, got: %s", sql)
	}
	if !argListContains(args, "blocked") {
		t.Fatalf("expected args to contain %q, got %#v", "blocked", args)
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

	sql, args := buildSMSQL(t, expr)
	if !strings.Contains(sql, "::double precision") {
		t.Fatalf("expected double precision cast in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "::time") {
		t.Fatalf("expected time cast in SQL, got: %s", sql)
	}
	if !argListContains(args, float64(5)) {
		t.Fatalf("expected args to contain %v, got %#v", float64(5), args)
	}
	if !argListContains(args, "12:00") {
		t.Fatalf("expected args to contain %q, got %#v", "12:00", args)
	}
}

func TestLogicalExpression_SM_SemanticType(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$sm#semanticId.keys[0].type"),
			strVal("GlobalReference"),
		},
	}

	sql, args := buildSMSQL(t, expr)
	if !strings.Contains(sql, "semantic_id_reference_key") {
		t.Fatalf("expected semantic_id_reference_key in SQL, got: %s", sql)
	}
	if !argListContains(args, "GlobalReference") {
		t.Fatalf("expected args to contain %q, got %#v", "GlobalReference", args)
	}
	if !argListContains(args, 0) {
		t.Fatalf("expected args to contain %d, got %#v", 0, args)
	}
}

func TestLogicalExpression_SM_SemanticValueDifferentIndex(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$sm#semanticId.keys[2].value"),
			strVal("urn:other"),
		},
	}

	_, args := buildSMSQL(t, expr)
	if !argListContains(args, "urn:other") {
		t.Fatalf("expected args to contain %q, got %#v", "urn:other", args)
	}
	if !argListContains(args, 2) {
		t.Fatalf("expected args to contain %d, got %#v", 2, args)
	}
}
