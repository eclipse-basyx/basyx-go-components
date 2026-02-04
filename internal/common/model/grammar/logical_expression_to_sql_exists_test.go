package grammar

import (
	"strings"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
)

func argListContains(args []interface{}, want interface{}) bool {
	switch w := want.(type) {
	case string:
		for _, a := range args {
			if s, ok := a.(string); ok && s == w {
				return true
			}
		}
		return false
	case bool:
		for _, a := range args {
			if b, ok := a.(bool); ok && b == w {
				return true
			}
		}
		return false
	case int:
		for _, a := range args {
			switch v := a.(type) {
			case int:
				if v == w {
					return true
				}
			case int64:
				if int(v) == w {
					return true
				}
			case float64:
				if int(v) == w {
					return true
				}
			}
		}
		return false
	case float64:
		for _, a := range args {
			switch v := a.(type) {
			case float64:
				if v == w {
					return true
				}
			case int:
				if float64(v) == w {
					return true
				}
			case int64:
				if float64(v) == w {
					return true
				}
			}
		}
		return false
	case time.Time:
		for _, a := range args {
			if tm, ok := a.(time.Time); ok && tm.Equal(w) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func TestHandleComparison_BuildsExistsForSpecificAssetExternalSubjectKeyValue(t *testing.T) {
	field := ModelStringPattern("$aasdesc#specificAssetIds[0].externalSubjectId.keys[1].value")
	lit := StandardString("WRITTEN_BY_X")

	left := Value{Field: &field}
	right := Value{StrVal: &lit}

	collector := mustCollectorForRoot(t, "$aasdesc")
	expr, _, err := HandleComparisonWithCollector(&left, &right, "$eq", collector)
	if err != nil {
		t.Fatalf("HandleComparison returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Where(expr)
	ds = ds.Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	// Ensure the EXISTS subquery includes the right joins.
	if !strings.Contains(sql, "EXISTS") {
		t.Fatalf("expected EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "JOIN \"reference\" AS \"external_subject_reference\"") {
		t.Fatalf("expected join to external_subject_reference in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "JOIN \"reference_key\" AS \"external_subject_reference_key\"") {
		t.Fatalf("expected join to external_subject_reference_key in SQL, got: %s", sql)
	}

	// Binding constraints (array indices).
	if !strings.Contains(sql, "\"specific_asset_id\".\"position\"") {
		t.Fatalf("expected specific_asset_id.position constraint in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "\"external_subject_reference_key\".\"position\"") {
		t.Fatalf("expected external_subject_reference_key.position constraint in SQL, got: %s", sql)
	}

	// Predicate on the resolved column.
	if !strings.Contains(sql, "\"external_subject_reference_key\".\"value\"") {
		t.Fatalf("expected predicate on external_subject_reference_key.value in SQL, got: %s", sql)
	}
	if !argListContains(args, 0) {
		t.Fatalf("expected args to contain %d, got %#v", 0, args)
	}
	if !argListContains(args, 1) {
		t.Fatalf("expected args to contain %d, got %#v", 1, args)
	}
	if !argListContains(args, string(lit)) {
		t.Fatalf("expected args to contain %q, got %#v", string(lit), args)
	}
}

func TestHandleComparison_BuildsExistsForSpecificAssetExternalSubjectKeyValue_WildcardsNoBindings(t *testing.T) {
	field := ModelStringPattern("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value")
	lit := StandardString("WRITTEN_BY_X")

	left := Value{Field: &field}
	right := Value{StrVal: &lit}

	collector := mustCollectorForRoot(t, "$aasdesc")
	expr, _, err := HandleComparisonWithCollector(&left, &right, "$eq", collector)
	if err != nil {
		t.Fatalf("HandleComparison returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Where(expr)
	ds = ds.Prepared(true)

	sql, args, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	// Ensure the EXISTS subquery includes the right joins, even without any index bindings.
	if !strings.Contains(sql, "EXISTS") {
		t.Fatalf("expected EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "JOIN \"reference\" AS \"external_subject_reference\"") {
		t.Fatalf("expected join to external_subject_reference in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "JOIN \"reference_key\" AS \"external_subject_reference_key\"") {
		t.Fatalf("expected join to external_subject_reference_key in SQL, got: %s", sql)
	}

	// No binding constraints for wildcards.
	if strings.Contains(sql, "\"specific_asset_id\".\"position\"") {
		t.Fatalf("did not expect specific_asset_id.position constraint in SQL, got: %s", sql)
	}
	if strings.Contains(sql, "\"external_subject_reference_key\".\"position\"") {
		t.Fatalf("did not expect external_subject_reference_key.position constraint in SQL, got: %s", sql)
	}

	// Predicate on the resolved column.
	if !strings.Contains(sql, "\"external_subject_reference_key\".\"value\"") {
		t.Fatalf("expected predicate on external_subject_reference_key.value in SQL, got: %s", sql)
	}
	if argListContains(args, 0) {
		t.Fatalf("did not expect args to contain %d (no bindings), got %#v", 0, args)
	}
	// Prepared queries pass constants like SELECT 1 as bind args; ensure the literal value is present.
	if !argListContains(args, string(lit)) {
		t.Fatalf("expected args to contain %q, got %#v", string(lit), args)
	}
}
