package auth

import "testing"

func TestResolveAttributeValue_MissingClaimReturnsNil(t *testing.T) {
	attr := map[string]any{"CLAIM": "clear"}
	claims := Claims{"role": "editor"}

	got := resolveAttributeValue(attr, claims)
	if got != nil {
		t.Fatalf("expected nil for missing claim, got %#v", got)
	}
}

func TestResolveAttributeValue_ClaimArrayUnwrapsFirstElement(t *testing.T) {
	attr := map[string]any{"CLAIM": "role"}
	claims := Claims{"role": []any{"editor"}}

	got := resolveAttributeValue(attr, claims)
	if got != "editor" {
		t.Fatalf("expected editor, got %#v", got)
	}
}
