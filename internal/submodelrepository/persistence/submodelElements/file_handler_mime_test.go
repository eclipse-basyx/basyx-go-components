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

package submodelelements

import (
	"database/sql"
	"io"
	"strings"
	"testing"
)

func TestResolveUploadFileMetadataDeclarationSources(t *testing.T) {
	tests := []struct {
		name                 string
		content              string
		fileName             string
		currentDeclarations  []string
		persistedContentType string
		expectedContentType  string
	}{
		{
			name:                 "current plain text blocks stale persisted type",
			content:              "plain text attachment",
			fileName:             "attachment.bin",
			currentDeclarations:  []string{"text/plain"},
			persistedContentType: "application/pdf",
			expectedContentType:  "text/plain",
		},
		{
			name:                 "current XML blocks stale persisted type",
			content:              "<?xml version=\"1.0\"?><root/>",
			fileName:             "attachment.bin",
			currentDeclarations:  []string{"application/xml"},
			persistedContentType: "application/pdf",
			expectedContentType:  "application/xml",
		},
		{
			name:                 "current ZIP blocks stale persisted type",
			content:              "PK\x03\x04archive",
			fileName:             "attachment.bin",
			currentDeclarations:  []string{"application/zip"},
			persistedContentType: "application/pdf",
			expectedContentType:  "application/zip",
		},
		{
			name:                 "binary placeholder allows persisted fallback",
			content:              "plain text attachment",
			fileName:             "attachment.bin",
			currentDeclarations:  []string{"application/octet-stream"},
			persistedContentType: "application/pdf",
			expectedContentType:  "application/pdf",
		},
		{
			name:                 "invalid current declaration allows persisted fallback",
			content:              "plain text attachment",
			fileName:             "attachment.bin",
			currentDeclarations:  []string{"not a content type"},
			persistedContentType: "application/pdf",
			expectedContentType:  "application/pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedFileName, resolvedContentType, uploadContent, err := resolveUploadFileMetadata(
				strings.NewReader(tt.content),
				tt.fileName,
				tt.currentDeclarations,
				fileElementUploadMetadata{
					existingContentType: sql.NullString{String: tt.persistedContentType, Valid: true},
				},
			)
			if err != nil {
				t.Fatalf("resolve upload metadata: %v", err)
			}
			if resolvedFileName != tt.fileName {
				t.Fatalf("expected filename %q, got %q", tt.fileName, resolvedFileName)
			}
			if resolvedContentType != tt.expectedContentType {
				t.Fatalf("expected content type %q, got %q", tt.expectedContentType, resolvedContentType)
			}
			replayed, err := io.ReadAll(uploadContent)
			if err != nil {
				t.Fatalf("read replayed upload: %v", err)
			}
			if string(replayed) != tt.content {
				t.Fatalf("replayed content differs: got %q", string(replayed))
			}
		})
	}
}
