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
	"mime"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

var preferredFileExtensions = map[string]string{
	"application/json":         ".json",
	"application/octet-stream": ".bin",
	"application/pdf":          ".pdf",
	"application/xml":          ".xml",
	"application/zip":          ".zip",
	"image/bmp":                ".bmp",
	"image/gif":                ".gif",
	"image/jpeg":               ".jpg",
	"image/png":                ".png",
	"image/svg+xml":            ".svg",
	"image/tiff":               ".tiff",
	"image/webp":               ".webp",
	"text/csv":                 ".csv",
	"text/html":                ".html",
	"text/plain":               ".txt",
	"text/xml":                 ".xml",
}

// UploadDeclaration is what an upload states about its own content.
type UploadDeclaration struct {
	FileName     string
	ContentTypes []string
}

// StoredMetadata is what the upload target already records about its content.
type StoredMetadata struct {
	FileName    string
	ContentType string
}

// ResolvedUpload is an upload stream together with its resolved metadata.
type ResolvedUpload struct {
	FileName     string
	ContentType  string
	Content      io.Reader
	TypeMismatch bool
}

// ResolveUpload sniffs an upload and resolves its file name and content type.
//
// Stored metadata applies only where the upload declares nothing. A nameless
// upload into a target without a stored name is named fallbackBaseName plus an
// extension matching the resolved content type.
func ResolveUpload(file io.Reader, declaration UploadDeclaration, stored StoredMetadata, fallbackBaseName string) (ResolvedUpload, error) {
	detectedContentType, content, err := common.SniffContentTypeReader(file)
	if err != nil {
		return ResolvedUpload{}, common.NewInternalServerError("BINARYCONTENT-RESOLVEUPLOAD-SNIFF " + err.Error())
	}
	fileName := firstNonBlank(declaration.FileName, stored.FileName)
	declaredContentTypes := declaration.ContentTypes
	if !common.HasAuthoritativeContentTypeDeclaration(declaredContentTypes...) {
		declaredContentTypes = append(append([]string{}, declaredContentTypes...), stored.ContentType)
	}
	contentType, mismatch := common.ResolveUploadedContentType(detectedContentType, fileName, declaredContentTypes...)
	if fileName == "" {
		fileName = FallbackFileName(fallbackBaseName, contentType)
	}
	return ResolvedUpload{FileName: fileName, ContentType: contentType, Content: content, TypeMismatch: mismatch}, nil
}

// FallbackFileName names a nameless upload after its target and content type.
func FallbackFileName(baseName string, contentType string) string {
	return strings.TrimSpace(baseName) + fileExtensionForContentType(contentType)
}

func fileExtensionForContentType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	if extension, ok := preferredFileExtensions[mediaType]; ok {
		return extension
	}
	extensions, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(extensions) == 0 {
		return ""
	}
	return extensions[0]
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
