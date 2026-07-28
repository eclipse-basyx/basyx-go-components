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
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

const fallbackBinaryContentType = "application/octet-stream"

// ResolveUploadedContentType resolves a stable content type for uploaded files.
//
// Precedence:
//  1. First specific declared content type
//  2. Detected content type when it is specific
//  3. Filename extension mapping
//  4. First generic declared content type
//  5. application/octet-stream
//
// Declared content types are evaluated in caller-provided priority order.
// mismatchDetectedVsDeclared is true if a selected specific declaration differs
// from a valid detected content type, including a generic detection result.
func ResolveUploadedContentType(detectedContentType, fileName string, declaredContentTypes ...string) (resolved string, mismatchDetectedVsDeclared bool) {
	normalizedDetected := normalizeContentType(detectedContentType)
	genericDeclared := ""

	for _, declaredContentType := range declaredContentTypes {
		normalizedDeclared := normalizeContentType(declaredContentType)
		if isSpecificDeclaredContentType(normalizedDeclared) {
			return normalizedDeclared,
				normalizedDetected != "" &&
					normalizedDetected != fallbackBinaryContentType &&
					normalizedDetected != normalizedDeclared
		}
		if normalizedDeclared != "" && normalizedDeclared != fallbackBinaryContentType && genericDeclared == "" {
			genericDeclared = normalizedDeclared
		}
	}

	if isSpecificDetectedContentType(normalizedDetected) {
		return normalizedDetected, false
	}

	if extensionContentType := contentTypeFromExtension(fileName); extensionContentType != "" && extensionContentType != fallbackBinaryContentType {
		return extensionContentType, false
	}

	if genericDeclared != "" {
		return genericDeclared, false
	}

	return fallbackBinaryContentType, false
}

// HasAuthoritativeContentTypeDeclaration reports whether declarations contain
// a valid content type other than the binary placeholder.
func HasAuthoritativeContentTypeDeclaration(declaredContentTypes ...string) bool {
	for _, declaredContentType := range declaredContentTypes {
		normalizedDeclared := normalizeContentType(declaredContentType)
		if normalizedDeclared != "" && normalizedDeclared != fallbackBinaryContentType {
			return true
		}
	}
	return false
}

// SniffContentTypeReader detects a stream's content type and returns a reader
// that replays the sniffed bytes before continuing with the remaining stream.
func SniffContentTypeReader(reader io.Reader) (string, io.Reader, error) {
	if reader == nil {
		return "", nil, errors.New("reader is nil")
	}

	contentTypeBuffer := make([]byte, 512)
	readBytes, err := io.ReadFull(reader, contentTypeBuffer)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", nil, err
	}

	detectedContentType := fallbackBinaryContentType
	if readBytes > 0 {
		detectedContentType = http.DetectContentType(contentTypeBuffer[:readBytes])
	}

	return detectedContentType, io.MultiReader(bytes.NewReader(contentTypeBuffer[:readBytes]), reader), nil
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

func isSpecificDeclaredContentType(contentType string) bool {
	return contentType != "" && !isGenericDeclaredContentType(contentType)
}

func isSpecificDetectedContentType(contentType string) bool {
	return contentType != "" && !isGenericDetectedContentType(contentType)
}

func isGenericDeclaredContentType(contentType string) bool {
	switch contentType {
	case fallbackBinaryContentType, "application/xml", "text/plain", "text/xml":
		return true
	default:
		return false
	}
}

func isGenericDetectedContentType(contentType string) bool {
	return contentType == "application/zip" || isGenericDeclaredContentType(contentType)
}

func contentTypeFromExtension(fileName string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
	if ext == "" {
		return ""
	}

	return normalizeContentType(mime.TypeByExtension(ext))
}
