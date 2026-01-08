package grammar

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

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

	collector, err := NewResolvedFieldPathCollectorForRoot("$sm", "sm_flags")
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
	if len(ctes) != 2 {
		t.Fatalf("expected 2 SM CTEs, got %d", len(ctes))
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("submodel").As("s")).Select(goqu.V(1)).Where(whereExpr)
	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset).
			LeftJoin(
				goqu.T(cte.Alias),
				goqu.On(goqu.I(cte.Alias+".descriptor_id").Eq(goqu.I("s.id"))),
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
	if !strings.Contains(sql, "WITH sm_flags_1") || !strings.Contains(sql, "sm_flags_2") {
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
