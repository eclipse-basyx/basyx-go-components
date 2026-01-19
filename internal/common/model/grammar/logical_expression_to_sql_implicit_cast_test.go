package grammar

import (
	"strings"
	"testing"
)

func TestLogicalExpression_ToSQL_ImplicitCast_OtherOperandTypeDrivesCast_StrValForcesTextCast(t *testing.T) {
	// Use a field that is known to be implicitly cast to numeric when compared to NumVal
	// (see other tests using "$aasdesc#id" with NumVal). Here we compare it to StrVal("123")
	// and expect a ::text cast (driven by StrVal), not a numeric cast.
	le := LogicalExpression{Eq: ComparisonItems{field("$aasdesc#id"), strVal("123")}}

	sql, args := toPreparedSQLForDescriptor(t, le)

	if !strings.Contains(sql, "::text") {
		t.Fatalf("expected implicit ::text cast in SQL, got: %s", sql)
	}
	if strings.Contains(sql, "::double precision") {
		t.Fatalf("did not expect numeric cast for StrVal operand, got: %s", sql)
	}
	if strings.Contains(sql, "CASE WHEN") {
		t.Fatalf("did not expect guarded cast for ::text, got: %s", sql)
	}
	if !strings.Contains(sql, "= ?") {
		t.Fatalf("expected SQL to contain '= ?', got: %s", sql)
	}
	if !argListContains(args, "123") {
		t.Fatalf("expected args to contain %q, got %#v", "123", args)
	}
}

func TestLogicalExpression_ToSQL_ImplicitCast_WithCollector_FlagCTEPredicateUsesTextCast(t *testing.T) {
	// This fieldidentifier requires joins and therefore gets registered into the collector as a flag predicate.
	// The test verifies that implicit casting still uses the other operand type (StrVal => ::text),
	// and that the cast shows up in the generated CTE predicate.
	le := LogicalExpression{Eq: ComparisonItems{field("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value"), strVal("123")}}

	sql, args := toPreparedSQLForDescriptor(t, le)

	if !strings.Contains(sql, "WITH flagtable_1") {
		t.Fatalf("expected a flagtable_1 CTE (collector path), got: %s", sql)
	}
	if !strings.Contains(sql, "flagtable_1") {
		t.Fatalf("expected SQL to reference flagtable_1, got: %s", sql)
	}
	if !strings.Contains(sql, "::text") {
		t.Fatalf("expected implicit ::text cast in SQL, got: %s", sql)
	}
	if strings.Contains(sql, "::double precision") {
		t.Fatalf("did not expect numeric cast for StrVal operand, got: %s", sql)
	}
	if strings.Contains(sql, "CASE WHEN") {
		t.Fatalf("did not expect guarded cast for ::text, got: %s", sql)
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
