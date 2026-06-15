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
