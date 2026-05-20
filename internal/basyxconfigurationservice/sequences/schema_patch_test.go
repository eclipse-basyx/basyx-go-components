package steps

import "testing"

func TestCompareSemanticVersions(t *testing.T) {
	testCases := []struct {
		name    string
		current string
		target  string
		want    int
		wantErr bool
	}{
		{name: "equal", current: "v1.0.1", target: "1.0.1", want: 0},
		{name: "current lower", current: "1.0.1", target: "1.0.2", want: -1},
		{name: "current higher", current: "1.2.0", target: "1.1.9", want: 1},
		{name: "invalid current", current: "1.0", target: "1.0.1", wantErr: true},
		{name: "invalid target", current: "1.0.0", target: "abc", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compareSemanticVersions(tc.current, tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("compareSemanticVersions()=%d want=%d", got, tc.want)
			}
		})
	}
}
