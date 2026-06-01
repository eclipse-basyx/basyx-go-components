package grammar

import "testing"

func TestSimplifyForBackendFilter_UnresolvedAttributeStringComparison_FailsClosed(t *testing.T) {
	role := map[string]any{"CLAIM": "role"}
	viewRole := StandardString("view_digital_twin")
	le := LogicalExpression{
		Contains: []StringValue{
			{Attribute: role},
			{StrVal: &viewRole},
		},
	}

	simplified, decision := le.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
	if decision != SimplifyFalse {
		t.Fatalf("expected SimplifyFalse, got %v", decision)
	}
	if simplified.Boolean == nil || *simplified.Boolean {
		t.Fatalf("expected boolean false, got %#v", simplified.Boolean)
	}
}

func TestSimplifyForBackendFilter_UnresolvedAttributeNumCast_FailsClosed(t *testing.T) {
	role := map[string]any{"CLAIM": "clear"}
	minClearance := float64(1)
	le := LogicalExpression{
		Ge: []Value{
			{NumCast: &Value{Attribute: role}},
			{NumVal: &minClearance},
		},
	}

	simplified, decision := le.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
	if decision != SimplifyFalse {
		t.Fatalf("expected SimplifyFalse, got %v", decision)
	}
	if simplified.Boolean == nil || *simplified.Boolean {
		t.Fatalf("expected boolean false, got %#v", simplified.Boolean)
	}
}
