package grammar

import (
	"strings"
	"testing"
)

func TestLogicalExpression_ToSQL_StrValUsesTextCast(t *testing.T) {
	// Use a field that previously used implicit casting when compared to NumVal.
	// Here we compare it to StrVal("123") and expect no implicit cast in SQL.
	le := LogicalExpression{Eq: ComparisonItems{field("$aasdesc#id"), strVal("123")}}

	sql, args := toPreparedSQLForDescriptor(t, le)

	if strings.Contains(sql, "::double precision") {
		t.Fatalf("did not expect numeric cast for StrVal operand, got: %s", sql)
	}
	if strings.Contains(sql, "::text") {
		t.Fatalf("did not expect implicit ::text cast in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "= ?") {
		t.Fatalf("expected SQL to contain '= ?', got: %s", sql)
	}
	if !argListContains(args, "123") {
		t.Fatalf("expected args to contain %q, got %#v", "123", args)
	}
}

func TestLogicalExpression_ToSQL_WithCollector_ExistsPredicateUsesTextCast(t *testing.T) {
	// This fieldidentifier requires joins and therefore gets translated into an EXISTS predicate.
	// The test verifies that the predicate is still generated in the EXISTS subquery
	// without implicit ::text casting.
	le := LogicalExpression{Eq: ComparisonItems{field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"), strVal("123")}}

	sql, args := toPreparedSQLForDescriptor(t, le)

	if !strings.Contains(sql, "EXISTS") {
		t.Fatalf("expected EXISTS in SQL, got: %s", sql)
	}
	if strings.Contains(sql, "::double precision") {
		t.Fatalf("did not expect numeric cast for StrVal operand, got: %s", sql)
	}
	if strings.Contains(sql, "::text") {
		t.Fatalf("did not expect implicit ::text cast in SQL, got: %s", sql)
	}
	if !argListContains(args, "123") {
		t.Fatalf("expected args to contain %q, got %#v", "123", args)
	}
	// Ensure the array index bindings are present (specificAssetIds[0], keys[1]).
	if !argListContains(args, 0) {
		t.Fatalf("expected args to contain %d, got %#v", 0, args)
	}
	if !argListContains(args, 1) {
		t.Fatalf("expected args to contain %d, got %#v", 1, args)
	}
}
