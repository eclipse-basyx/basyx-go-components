package grammar

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestLogicalExpression_BD_WithCollector_BuildsCTE(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$bd#specificAssetIds[0].externalSubjectId.keys[1].value"),
			strVal("WRITTEN_BY_X"),
		},
	}

	collector := mustCollectorForRoot(t, "$bd")
	whereExpr, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("aas_identifier").As("aas_identifier")).Select(goqu.V(1)).Where(whereExpr)
	sql, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	t.Logf("SQL: %s", sql)
	t.Logf("Args: %#v", args)

	if !strings.Contains(sql, "EXISTS") {
		t.Fatalf("expected EXISTS in SQL, got: %s", sql)
	}
	if !argListContains(args, "WRITTEN_BY_X") {
		t.Fatalf("expected args to contain %q, got %#v", "WRITTEN_BY_X", args)
	}
	if !argListContains(args, 0) || !argListContains(args, 1) {
		t.Fatalf("expected args to contain array indices 0 and 1, got %#v", args)
	}
}
