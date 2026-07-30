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
		fileName            string
		declared            []string
		expectedContentType string
		expectedMismatch    bool
	}{
		{
			name:                "declared image wins over detected image",
			detected:            "image/gif",
			fileName:            "demo.bin",
			declared:            []string{"image/png"},
			expectedContentType: "image/png",
			expectedMismatch:    true,
		},
		{
			name:                "binary fallback detection does not report mismatch",
			detected:            "application/octet-stream",
			fileName:            "demo.bin",
			declared:            []string{"image/png"},
			expectedContentType: "image/png",
			expectedMismatch:    false,
		},
		{
			name:                "declared ZIP is specific",
			detected:            "application/zip",
			fileName:            "archive.bin",
			declared:            []string{"application/zip"},
			expectedContentType: "application/zip",
			expectedMismatch:    false,
		},
		{
			name:                "IFC declaration wins over text sniff",
			detected:            "text/plain; charset=utf-8",
			fileName:            "model.ifc",
			declared:            []string{"application/x-step"},
			expectedContentType: "application/x-step",
			expectedMismatch:    true,
		},
		{
			name:                "CSV declaration wins over text sniff",
			detected:            "text/plain; charset=utf-8",
			fileName:            "data.csv",
			declared:            []string{"text/csv"},
			expectedContentType: "text/csv",
			expectedMismatch:    true,
		},
		{
			name:                "SVG declaration wins over generic XML sniff",
			detected:            "text/xml; charset=utf-8",
			fileName:            "diagram.svg",
			declared:            []string{"image/svg+xml"},
			expectedContentType: "image/svg+xml",
			expectedMismatch:    true,
		},
		{
			name:                "detected with parameters normalized",
			detected:            "text/plain; charset=utf-8",
			fileName:            "doc.txt",
			declared:            []string{"TEXT/PLAIN; charset=us-ascii"},
			expectedContentType: "text/plain",
			expectedMismatch:    false,
		},
		{
			name:                "first specific declaration wins",
			detected:            "text/plain",
			fileName:            "model.ifc",
			declared:            []string{"application/octet-stream", "application/x-step", "text/csv"},
			expectedContentType: "application/x-step",
			expectedMismatch:    true,
		},
		{
			name:                "invalid and generic declarations fall back to detected",
			detected:            "image/gif",
			fileName:            "demo.bin",
			declared:            []string{"not/a valid content type", "application/octet-stream"},
			expectedContentType: "image/gif",
			expectedMismatch:    false,
		},
		{
			name:                "weak detection and invalid declaration fall back to extension",
			detected:            "application/octet-stream",
			fileName:            "picture.tif",
			declared:            []string{"not/a valid content type"},
			expectedContentType: "image/tiff",
			expectedMismatch:    false,
		},
		{
			name:                "plain text detection falls back to CSV extension",
			detected:            "text/plain; charset=utf-8",
			fileName:            "data.csv",
			expectedContentType: "text/csv",
			expectedMismatch:    false,
		},
		{
			name:                "generic XML detection falls back to SVG extension",
			detected:            "text/xml; charset=utf-8",
			fileName:            "diagram.svg",
			expectedContentType: "image/svg+xml",
			expectedMismatch:    false,
		},
		{
			name:                "ZIP detection falls back to filename extension",
			detected:            "application/zip",
			fileName:            "diagram.svg",
			expectedContentType: "image/svg+xml",
			expectedMismatch:    false,
		},
		{
			name:                "extension wins over generic declaration",
			detected:            "application/octet-stream",
			fileName:            "data.csv",
			declared:            []string{"text/plain"},
			expectedContentType: "text/csv",
			expectedMismatch:    false,
		},
		{
			name:                "generic declaration is retained after other fallbacks",
			detected:            "application/octet-stream",
			fileName:            "attachment.bin",
			declared:            []string{"text/plain"},
			expectedContentType: "text/plain",
			expectedMismatch:    false,
		},
		{
			name:                "binary placeholder does not mask later generic declaration",
			detected:            "application/octet-stream",
			fileName:            "attachment.bin",
			declared:            []string{"application/octet-stream", "text/plain"},
			expectedContentType: "text/plain",
			expectedMismatch:    false,
		},
		{
			name:                "all empty falls back to binary",
			detected:            "",
			fileName:            "",
			expectedContentType: "application/octet-stream",
			expectedMismatch:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, mismatch := ResolveUploadedContentType(tt.detected, tt.fileName, tt.declared...)

			if resolved != tt.expectedContentType {
				t.Fatalf("expected content type %q, got %q", tt.expectedContentType, resolved)
			}
			if mismatch != tt.expectedMismatch {
				t.Fatalf("expected mismatch %t, got %t", tt.expectedMismatch, mismatch)
			}
		})
	}
}

func TestHasAuthoritativeContentTypeDeclaration(t *testing.T) {
	tests := []struct {
		name         string
		declarations []string
		expected     bool
	}{
		{
			name:         "specific declaration",
			declarations: []string{"application/x-step"},
			expected:     true,
		},
		{
			name:         "plain text declaration",
			declarations: []string{"text/plain; charset=utf-8"},
			expected:     true,
		},
		{
			name:         "XML declaration",
			declarations: []string{"application/xml"},
			expected:     true,
		},
		{
			name:         "ZIP declaration",
			declarations: []string{"application/zip"},
			expected:     true,
		},
		{
			name:         "binary placeholder",
			declarations: []string{"application/octet-stream"},
			expected:     false,
		},
		{
			name:         "invalid and empty declarations",
			declarations: []string{"", "not a content type"},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := HasAuthoritativeContentTypeDeclaration(tt.declarations...); actual != tt.expected {
				t.Fatalf("expected %t, got %t", tt.expected, actual)
			}
		})
	}
}
