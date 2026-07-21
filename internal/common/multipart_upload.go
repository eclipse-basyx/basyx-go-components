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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

const maxMultipartMetadataBytes int64 = 1 << 20

type multipartFileStager func(context.Context, string, string, io.Reader, int64) (StagedUpload, error)

// MultipartUpload contains one staged file and its text form fields.
type MultipartUpload struct {
	// File is the staged, seekable file payload and must be closed by the caller.
	File StagedUpload
	// MultipartFileName is the filename declared by the multipart file part.
	MultipartFileName string
	// FileContentType is the media type declared by the multipart file part.
	FileContentType string
	// Fields contains all non-file form values in encounter order.
	Fields map[string][]string
}

// Close discards the staged file unless it has already been promoted.
//
// Returns:
//   - error: Cleanup error, or nil when already closed/promoted.
func (upload *MultipartUpload) Close() error {
	if upload == nil || upload.File == nil {
		return nil
	}
	return upload.File.Close()
}

// FirstField returns the first form value for a field.
//
// Parameters:
//   - name: Multipart form field name.
//
// Returns:
//   - string: First value, or an empty string when absent.
func (upload *MultipartUpload) FirstField(name string) string {
	if upload == nil {
		return ""
	}
	values := upload.Fields[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// ReadMultipartUpload parses a request sequentially and stages exactly one file part.
//
// Parameters:
//   - w: Response writer used by http.MaxBytesReader.
//   - r: Multipart HTTP request.
//   - maxRequestBytes: Maximum complete request size, including multipart framing.
//   - fileField: Accepted file-part field name.
//   - stager: External seekable-storage provider.
//
// Returns:
//   - *MultipartUpload: Parsed metadata and caller-owned staged file.
//   - error: Coded 400, 413, or internal staging error.
func ReadMultipartUpload(
	w http.ResponseWriter,
	r *http.Request,
	maxRequestBytes int64,
	fileField string,
	stager UploadStager,
) (*MultipartUpload, error) {
	return ReadMultipartUploadFields(w, r, maxRequestBytes, stager, fileField)
}

// ReadMultipartUploadFields stages exactly one part matching any supplied field name.
//
// Parameters:
//   - w: Response writer used by http.MaxBytesReader.
//   - r: Multipart HTTP request.
//   - maxRequestBytes: Maximum complete request size, including multipart framing.
//   - stager: External seekable-storage provider.
//   - fileFields: Accepted file-part field names.
//
// Returns:
//   - *MultipartUpload: Parsed metadata and caller-owned staged file.
//   - error: Coded 400, 413, or internal staging error.
func ReadMultipartUploadFields(
	w http.ResponseWriter,
	r *http.Request,
	maxRequestBytes int64,
	stager UploadStager,
	fileFields ...string,
) (*MultipartUpload, error) {
	if stager == nil {
		return nil, NewInternalServerError("COMMON-MULTIPART-NILSTAGER upload stager is required")
	}
	return readMultipartUploadFields(w, r, maxRequestBytes, func(ctx context.Context, _ string, _ string, input io.Reader, maximum int64) (StagedUpload, error) {
		return stager(ctx, input, maximum)
	}, fileFields...)
}

func readMultipartUploadFields(
	w http.ResponseWriter,
	r *http.Request,
	maxRequestBytes int64,
	stager multipartFileStager,
	fileFields ...string,
) (*MultipartUpload, error) {
	if r == nil || stager == nil {
		return nil, NewInternalServerError("COMMON-MULTIPART-INVALID request and upload stager are required")
	}
	if maxRequestBytes <= 0 {
		maxRequestBytes = DefaultConfig.GeneralUploadMaxSizeBytes
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return nil, NewErrBadRequest("COMMON-MULTIPART-CONTENTTYPE multipart/form-data is required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, NewErrBadRequest("COMMON-MULTIPART-CREATEREADER " + err.Error())
	}

	acceptedFileFields := make(map[string]struct{}, len(fileFields))
	for _, field := range fileFields {
		acceptedFileFields[field] = struct{}{}
	}
	result := &MultipartUpload{Fields: make(map[string][]string)}
	cleanup := true
	defer func() {
		if cleanup {
			_ = result.Close()
		}
	}()

	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, normalizeMultipartReadError(nextErr, maxRequestBytes)
		}

		name := part.FormName()
		if _, isFile := acceptedFileFields[name]; isFile {
			if result.File != nil {
				_ = part.Close()
				return nil, NewErrBadRequest("COMMON-MULTIPART-DUPLICATEFILE exactly one file part is allowed")
			}
			result.MultipartFileName = strings.TrimSpace(part.FileName())
			result.FileContentType = strings.TrimSpace(part.Header.Get("Content-Type"))
			result.File, err = stager(r.Context(), result.MultipartFileName, result.FileContentType, part, maxRequestBytes)
			closeErr := part.Close()
			if err != nil {
				return nil, normalizeMultipartReadError(err, maxRequestBytes)
			}
			if closeErr != nil {
				return nil, normalizeMultipartReadError(closeErr, maxRequestBytes)
			}
			continue
		}

		value, readErr := io.ReadAll(io.LimitReader(part, maxMultipartMetadataBytes+1))
		closeErr := part.Close()
		if readErr != nil {
			return nil, normalizeMultipartReadError(readErr, maxRequestBytes)
		}
		if closeErr != nil {
			return nil, normalizeMultipartReadError(closeErr, maxRequestBytes)
		}
		if int64(len(value)) > maxMultipartMetadataBytes {
			return nil, NewErrPayloadTooLarge(fmt.Sprintf("COMMON-MULTIPART-METADATA field %q exceeds %d bytes", name, maxMultipartMetadataBytes))
		}
		result.Fields[name] = append(result.Fields[name], string(value))
	}

	if result.File == nil {
		return nil, NewErrBadRequest(fmt.Sprintf("COMMON-MULTIPART-MISSINGFILE one of multipart file fields %q is required", fileFields))
	}
	cleanup = false
	return result, nil
}

func normalizeMultipartReadError(err error, maximum int64) error {
	if IsErrPayloadTooLarge(err) {
		return err
	}
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return NewErrPayloadTooLarge(fmt.Sprintf("COMMON-MULTIPART-TOOLARGE request exceeds configured maximum of %d bytes", maximum))
	}
	return NewErrBadRequest("COMMON-MULTIPART-READ " + err.Error())
}
