package grammar

import (
	"fmt"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestLogicalExpression_EvaluateToExpressionWithNegatedFragments_NoCollector_ORsWithNegatedFragment(t *testing.T) {
	le := &LogicalExpression{Eq: ComparisonItems{field("$aasdesc#idShort"), strVal("shell-short")}}

	fragments := []FragmentStringPattern{FragmentStringPattern("$aasdesc#endpoints[2]")}

	whereExpr, _, err := le.EvaluateToExpressionWithNegatedFragments(nil, fragments)
	if err != nil {
		t.Fatalf("EvaluateToExpressionWithNegatedFragments returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr).
		Prepared(true)
	sqlStr, args, err := ds.ToSQL()
	_, _ = fmt.Println(sqlStr, args)
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	if strings.Contains(strings.ToLower(sqlStr), "true") {
		t.Fatalf("did not expect literal TRUE in SQL, got: %s", sqlStr)
	}

	if !strings.Contains(sqlStr, " OR ") {
		t.Fatalf("expected OR in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(strings.ToUpper(sqlStr), "NOT") {
		t.Fatalf("expected NOT in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "\"aas_descriptor_endpoint\".\"position\"") {
		t.Fatalf("expected endpoint position binding in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "\"aas_descriptor\".\"id_short\"") {
		t.Fatalf("expected idShort column in SQL, got: %s", sqlStr)
	}

	// Prepared args also include SELECT goqu.V(1).
	foundShellShort := false
	found2 := false
	for _, a := range args {
		if v, ok := a.(string); ok && v == "shell-short" {
			foundShellShort = true
		}
		if v, ok := a.(int); ok && v == 2 {
			found2 = true
		}
		if v, ok := a.(int64); ok && v == 2 {
			found2 = true
		}
	}
	if !foundShellShort {
		t.Fatalf("expected args to contain shell-short, got: %#v", args)
	}
	if !found2 {
		t.Fatalf("expected args to contain 2, got: %#v", args)
	}
}

func TestLogicalExpression_EvaluateToExpressionWithNegatedFragments_NoCollector_NoIndexes_WildcardFragment(t *testing.T) {
	le := &LogicalExpression{Eq: ComparisonItems{field("$aasdesc#idShort"), strVal("shell-short")}}

	fragments := []FragmentStringPattern{FragmentStringPattern("$aasdesc#endpoints[]")}

	whereExpr, _, err := le.EvaluateToExpressionWithNegatedFragments(nil, fragments)
	if err != nil {
		t.Fatalf("EvaluateToExpressionWithNegatedFragments returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr).
		Prepared(true)

	sqlStr, args, err := ds.ToSQL()
	_, _ = fmt.Println(sqlStr)
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	if strings.Contains(strings.ToLower(sqlStr), "true") {
		t.Fatalf("did not expect literal TRUE in SQL, got: %s", sqlStr)
	}
	if strings.Contains(sqlStr, " OR ") {
		t.Fatalf("did not expect OR for wildcard fragment guard, got: %s", sqlStr)
	}
	if strings.Contains(sqlStr, "1=1") {
		t.Fatalf("did not expect 1=1 to leak into combined SQL, got: %s", sqlStr)
	}
	if !argListContains(args, "shell-short") {
		t.Fatalf("expected args to contain %q, got %#v", "shell-short", args)
	}
}

func TestLogicalExpression_EvaluateToExpressionWithNegatedFragments_WithCollector_UsesFlagAndBuildsCTE(t *testing.T) {
	le := &LogicalExpression{Eq: ComparisonItems{field("$aasdesc#idShort"), strVal("shell-short")}}

	collector := mustCollectorForRoot(t, "$aasdesc")
	fragments := []FragmentStringPattern{FragmentStringPattern("$aasdesc#endpoints[2]")}

	whereExpr, _, err := le.EvaluateToExpressionWithNegatedFragments(collector, fragments)
	if err != nil {
		t.Fatalf("EvaluateToExpressionWithNegatedFragments returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)

	ds = ds.Prepared(true)
	sqlStr, _, err := ds.ToSQL()
	_, _ = fmt.Println(sqlStr)
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	if strings.Contains(strings.ToLower(sqlStr), "true") {
		t.Fatalf("did not expect literal TRUE in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(strings.ToUpper(sqlStr), "NOT") {
		t.Fatalf("expected NOT in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "\"aas_descriptor\".\"id_short\"") {
		t.Fatalf("expected idShort column in SQL, got: %s", sqlStr)
	}
	if strings.Contains(sqlStr, "flagtable") {
		t.Fatalf("did not expect flagtable CTE usage anymore, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "\"aas_descriptor_endpoint\".\"position\"") {
		t.Fatalf("expected endpoint position binding in SQL, got: %s", sqlStr)
	}
	if len(collector.Entries()) != 0 {
		t.Fatalf("expected collector to stay empty for fragments, got %d entries", len(collector.Entries()))
	}
}
