package auth

import (
	"encoding/json"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestQueryFilter_FilterExpressionsFor_ExactMatch(t *testing.T) {
	b := true
	expr := grammar.LogicalExpression{Boolean: &b}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#endpoints[2]": expr,
	}}

	entries := q.FilterExpressionEntriesFor("$aasdesc#endpoints[2]")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Fragment != "$aasdesc#endpoints[2]" {
		t.Fatalf("expected fragment %q, got %q", "$aasdesc#endpoints[2]", entries[0].Fragment)
	}
	jEntry, _ := json.Marshal(entries[0].Expression)
	jWantEntry, _ := json.Marshal(expr)
	if string(jEntry) != string(jWantEntry) {
		t.Fatalf("expected %s, got %s", string(jWantEntry), string(jEntry))
	}

	got := q.FilterExpressionsFor("$aasdesc#endpoints[2]")
	if len(got) != 1 {
		t.Fatalf("expected 1 expression, got %d", len(got))
	}
	jGot, _ := json.Marshal(got[0])
	jWantExpr, _ := json.Marshal(expr)
	if string(jGot) != string(jWantExpr) {
		t.Fatalf("expected %s, got %s", string(jWantExpr), string(jGot))
	}
}

func TestQueryFilter_FilterExpressionEntriesFor_WildcardIncludesLiteralAndIndexed(t *testing.T) {
	b1 := true
	b2 := false
	b3 := true

	// literal [] + indexed entries
	exprWildcard := grammar.LogicalExpression{Boolean: &b1}
	expr2 := grammar.LogicalExpression{Boolean: &b2}
	expr10 := grammar.LogicalExpression{Boolean: &b3}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#specificAssetIds[]":   exprWildcard,
		"$aasdesc#specificAssetIds[2]":  expr2,
		"$aasdesc#specificAssetIds[10]": expr10,
	}}

	entries := q.FilterExpressionEntriesFor("$aasdesc#specificAssetIds[]")
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Fragment != "$aasdesc#specificAssetIds[]" {
		t.Fatalf("expected first fragment %q, got %q", "$aasdesc#specificAssetIds[]", entries[0].Fragment)
	}
	if entries[1].Fragment != "$aasdesc#specificAssetIds[2]" {
		t.Fatalf("expected second fragment %q, got %q", "$aasdesc#specificAssetIds[2]", entries[1].Fragment)
	}
	if entries[2].Fragment != "$aasdesc#specificAssetIds[10]" {
		t.Fatalf("expected third fragment %q, got %q", "$aasdesc#specificAssetIds[10]", entries[2].Fragment)
	}
}

func TestQueryFilter_FilterExpressionsFor_WildcardMatchesIndexedAndSorted(t *testing.T) {
	b1 := true
	b3 := true

	expr2 := grammar.LogicalExpression{Boolean: &b1}
	expr10 := grammar.LogicalExpression{Boolean: &b3}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#endpoints[10]": expr10,
		"$aasdesc#endpoints[2]":  expr2,
		"$aasdesc#other[1]":      expr2,
	}}

	entries := q.FilterExpressionEntriesFor("$aasdesc#endpoints[]")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Fragment != "$aasdesc#endpoints[2]" {
		t.Fatalf("expected first fragment %q, got %q", "$aasdesc#endpoints[2]", entries[0].Fragment)
	}
	if entries[1].Fragment != "$aasdesc#endpoints[10]" {
		t.Fatalf("expected second fragment %q, got %q", "$aasdesc#endpoints[10]", entries[1].Fragment)
	}

	got := q.FilterExpressionsFor("$aasdesc#endpoints[]")
	if len(got) != 2 {
		t.Fatalf("expected 2 expressions, got %d", len(got))
	}

	// Expect stable order: idx 2, then idx 10.
	j0, _ := json.Marshal(got[0])
	j1, _ := json.Marshal(got[1])
	want0, _ := json.Marshal(expr2)
	want1, _ := json.Marshal(expr10)

	if string(j0) != string(want0) {
		t.Fatalf("expected first %s, got %s", string(want0), string(j0))
	}
	if string(j1) != string(want1) {
		t.Fatalf("expected second %s, got %s", string(want1), string(j1))
	}
}

func TestQueryFilter_FilterExpressionEntriesFor_WildcardSuffixMustMatchPath(t *testing.T) {
	b1 := true
	b2 := false
	b3 := true

	exprLiteral := grammar.LogicalExpression{Boolean: &b1}
	expr2name := grammar.LogicalExpression{Boolean: &b2}
	expr2 := grammar.LogicalExpression{Boolean: &b3}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#specificAssetIds[]":       exprLiteral,
		"$aasdesc#specificAssetIds[2].name": expr2name,
		"$aasdesc#specificAssetIds[2]":      expr2,
	}}

	entries := q.FilterExpressionEntriesFor("$aasdesc#specificAssetIds[].name")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Fragment != "$aasdesc#specificAssetIds[2].name" {
		t.Fatalf("expected fragment %q, got %q", "$aasdesc#specificAssetIds[2].name", entries[0].Fragment)
	}
}

func TestQueryFilter_FilterExpressionFor_WildcardCombinesOr(t *testing.T) {
	b1 := true
	b2 := false
	exprA := grammar.LogicalExpression{Boolean: &b1}
	exprB := grammar.LogicalExpression{Boolean: &b2}

	q := QueryFilter{Filters: FragmentFilters{
		"$aasdesc#endpoints[0]": exprA,
		"$aasdesc#endpoints[1]": exprB,
	}}

	combined := q.FilterExpressionFor("$aasdesc#endpoints[]")
	if combined == nil {
		t.Fatalf("expected non-nil combined expression")
	}
	if len(combined.Or) != 2 {
		j, _ := json.Marshal(combined)
		t.Fatalf("expected OR with 2 entries, got %d: %s", len(combined.Or), string(j))
	}
}
