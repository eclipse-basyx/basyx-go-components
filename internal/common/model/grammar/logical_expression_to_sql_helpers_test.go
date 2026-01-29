package grammar

import "testing"

func mustCollectorForRoot(t *testing.T, root string) *ResolvedFieldPathCollector {
	t.Helper()
	parsed, err := ParseCollectorRoot(root)
	if err != nil {
		t.Fatalf("ParseCollectorRoot returned error: %v", err)
	}
	collector, err := NewResolvedFieldPathCollectorForRoot(parsed)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}
	return collector
}
