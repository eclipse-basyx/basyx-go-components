package grammar

import (
	"fmt"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestLogicalExpression_evaluateFragmentToExpression_NoCollector_AppliesBindings(t *testing.T) {
	le := LogicalExpression{}
	fragment := FragmentStringPattern("$aasdesc#endpoints[2]")

	whereExpr, _, err := le.evaluateFragmentToExpression(nil, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1)).Where(whereExpr).Prepared(true)
	sql, args, err := ds.ToSQL()
	_, _ = fmt.Println(sql, args)
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	if strings.Contains(strings.ToLower(sql), "true") {
		t.Fatalf("did not expect literal TRUE in SQL, got: %s", sql)
	}
	if strings.Contains(sql, "flagtable") {
		t.Fatalf("did not expect flagtable CTE usage anymore, got: %s", sql)
	}
	if !strings.Contains(sql, "\"aas_descriptor_endpoint\".\"position\"") {
		t.Fatalf("expected SQL to reference aas_descriptor_endpoint.position, got: %s", sql)
	}
	found := false
	for _, arg := range args {
		if arg == int64(2) || arg == 2 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected SQL args to include the bound index 2, got %#v", args)
	}
}

func TestLogicalExpression_evaluateFragmentToExpression_NoCollector_TwoIndexes_AppliesBothBindings(t *testing.T) {
	le := LogicalExpression{}
	fragment := FragmentStringPattern("$aasdesc#submodelDescriptors[1].endpoints[2]")

	whereExpr, _, err := le.evaluateFragmentToExpression(nil, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1)).Where(whereExpr).Prepared(true)
	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	if strings.Contains(strings.ToLower(sql), "true") {
		t.Fatalf("did not expect literal TRUE in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "\"submodel_descriptor\".\"position\"") {
		t.Fatalf("expected SQL to reference submodel_descriptor.position, got: %s", sql)
	}
	if !strings.Contains(sql, "\"submodel_descriptor_endpoint\".\"position\"") {
		t.Fatalf("expected SQL to reference submodel_descriptor_endpoint.position, got: %s", sql)
	}

	found1 := false
	found2 := false
	for _, arg := range args {
		if arg == int64(1) || arg == 1 {
			found1 = true
		}
		if arg == int64(2) || arg == 2 {
			found2 = true
		}
	}
	if !found1 {
		t.Fatalf("expected SQL args to include the bound index 1, got %#v", args)
	}
	if !found2 {
		t.Fatalf("expected SQL args to include the bound index 2, got %#v", args)
	}
}

func TestLogicalExpression_evaluateFragmentToExpression_NoCollector_NoIndexes_ReturnsNoBindingPredicate(t *testing.T) {
	le := LogicalExpression{}
	fragment := FragmentStringPattern("$aasdesc#endpoints[]")

	whereExpr, _, err := le.evaluateFragmentToExpression(nil, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1)).Where(whereExpr).Prepared(true)
	sql, _, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	if strings.Contains(strings.ToLower(sql), "true") {
		t.Fatalf("did not expect literal TRUE in SQL, got: %s", sql)
	}
	if strings.Contains(sql, "\"aas_descriptor_endpoint\".\"position\"") {
		t.Fatalf("did not expect any position binding in SQL, got: %s", sql)
	}
}

func TestLogicalExpression_evaluateFragmentToExpression_NoCollector_SpecificAssetIdsExternalSubjectKeys_AppliesBothBindings(t *testing.T) {
	le := LogicalExpression{}
	fragment := FragmentStringPattern("$aasdesc#specificAssetIds[0].externalSubjectId.keys[0]")

	whereExpr, _, err := le.evaluateFragmentToExpression(nil, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1)).Where(whereExpr).Prepared(true)
	sql, args, err := ds.ToSQL()
	_, _ = fmt.Println(sql, args)
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	if strings.Contains(strings.ToLower(sql), "true") {
		t.Fatalf("did not expect literal TRUE in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "\"specific_asset_id\".\"position\"") {
		t.Fatalf("expected SQL to reference specific_asset_id.position, got: %s", sql)
	}
	if !strings.Contains(sql, "\"external_subject_reference_key\".\"position\"") {
		t.Fatalf("expected SQL to reference external_subject_reference_key.position, got: %s", sql)
	}

	zeros := 0
	for _, arg := range args {
		if arg == int64(0) || arg == 0 {
			zeros++
		}
	}
	if zeros < 2 {
		t.Fatalf("expected SQL args to include two bound index 0 values, got %#v", args)
	}
}

func TestLogicalExpression_evaluateFragmentToExpression_WithCollector_UsesFlagAlias(t *testing.T) {
	le := LogicalExpression{}
	fragment := FragmentStringPattern("$aasdesc#endpoints[2]")

	collector := mustCollectorForRoot(t, "$aasdesc")
	whereExpr, _, err := le.evaluateFragmentToExpression(collector, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1)).Where(whereExpr).Prepared(true)
	sql, _, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	if strings.Contains(sql, "flagtable") {
		t.Fatalf("did not expect flagtable CTE usage anymore, got: %s", sql)
	}
	if !strings.Contains(sql, "\"aas_descriptor_endpoint\".\"position\"") {
		t.Fatalf("expected endpoint position binding in SQL, got: %s", sql)
	}
}
