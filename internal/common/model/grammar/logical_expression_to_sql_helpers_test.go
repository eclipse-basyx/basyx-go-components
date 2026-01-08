package grammar

import "testing"

func mustCollectorForRoot(t *testing.T, root string, alias string) *ResolvedFieldPathCollector {
	t.Helper()
	collector, err := NewResolvedFieldPathCollectorForRoot(root, alias)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}
	return collector
}
