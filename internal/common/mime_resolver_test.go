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

package common

import "testing"

func TestResolveUploadedContentType(t *testing.T) {
	tests := []struct {
		name                string
		detected            string
		declared            string
		fileName            string
		expectedContentType string
		expectedMismatch    bool
	}{
		{
			name:                "detected specific wins",
			detected:            "image/gif",
			declared:            "image/png",
			fileName:            "demo.bin",
			expectedContentType: "image/gif",
			expectedMismatch:    true,
		},
		{
			name:                "weak detected falls back to declared",
			detected:            "application/octet-stream",
			declared:            "image/tiff",
			fileName:            "demo.bin",
			expectedContentType: "image/tiff",
			expectedMismatch:    false,
		},
		{
			name:                "weak detected with invalid declared falls back to extension",
			detected:            "application/octet-stream",
			declared:            "not/a valid content type",
			fileName:            "picture.tif",
			expectedContentType: "image/tiff",
			expectedMismatch:    false,
		},
		{
			name:                "all weak falls back to binary",
			detected:            "application/octet-stream",
			declared:            "",
			fileName:            "",
			expectedContentType: "application/octet-stream",
			expectedMismatch:    false,
		},
		{
			name:                "detected with parameters normalized",
			detected:            "text/plain; charset=utf-8",
			declared:            "text/plain",
			fileName:            "doc.txt",
			expectedContentType: "text/plain",
			expectedMismatch:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, mismatch := ResolveUploadedContentType(tt.detected, tt.declared, tt.fileName)

			if resolved != tt.expectedContentType {
				t.Fatalf("expected content type %q, got %q", tt.expectedContentType, resolved)
			}
			if mismatch != tt.expectedMismatch {
				t.Fatalf("expected mismatch %t, got %t", tt.expectedMismatch, mismatch)
			}
		})
	}
}
