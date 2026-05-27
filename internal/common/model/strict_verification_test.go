package model

import "testing"

func TestParseVerificationMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected VerificationMode
		wantErr  bool
	}{
		{name: "off", input: "off", expected: VerificationModeOff},
		{name: "permissive", input: "permissive", expected: VerificationModePermissive},
		{name: "strict", input: "strict", expected: VerificationModeStrict},
		{name: "mixed case", input: " PeRmIsSiVe ", expected: VerificationModePermissive},
		{name: "invalid bool", input: "true", wantErr: true},
		{name: "invalid unknown", input: "legacy", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := ParseVerificationMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if actual != tt.expected {
				t.Fatalf("unexpected mode, got %q want %q", actual, tt.expected)
			}
		})
	}
}

func TestSetStrictVerificationEnabledCompatibilityWrapper(t *testing.T) {
	SetStrictVerificationEnabled(false)
	if mode := GetVerificationMode(); mode != VerificationModeOff {
		t.Fatalf("expected off mode, got %q", mode)
	}

	SetStrictVerificationEnabled(true)
	if mode := GetVerificationMode(); mode != VerificationModeStrict {
		t.Fatalf("expected strict mode, got %q", mode)
	}
}
