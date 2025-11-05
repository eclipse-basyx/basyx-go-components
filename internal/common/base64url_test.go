//go:build unit

package common

import (
	"testing"
)

func TestEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "SimpleString",
			input:    []byte("hello world"),
			expected: "aGVsbG8gd29ybGQ",
		},
		{
			name:     "EmptyString",
			input:    []byte(""),
			expected: "",
		},
		{
			name:     "WithSpecialChars",
			input:    []byte("hello+world/test"),
			expected: "aGVsbG8rd29ybGQvdGVzdA",
		},
		{
			name:     "WithNonASCII",
			input:    []byte("こんにちは"),
			expected: "44GT44KT44Gr44Gh44Gv",
		},
		{
			name:     "WithPaddingNeeded",
			input:    []byte("a"),
			expected: "YQ",
		},
		{
			name:     "WithPaddingNeeded2",
			input:    []byte("ab"),
			expected: "YWI",
		},
		{
			name:     "WithPaddingNeeded3",
			input:    []byte("abc"),
			expected: "YWJj",
		},
		{
			name:     "BinaryData",
			input:    []byte{0, 1, 2, 3, 255, 254},
			expected: "AAECA__-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Encode(tt.input)
			if result != tt.expected {
				t.Errorf("Encode(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []byte
		expectError bool
	}{
		{
			name:        "SimpleString",
			input:       "aGVsbG8gd29ybGQ",
			expected:    []byte("hello world"),
			expectError: false,
		},
		{
			name:        "EmptyString",
			input:       "",
			expected:    []byte{},
			expectError: false,
		},
		{
			name:        "WithDashUnderscoreChars",
			input:       "aGVsbG8td29ybGRfdGVzdA",
			expected:    []byte("hello-world_test"),
			expectError: false,
		},
		{
			name:        "WithNonASCII",
			input:       "44GT44KT44Gr44Gh44Gv",
			expected:    []byte("こんにちは"),
			expectError: false,
		},
		{
			name:        "InvalidBase64",
			input:       "!@#$%^",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "WithAutoPadding1",
			input:       "YQ",
			expected:    []byte("a"),
			expectError: false,
		},
		{
			name:        "WithAutoPadding2",
			input:       "YWI",
			expected:    []byte("ab"),
			expectError: false,
		},
		{
			name:        "WithAutoPadding3",
			input:       "YWJj",
			expected:    []byte("abc"),
			expectError: false,
		},
		{
			name:        "BinaryData",
			input:       "AAECA__-",
			expected:    []byte{0, 1, 2, 3, 255, 254},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Decode(tt.input)

			if tt.expectError && err == nil {
				t.Errorf("Decode(%q) expected error but got none", tt.input)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Decode(%q) unexpected error: %v", tt.input, err)
			}

			if !tt.expectError {
				if string(result) != string(tt.expected) {
					t.Errorf("Decode(%q) = %q, want %q", tt.input, string(result), string(tt.expected))
				}
			}
		})
	}
}

func TestEncodeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SimpleString",
			input:    "hello world",
			expected: "aGVsbG8gd29ybGQ",
		},
		{
			name:     "EmptyString",
			input:    "",
			expected: "",
		},
		{
			name:     "WithSpecialChars",
			input:    "hello+world/test",
			expected: "aGVsbG8rd29ybGQvdGVzdA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeString(tt.input)
			if result != tt.expected {
				t.Errorf("EncodeString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDecodeString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "SimpleString",
			input:       "aGVsbG8gd29ybGQ",
			expected:    "hello world",
			expectError: false,
		},
		{
			name:        "EmptyString",
			input:       "",
			expected:    "",
			expectError: false,
		},
		{
			name:        "WithDashUnderscoreChars",
			input:       "aGVsbG8td29ybGRfdGVzdA",
			expected:    "hello-world_test",
			expectError: false,
		},
		{
			name:        "InvalidBase64",
			input:       "!@#$%^",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeString(tt.input)

			if tt.expectError && err == nil {
				t.Errorf("DecodeString(%q) expected error but got none", tt.input)
			}

			if !tt.expectError && err != nil {
				t.Errorf("DecodeString(%q) unexpected error: %v", tt.input, err)
			}

			if !tt.expectError && result != tt.expected {
				t.Errorf("DecodeString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRoundtrip(t *testing.T) {
	tests := []string{
		"",
		"a",
		"ab",
		"abc",
		"hello world",
		"Hello, 世界!",
		"Special chars: !@#$%^&*()_+-=[]{}|;':\",./<>?",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			encoded := EncodeString(tt)
			decoded, err := DecodeString(encoded)

			if err != nil {
				t.Errorf("Failed to decode %q: %v", encoded, err)
			}

			if decoded != tt {
				t.Errorf("Roundtrip failed: original=%q, got=%q", tt, decoded)
			}
		})
	}
}
