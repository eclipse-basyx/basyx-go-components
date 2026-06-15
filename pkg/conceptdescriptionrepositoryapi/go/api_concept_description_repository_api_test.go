/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

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

func TestParseTime_QueryDecodedPositiveTimezoneOffsetParses(t *testing.T) {
	t.Parallel()

	got, err := parseTime("2026-06-01T14:30:15 02:00")
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}

	want := time.Date(2026, time.June, 1, 12, 30, 15, 0, time.UTC)
	if !got.UTC().Equal(want) {
		t.Fatalf("expected %v, got %v", want, got.UTC())
	}
}

func TestParseTime_InvalidValueFails(t *testing.T) {
	t.Parallel()

	_, err := parseTime("not-a-time")
	if err == nil {
		t.Fatalf("expected parse error for invalid timestamp")
	}
}
