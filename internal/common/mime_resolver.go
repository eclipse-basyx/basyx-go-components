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

import (
	"mime"
	"path/filepath"
	"strings"
)

const fallbackBinaryContentType = "application/octet-stream"

// ResolveUploadedContentType resolves a stable content type for uploaded files.
//
// Precedence:
//  1. Detected content type when it is specific (not empty and not fallback binary)
//  2. Declared content type
//  3. Filename extension mapping
//  4. application/octet-stream
//
// mismatchDetectedVsDeclared is true if both detected and declared are specific and differ.
func ResolveUploadedContentType(detectedContentType, declaredContentType, fileName string) (resolved string, mismatchDetectedVsDeclared bool) {
	normalizedDetected := normalizeContentType(detectedContentType)
	normalizedDeclared := normalizeContentType(declaredContentType)

	if isSpecificContentType(normalizedDetected) {
		return normalizedDetected, isSpecificContentType(normalizedDeclared) && normalizedDetected != normalizedDeclared
	}

	if normalizedDeclared != "" {
		return normalizedDeclared, false
	}

	if extensionContentType := contentTypeFromExtension(fileName); extensionContentType != "" {
		return extensionContentType, false
	}

	return fallbackBinaryContentType, false
}

func normalizeContentType(rawContentType string) string {
	trimmed := strings.TrimSpace(strings.ToLower(rawContentType))
	if trimmed == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err != nil {
		return ""
	}

	return strings.ToLower(strings.TrimSpace(mediaType))
}

func isSpecificContentType(contentType string) bool {
	return contentType != "" && contentType != fallbackBinaryContentType
}

func contentTypeFromExtension(fileName string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
	if ext == "" {
		return ""
	}

	return normalizeContentType(mime.TypeByExtension(ext))
}
