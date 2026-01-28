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

	ctes, err := BuildResolvedFieldPathFlagCTEsWithCollector(collector, collector.Entries(), nil)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithCollector returned error: %v", err)
	}
	if len(ctes) != 1 {
		t.Fatalf("expected 1 BD CTE, got %d", len(ctes))
	}

	cteSQL, cteArgs, err := ctes[0].Dataset.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("CTE ToSQL returned error: %v", err)
	}
	t.Logf("CTE SQL: %s", cteSQL)
	t.Logf("CTE Args: %#v", cteArgs)

	if !strings.Contains(cteSQL, "FROM \"specific_asset_id\" AS \"specific_asset_id\"") {
		t.Fatalf("expected CTE to select from specific_asset_id, got: %s", cteSQL)
	}
	if !strings.Contains(cteSQL, "JOIN \"reference\" AS \"external_subject_reference\"") {
		t.Fatalf("expected CTE to join external_subject_reference, got: %s", cteSQL)
	}
	if !strings.Contains(cteSQL, "JOIN \"reference_key\" AS \"external_subject_reference_key\"") {
		t.Fatalf("expected CTE to join external_subject_reference_key, got: %s", cteSQL)
	}
	if !strings.Contains(cteSQL, "GROUP BY \"specific_asset_id\".\"aasref\"") {
		t.Fatalf("expected CTE to group by specific_asset_id.aasref, got: %s", cteSQL)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("aas_identifier").As("aas_identifier")).Select(goqu.V(1)).Where(whereExpr)
	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset).
			LeftJoin(
				goqu.T(cte.Alias),
				goqu.On(goqu.I(cte.Alias+".root_id").Eq(goqu.I("aas_identifier.id"))),
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
	if !strings.Contains(sql, "WITH flagtable_1") {
		t.Fatalf("expected flagtable_1 CTE in SQL, got: %s", sql)
	}
	if !argListContains(args, "WRITTEN_BY_X") {
		t.Fatalf("expected args to contain %q, got %#v", "WRITTEN_BY_X", args)
	}
	if !argListContains(args, 0) || !argListContains(args, 1) {
		t.Fatalf("expected args to contain array indices 0 and 1, got %#v", args)
	}
}
