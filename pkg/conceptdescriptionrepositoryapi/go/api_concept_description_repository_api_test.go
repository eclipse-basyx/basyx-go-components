package openapi

import (
	"testing"
	"time"
)

func TestParseTime_EmptyAndWhitespaceReturnZeroTime(t *testing.T) {
	t.Parallel()

	tests := []string{"", "   ", "\t\n"}
	for _, input := range tests {
		input := input
		t.Run(input, func(t *testing.T) {
			got, err := parseTime(input)
			if err != nil {
				t.Fatalf("expected no error for input %q, got %v", input, err)
			}
			if !got.IsZero() {
				t.Fatalf("expected zero time for input %q, got %v", input, got)
			}
		})
	}
}

func TestParseTime_TrimmedRFC3339NanoValueParses(t *testing.T) {
	t.Parallel()

	input := "  2026-06-01T12:30:15.123456789Z  "
	got, err := parseTime(input)
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}

	want := time.Date(2026, time.June, 1, 12, 30, 15, 123456789, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestParseTime_InvalidValueFails(t *testing.T) {
	t.Parallel()

	_, err := parseTime("not-a-time")
	if err == nil {
		t.Fatalf("expected parse error for invalid timestamp")
	}
}
