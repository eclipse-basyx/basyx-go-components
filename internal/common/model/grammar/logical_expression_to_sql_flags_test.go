package grammar

import (
	"fmt"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/lib/pq"
)

func TestLogicalExpression_EvaluateToExpression_WithCollector_UsesFlagAlias(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"),
			strVal("WRITTEN_BY_X"),
		},
	}

	collector := NewResolvedFieldPathCollector("descriptor_flags")
	whereExpr, resolved, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}
	t.Logf("whereExpr: %#v", whereExpr)
	t.Logf("resolved: %#v", resolved)

	entries := collector.Entries()
	t.Logf("collector entries: %#v", entries)
	if len(entries) != 1 {
		t.Fatalf("expected 1 collected entry, got %d", len(entries))
	}

	alias := entries[0].Alias

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1)).Where(whereExpr).Prepared(true)
	sql, _, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	t.Logf("Generated SQL: %s", sql)

	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "\"descriptor_flags\".\""+alias+"\"") {
		t.Fatalf("expected SQL to reference flag alias %q, got: %s", alias, sql)
	}

	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved field path, got %d", len(resolved))
	}
	r := resolved[0]
	if r.Column != "external_subject_reference_key.value" {
		t.Fatalf("expected resolved column %q, got %q", "external_subject_reference_key.value", r.Column)
	}
	if len(r.ArrayBindings) != 2 {
		t.Fatalf("expected 2 array bindings, got %d", len(r.ArrayBindings))
	}

	var sawSpecificAsset, sawReferenceKey bool
	for _, b := range r.ArrayBindings {
		t.Logf("ArrayBinding: %#v", b)
		switch b.Alias {
		case "specific_asset_id.position":
			if b.Index.intValue == nil || *b.Index.intValue != 0 {
				t.Fatalf("expected specific_asset_id.position binding with index 0, got %#v", b.Index)
			}
			sawSpecificAsset = true
		case "external_subject_reference_key.position":
			if b.Index.intValue == nil || *b.Index.intValue != 1 {
				t.Fatalf("expected external_subject_reference_key.position binding with index 1, got %#v", b.Index)
			}
			sawReferenceKey = true
		}
	}

	if !sawSpecificAsset || !sawReferenceKey {
		t.Fatalf("expected bindings for specific_asset_id and external_subject_reference_key, got %#v", r.ArrayBindings)
	}
}

func TestLogicalExpression_EvaluateToExpression_WithCollector_ReusesAlias(t *testing.T) {
	expr := LogicalExpression{
		Eq: ComparisonItems{
			field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"),
			strVal("WRITTEN_BY_X"),
		},
	}

	collector := NewResolvedFieldPathCollector("descriptor_flags")

	_, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	firstEntries := collector.Entries()
	t.Logf("firstEntries: %#v", firstEntries)
	if len(firstEntries) != 1 {
		t.Fatalf("expected 1 collected entry, got %d", len(firstEntries))
	}
	firstAlias := firstEntries[0].Alias

	_, _, err = expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	secondEntries := collector.Entries()
	t.Logf("secondEntries: %#v", secondEntries)
	if len(secondEntries) != 1 {
		t.Fatalf("expected 1 collected entry after reuse, got %d", len(secondEntries))
	}
	if secondEntries[0].Alias != firstAlias {
		t.Fatalf("expected alias reuse (%q), got %q", firstAlias, secondEntries[0].Alias)
	}
}

func TestBuildResolvedFieldPathFlagCTEs_GroupsSameJoinGraph(t *testing.T) {
	expr1 := LogicalExpression{
		Eq: ComparisonItems{
			field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"),
			strVal("WRITTEN_BY_X"),
		},
	}
	expr2 := LogicalExpression{
		Eq: ComparisonItems{
			field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[2].value"),
			strVal("WRITTEN_BY_Y"),
		},
	}

	collector := NewResolvedFieldPathCollector("descriptor_flags")

	a, _, err := expr1.EvaluateToExpression(collector)
	_, _ = fmt.Println(a)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}
	b, _, err := expr2.EvaluateToExpression(collector)
	_, _ = fmt.Println(b)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	entries := collector.Entries()
	ctes, err := BuildResolvedFieldPathFlagCTEs(collector.CTEAlias, entries)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEs returned error: %v", err)
	}
	if len(ctes) != 1 {
		t.Fatalf("expected 1 CTE, got %d", len(ctes))
	}

	sql, _, err := ctes[0].Dataset.Prepared(true).ToSQL()
	_, _ = fmt.Println(sql)
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if !strings.Contains(sql, "BOOL_OR") {
		t.Fatalf("expected BOOL_OR in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "GROUP BY") {
		t.Fatalf("expected GROUP BY in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "FROM \"specific_asset_id\"") {
		t.Fatalf("expected base specific_asset_id in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "JOIN \"reference\" AS \"external_subject_reference\"") {
		t.Fatalf("expected join to external_subject_reference in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "JOIN \"reference_key\" AS \"external_subject_reference_key\"") {
		t.Fatalf("expected join to external_subject_reference_key in SQL, got: %s", sql)
	}

	for _, entry := range entries {
		if !strings.Contains(sql, "AS \""+entry.Alias+"\"") {
			t.Fatalf("expected flag alias %q in SQL, got: %s", entry.Alias, sql)
		}
	}
}

func TestResolvedFieldPathFlags_FinalQueryExample(t *testing.T) {
	expr := LogicalExpression{
		Or: []LogicalExpression{
			{
				And: []LogicalExpression{
					{
						Eq: ComparisonItems{
							field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"),
							strVal("WRITTEN_BY_X"),
						},
					},
					{
						Eq: ComparisonItems{
							field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[2].value"),
							strVal("WRITTEN_BY_X"),
						},
					},
				},
			},
			{
				Eq: ComparisonItems{
					field("$aasdesc#specificAssetIds[2].externalSubjectId.keys[2].value"),
					strVal("WRITTEN_BY_Y"),
				},
			},
		},
	}

	collector := NewResolvedFieldPathCollector("descriptor_flags")
	whereExpr, resolved, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}
	t.Logf("resolved paths: %#v", resolved)
	t.Logf("collector entries: %#v", collector.Entries())

	descriptorIDs := []int64{1334}
	where := goqu.L("specific_asset_id.descriptor_id = ANY(?::bigint[])", pq.Array(descriptorIDs))

	ctes, err := BuildResolvedFieldPathFlagCTEsWithWhere(collector.CTEAlias, collector.Entries(), where)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithWhere returned error: %v", err)
	}
	if len(ctes) != 1 {
		t.Fatalf("expected 1 CTE, got %d", len(ctes))
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1))
	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset)
	}
	ds = ds.LeftJoin(
		goqu.T(ctes[0].Alias),
		goqu.On(goqu.I(ctes[0].Alias+".descriptor_id").Eq(goqu.I("descriptor.id"))),
	).Where(whereExpr).Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	t.Logf("Final SQL: %s", sql)
	t.Logf("Args: %#v", args)

	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "WITH descriptor_flags") {
		t.Fatalf("expected CTE in SQL, got: %s", sql)
	}
	for _, entry := range collector.Entries() {
		if !strings.Contains(sql, "\"descriptor_flags\".\""+entry.Alias+"\"") {
			t.Fatalf("expected flag alias %q in SQL, got: %s", entry.Alias, sql)
		}
	}
}

func TestResolvedFieldPathFlags_MultipleCTEsExample(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Eq: ComparisonItems{
					field("$aasdesc#specificAssetIds[0].name"),
					strVal("customerPartId"),
				},
			},
			{
				Eq: ComparisonItems{
					field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"),
					strVal("WRITTEN_BY_X"),
				},
			},
		},
	}

	collector := NewResolvedFieldPathCollector("descriptor_flags")
	whereExpr, resolved, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}
	t.Logf("resolved paths: %#v", resolved)
	t.Logf("collector entries: %#v", collector.Entries())

	descriptorIDs := []int64{1334}
	where := goqu.L("specific_asset_id.descriptor_id = ANY(?::bigint[])", pq.Array(descriptorIDs))

	ctes, err := BuildResolvedFieldPathFlagCTEsWithWhere(collector.CTEAlias, collector.Entries(), where)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithWhere returned error: %v", err)
	}
	if len(ctes) != 2 {
		t.Fatalf("expected 2 CTEs, got %d", len(ctes))
	}

	for _, cte := range ctes {
		sql, args, err := cte.Dataset.Prepared(true).ToSQL()
		if err != nil {
			t.Fatalf("ToSQL returned error: %v", err)
		}
		t.Logf("CTE %s SQL: %s", cte.Alias, sql)
		t.Logf("CTE %s Args: %#v", cte.Alias, args)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1))
	for _, cte := range ctes {
		ds = ds.With(cte.Alias, cte.Dataset)
		ds = ds.LeftJoin(
			goqu.T(cte.Alias),
			goqu.On(goqu.I(cte.Alias+".descriptor_id").Eq(goqu.I("descriptor.id"))),
		)
	}
	ds = ds.Where(whereExpr).Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	t.Logf("Final SQL: %s", sql)
	t.Logf("Args: %#v", args)

	if !strings.Contains(sql, "WITH descriptor_flags_1") || !strings.Contains(sql, "descriptor_flags_2") {
		t.Fatalf("expected multiple CTEs in SQL, got: %s", sql)
	}
}
