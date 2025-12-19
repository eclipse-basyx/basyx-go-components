package grammar

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestSpecificAssetIdsWildcardHasNoPositionConstraint(t *testing.T) {
	field := ModelStringPattern("$aasdesc#specificAssetIds[].value")
	lit := StandardString("foo")

	expr, err := HandleComparison(&Value{Field: &field}, &Value{StrVal: &lit}, "$eq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql, _, err := goqu.From("descriptor").Where(expr).ToSQL()
	if err != nil {
		t.Fatalf("failed to build SQL: %v", err)
	}

	if strings.Contains(sql, "specific_asset_id.position") {
		t.Fatalf("wildcard specificAssetIds[] must not add position constraint, got SQL: %s", sql)
	}
}

func TestSpecificAssetIdsIndexedAddsPositionConstraint(t *testing.T) {
	field := ModelStringPattern("$aasdesc#specificAssetIds[2].value")
	lit := StandardString("bar")

	expr, err := HandleComparison(&Value{Field: &field}, &Value{StrVal: &lit}, "$eq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql, _, err := goqu.From("descriptor").Where(expr).ToSQL()
	if err != nil {
		t.Fatalf("failed to build SQL: %v", err)
	}

	if !strings.Contains(sql, "\"specific_asset_id\".\"position\"") {
		t.Fatalf("expected position constraint for specificAssetIds[2], got SQL: %s", sql)
	}
	if !strings.Contains(sql, `"specific_asset_id"."position" = 2`) {
		t.Fatalf("expected specific position filter for index 2, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "EXISTS") {
		t.Fatalf("expected EXISTS subquery for indexed array access, got SQL: %s", sql)
	}
}
