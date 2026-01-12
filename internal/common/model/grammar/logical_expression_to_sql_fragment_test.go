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

	whereExpr, resolved, err := le.evaluateFragmentToExpression(nil, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved field path, got %d", len(resolved))
	}
	if resolved[0].Column != "" {
		t.Fatalf("expected empty resolved column for fragment, got %q", resolved[0].Column)
	}
	if len(resolved[0].ArrayBindings) != 1 {
		t.Fatalf("expected 1 array binding, got %d", len(resolved[0].ArrayBindings))
	}
	b := resolved[0].ArrayBindings[0]
	if b.Alias != "aas_descriptor_endpoint.position" {
		t.Fatalf("expected binding alias %q, got %q", "aas_descriptor_endpoint.position", b.Alias)
	}
	if b.Index.intValue == nil || *b.Index.intValue != 2 {
		t.Fatalf("expected binding index 2, got %#v", b.Index)
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
	if strings.Contains(sql, "descriptor_flags") {
		t.Fatalf("did not expect CTE flag alias without collector, got SQL: %s", sql)
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

	whereExpr, resolved, err := le.evaluateFragmentToExpression(nil, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved field path, got %d", len(resolved))
	}
	if len(resolved[0].ArrayBindings) != 2 {
		t.Fatalf("expected 2 array bindings, got %d", len(resolved[0].ArrayBindings))
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

	whereExpr, resolved, err := le.evaluateFragmentToExpression(nil, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved field path, got %d", len(resolved))
	}
	if len(resolved[0].ArrayBindings) != 0 {
		t.Fatalf("expected 0 array bindings, got %d", len(resolved[0].ArrayBindings))
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

	whereExpr, resolved, err := le.evaluateFragmentToExpression(nil, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved field path, got %d", len(resolved))
	}
	if len(resolved[0].ArrayBindings) != 2 {
		t.Fatalf("expected 2 array bindings, got %d", len(resolved[0].ArrayBindings))
	}

	seenSpecificAsset := false
	seenExternalKey := false
	for _, b := range resolved[0].ArrayBindings {
		switch b.Alias {
		case "specific_asset_id.position":
			seenSpecificAsset = true
			if b.Index.intValue == nil || *b.Index.intValue != 0 {
				t.Fatalf("expected specific_asset_id.position binding with index 0, got %#v", b.Index)
			}
		case "external_subject_reference_key.position":
			seenExternalKey = true
			if b.Index.intValue == nil || *b.Index.intValue != 0 {
				t.Fatalf("expected external_subject_reference_key.position binding with index 0, got %#v", b.Index)
			}
		default:
			t.Fatalf("unexpected binding alias %q", b.Alias)
		}
	}
	if !seenSpecificAsset || !seenExternalKey {
		t.Fatalf("expected bindings for specific_asset_id and external_subject_reference_key, got %#v", resolved[0].ArrayBindings)
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

	collector := mustCollectorForRoot(t, "$aasdesc", "descriptor_flags")
	whereExpr, resolved, err := le.evaluateFragmentToExpression(collector, fragment)
	if err != nil {
		t.Fatalf("evaluateFragmentToExpression returned error: %v", err)
	}
	if whereExpr == nil {
		t.Fatalf("expected whereExpr to be non-nil")
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved field path, got %d", len(resolved))
	}

	entries := collector.Entries()
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
	if strings.Contains(sql, "EXISTS") {
		t.Fatalf("did not expect EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "\"descriptor_flags_1\".\""+alias+"\"") {
		t.Fatalf("expected SQL to reference flag alias %q, got: %s", alias, sql)
	}

	ctes, err := BuildResolvedFieldPathFlagCTEsWithCollector(collector, entries, nil)
	if err != nil {
		t.Fatalf("BuildResolvedFieldPathFlagCTEsWithCollector returned error: %v", err)
	}
	if len(ctes) != 1 {
		t.Fatalf("expected 1 CTE, got %d", len(ctes))
	}

	cteSQL, _, err := ctes[0].Dataset.Prepared(true).ToSQL()
	if err != nil {
		t.Fatalf("CTE ToSQL returned error: %v", err)
	}
	if strings.Contains(strings.ToLower(cteSQL), "true") {
		t.Fatalf("did not expect literal TRUE in CTE SQL, got: %s", cteSQL)
	}
	if !strings.Contains(cteSQL, "FROM \"aas_descriptor\"") {
		t.Fatalf("expected CTE to be based on aas_descriptor, got: %s", cteSQL)
	}
	if !strings.Contains(cteSQL, "JOIN \"aas_descriptor_endpoint\" AS \"aas_descriptor_endpoint\"") {
		t.Fatalf("expected CTE to join aas_descriptor_endpoint, got: %s", cteSQL)
	}
	if !strings.Contains(cteSQL, "\"aas_descriptor_endpoint\".\"position\"") {
		t.Fatalf("expected CTE SQL to reference aas_descriptor_endpoint.position, got: %s", cteSQL)
	}
}
