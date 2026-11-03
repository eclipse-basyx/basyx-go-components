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

package binarycontent

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const pngSignature = "\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR"

func TestResolveUploadContentTypeSources(t *testing.T) {
	tests := []struct {
		name                string
		content             string
		declarations        []string
		storedContentType   string
		expectedContentType string
	}{
		{"current plain text blocks stale stored type", "plain text attachment", []string{"text/plain"}, "application/pdf", "text/plain"},
		{"current XML blocks stale stored type", "<?xml version=\"1.0\"?><root/>", []string{"application/xml"}, "application/pdf", "application/xml"},
		{"current ZIP blocks stale stored type", "PK\x03\x04archive", []string{"application/zip"}, "application/pdf", "application/zip"},
		{"current PNG replaces stored JPEG", pngSignature, []string{"image/png"}, "image/jpeg", "image/png"},
		{"binary placeholder allows stored fallback", "plain text attachment", []string{"application/octet-stream"}, "application/pdf", "application/pdf"},
		{"invalid declaration allows stored fallback", "plain text attachment", []string{"not a content type"}, "application/pdf", "application/pdf"},
		{"no declaration and no stored type uses detection", pngSignature, nil, "", "image/png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upload, err := ResolveUpload(
				strings.NewReader(tt.content),
				UploadDeclaration{FileName: "attachment.bin", ContentTypes: tt.declarations},
				StoredMetadata{ContentType: tt.storedContentType},
				"file",
			)
			require.NoError(t, err)
			require.Equal(t, tt.expectedContentType, upload.ContentType)
			require.Equal(t, "attachment.bin", upload.FileName)
			replayed, err := io.ReadAll(upload.Content)
			require.NoError(t, err)
			require.Equal(t, tt.content, string(replayed))
		})
	}
}

func TestResolveUploadFileName(t *testing.T) {
	tests := []struct {
		name             string
		declaredFileName string
		storedFileName   string
		declaredType     string
		expectedFileName string
	}{
		{"declared name wins", " photo.png ", "old.jpg", "image/png", "photo.png"},
		{"stored name used when none declared", "", "old.jpg", "image/png", "old.jpg"},
		{"fallback name with preferred extension", "", "", "image/jpeg", "manual.jpg"},
		{"fallback name for plain text", "", "", "text/plain; charset=utf-8", "manual.txt"},
		{"fallback name for undeclared binary", "", "", "", "manual.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upload, err := ResolveUpload(
				strings.NewReader("\x00\x01binary"),
				UploadDeclaration{FileName: tt.declaredFileName, ContentTypes: []string{tt.declaredType}},
				StoredMetadata{FileName: tt.storedFileName},
				"manual",
			)
			require.NoError(t, err)
			require.Equal(t, tt.expectedFileName, upload.FileName)
			_, err = SafeFileName(upload.FileName)
			require.NoError(t, err)
		})
	}
}

func TestFallbackFileNameWithoutKnownExtension(t *testing.T) {
	require.Equal(t, "thumbnail", FallbackFileName("thumbnail", "application/x-basyx-unknown"))
	require.Equal(t, "thumbnail", FallbackFileName("thumbnail", "not a content type"))
}
