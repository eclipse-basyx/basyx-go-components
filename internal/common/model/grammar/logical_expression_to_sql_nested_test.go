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

	collector := mustCollectorForRoot(t, "$aasdesc", "descriptor_flags")
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
	ctes, err := BuildResolvedFieldPathFlagCTEsWithCollector(collector, collector.Entries(), nil)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithCollector returned error: %v", err)
	}
	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset).
			LeftJoin(
				goqu.T(cte.Alias),
				goqu.On(goqu.I(cte.Alias+".root_id").Eq(goqu.I("descriptor.id"))),
			)
	}
	ds = ds.Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
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
	if !strings.Contains(sql, "descriptor_flags_1") {
		t.Fatalf("expected descriptor_flags_1 in SQL, got: %s", sql)
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

	collector := mustCollectorForRoot(t, "$aasdesc", "descriptor_flags")
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
	ctes, err := BuildResolvedFieldPathFlagCTEsWithCollector(collector, collector.Entries(), nil)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithCollector returned error: %v", err)
	}
	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset).
			LeftJoin(
				goqu.T(cte.Alias),
				goqu.On(goqu.I(cte.Alias+".root_id").Eq(goqu.I("descriptor.id"))),
			)
	}
	ds = ds.Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("expected OR in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "NOT") {
		t.Fatalf("expected NOT in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "descriptor_flags_1") {
		t.Fatalf("expected descriptor_flags_1 in SQL, got: %s", sql)
	}
	if !argListContains(args, "WRITTEN_BY_X") {
		t.Fatalf("expected args to contain %q, got %#v", "WRITTEN_BY_X", args)
	}
}

func TestLogicalExpression_EvaluateToExpression_FieldToFieldComparisonForbidden(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{field("$aasdesc#idShort"), field("$aasdesc#id")},
	}

	collector := mustCollectorForRoot(t, "$aasdesc", "descriptor_flags")
	_, _, err := expr.EvaluateToExpression(collector)
	if err == nil {
		t.Fatal("expected error for field-to-field comparison, got nil")
	}
}

func TestLUL(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{field("$aasdesc#endpoints[0]"), strVal("djn")},
	}

	collector := mustCollectorForRoot(t, "$aasdesc", "descriptor_flags")
	_, re, err := expr.EvaluateToExpression(collector)
	_, _ = fmt.Println(re)
	if err == nil {
		t.Fatal("expected error for field-to-field comparison, got nil")
	}
}
