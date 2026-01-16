package grammar

import (
	"fmt"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestLogicalExpression_SMDesc_WithCollector_BuildsCTE(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Eq: ComparisonItems{
					field("$smdesc#idShort"),
					strVal("sub-short"),
				},
			},
			{
				Eq: ComparisonItems{
					field("$smdesc#semanticId.keys[0].value"),
					strVal("urn:sm"),
				},
			},
		},
	}

	collector, err := NewResolvedFieldPathCollectorForRoot(CollectorRootSMDesc)
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
		t.Fatalf("expected 2 SMDesc CTEs, got %d", len(ctes))
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		LeftJoin(
			goqu.T("submodel_descriptor").As("submodel_descriptor"),
			goqu.On(goqu.I("submodel_descriptor.aas_descriptor_id").Eq(goqu.I("aas_descriptor.descriptor_id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)

	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset).
			LeftJoin(
				goqu.T(cte.Alias),
				goqu.On(goqu.I(cte.Alias+".root_id").Eq(goqu.I("submodel_descriptor.descriptor_id"))),
			)
	}

	sql, args, err := ds.Prepared(true).ToSQL()
	_, _ = fmt.Println(sql)
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	t.Logf("SQL: %s", sql)
	t.Logf("Args: %#v", args)

	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "WITH flagtable_1") || !strings.Contains(sql, "flagtable_2") {
		t.Fatalf("expected multiple SMDesc CTEs in SQL, got: %s", sql)
	}
	if !argListContains(args, "sub-short") {
		t.Fatalf("expected args to contain %q, got %#v", "sub-short", args)
	}
	if !argListContains(args, "urn:sm") {
		t.Fatalf("expected args to contain %q, got %#v", "urn:sm", args)
	}
	if !argListContains(args, 0) {
		t.Fatalf("expected args to contain %d, got %#v", 0, args)
	}
}
